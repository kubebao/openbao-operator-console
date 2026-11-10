package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/store/sqlitedb"
)

// Apply merges a collection batch with the stored state of its connection in one
// transaction. The single SQLite connection serializes concurrent batches.
func (p *SQLite) Apply(ctx context.Context, batch Batch) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	queries := sqlitedb.New(tx)
	previous, stored, err := loadSQLiteConnection(ctx, queries, batch.Connection.ID)
	if err != nil {
		return err
	}
	if err := writeSQLiteChanges(ctx, queries, merge(batch, previous, stored)); err != nil {
		return err
	}
	return tx.Commit()
}

// Prune deletes observations and activity recorded before now minus the retention.
func (p *SQLite) Prune(ctx context.Context, now time.Time) error {
	queries := sqlitedb.New(p.db)
	cutoff := now.Add(-p.retention).UnixMicro()
	if err := queries.PruneObservations(ctx, cutoff); err != nil {
		return fmt.Errorf("prune observations: %w", err)
	}
	if err := queries.PruneActivity(ctx, cutoff); err != nil {
		return fmt.Errorf("prune activity: %w", err)
	}
	return nil
}

// RunPruning applies the history retention at start and then hourly until ctx ends.
func (p *SQLite) RunPruning(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(pruneInterval)
	defer ticker.Stop()
	for {
		prune, cancel := context.WithTimeout(ctx, time.Minute)
		if err := p.Prune(prune, time.Now()); err != nil && ctx.Err() == nil {
			logger.Error("Prune history failed")
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// loadConnection returns the stored connection, or nil before its first batch, and its
// deployments ordered by ID.
func loadSQLiteConnection(ctx context.Context, queries *sqlitedb.Queries, id string) (*fleet.Connection, []fleet.Deployment, error) {
	var previous *fleet.Connection
	row, err := queries.GetConnection(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return nil, nil, err
	default:
		previous = &fleet.Connection{}
		if err := json.Unmarshal(row, previous); err != nil {
			return nil, nil, fmt.Errorf("decode stored connection: %w", err)
		}
	}
	rows, err := queries.GetConnectionDeployments(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	stored := make([]fleet.Deployment, 0, len(rows))
	for _, row := range rows {
		var deployment fleet.Deployment
		if err := json.Unmarshal(row, &deployment); err != nil {
			return nil, nil, fmt.Errorf("decode stored deployment: %w", err)
		}
		stored = append(stored, deployment)
	}
	return previous, stored, nil
}

func writeSQLiteChanges(ctx context.Context, queries *sqlitedb.Queries, changes changes) error {
	connection, err := json.Marshal(changes.connection)
	if err != nil {
		return err
	}
	if err := queries.UpsertConnection(
		ctx,
		sqlitedb.UpsertConnectionParams{ID: changes.connection.ID, Projection: connection},
	); err != nil {
		return err
	}
	ids := make([]string, 0, len(changes.deployments))
	for _, deployment := range changes.deployments {
		projection, err := json.Marshal(deployment)
		if err != nil {
			return err
		}
		ids = append(ids, deployment.ID)
		if err := queries.UpsertDeployment(
			ctx,
			sqlitedb.UpsertDeploymentParams{ID: deployment.ID, ConnectionID: deployment.ConnectionID, Projection: projection},
		); err != nil {
			return err
		}
	}
	recordedAt := changes.connection.LastAttemptAt.UnixMicro()
	for _, deployment := range changes.observations {
		projection, err := json.Marshal(deployment)
		if err != nil {
			return err
		}
		if err := queries.InsertObservation(
			ctx,
			sqlitedb.InsertObservationParams{
				DeploymentID: deployment.ID,
				ConnectionID: deployment.ConnectionID,
				RecordedAt:   recordedAt,
				Projection:   projection,
			},
		); err != nil {
			return err
		}
	}
	for _, activity := range changes.activity {
		projection, err := json.Marshal(activity)
		if err != nil {
			return err
		}
		if err := queries.InsertActivity(ctx, sqlitedb.InsertActivityParams{
			ID:           activity.ID,
			DeploymentID: activity.DeploymentID,
			ConnectionID: activity.ConnectionID,
			Namespace:    activity.Namespace,
			RecordedAt:   recordedAt,
			Projection:   projection,
		}); err != nil {
			return err
		}
	}
	if !changes.complete {
		return nil
	}
	return queries.DeleteMissingDeployments(
		ctx,
		sqlitedb.DeleteMissingDeploymentsParams{ConnectionID: changes.connection.ID, Column2: stringList(ids)},
	)
}

// RetainConnections deletes the inventory, history, and activity of every stored
// connection that is not enrolled, in one transaction. A connection ID enrolled again
// later starts without inherited evidence.
func (p *SQLite) RetainConnections(ctx context.Context, enrolled []string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin connection removal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	queries := sqlitedb.New(tx)
	removed, err := queries.ListRemovedConnections(ctx, stringList(enrolled))
	if err != nil {
		return fmt.Errorf("list removed connections: %w", err)
	}
	if len(removed) == 0 {
		return nil
	}

	for _, remove := range []func(context.Context, string) error{
		queries.DeleteConnectionActivity,
		queries.DeleteConnectionObservations,
		queries.DeleteConnectionDeployments,
		queries.DeleteConnections,
	} {
		if err := remove(ctx, stringList(removed)); err != nil {
			return fmt.Errorf("delete removed connections: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit connection removal: %w", err)
	}
	return nil
}

func (p *SQLite) Snapshot(ctx context.Context) (fleet.Snapshot, error) {
	// A read transaction keeps counts, topology, and details coherent.
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fleet.Snapshot{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := sqlitedb.New(tx)
	result := fleet.Snapshot{
		GeneratedAt:   time.Now().UTC(),
		Connections:   []fleet.Connection{},
		Deployments:   []fleet.Deployment{},
		Relationships: []fleet.Relationship{},
	}
	connections, err := q.ListConnections(ctx)
	if err != nil {
		return result, err
	}
	for _, row := range connections {
		var c fleet.Connection
		if err := json.Unmarshal(row, &c); err != nil {
			return result, err
		}
		if c.Operators == nil {
			c.Operators = []fleet.Operator{}
		}
		if c.Capabilities == nil {
			c.Capabilities = []fleet.Capability{}
		}
		result.Connections = append(result.Connections, c)
	}
	deployments, err := q.ListDeployments(ctx)
	if err != nil {
		return result, err
	}
	for _, row := range deployments {
		var d fleet.Deployment
		if err := json.Unmarshal(row, &d); err != nil {
			return result, err
		}
		result.Deployments = append(result.Deployments, d)
	}
	return result, tx.Commit()
}
