package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/store/database"
)

// schemaVersion identifies schema.sql. Pre-release databases are not migrated.
const schemaVersion = 1

//go:embed schema.sql
var schema string

type Postgres struct {
	pool      *pgxpool.Pool
	retention time.Duration
}

func repeatableRead() pgx.TxOptions {
	return pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
}

func Open(ctx context.Context, dsn string, retention time.Duration) (*Postgres, error) {
	if retention <= 0 {
		return nil, errors.New("observation retention must be positive")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("configure database: %w", err)
	}
	success := false
	defer func() {
		if !success {
			pool.Close()
		}
	}()
	if err := connect(ctx, pool); err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := initialize(ctx, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	success = true
	return &Postgres{pool: pool, retention: retention}, nil
}

// initialize creates the schema in an empty database. An advisory lock serializes
// concurrent starts.
func initialize(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(694263281)"); err != nil {
		return err
	}
	if _, err := tx.Exec(
		ctx,
		"CREATE TABLE IF NOT EXISTS console_schema_migration (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())",
	); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRow(ctx, "SELECT COALESCE(MAX(version), 0) FROM console_schema_migration").Scan(&version); err != nil {
		return err
	}
	switch version {
	case schemaVersion:
		return nil
	case 0:
		if _, err := tx.Exec(ctx, schema); err != nil {
			return fmt.Errorf("initialize schema: %w", err)
		}
		_, err := tx.Exec(ctx, "INSERT INTO console_schema_migration(version) VALUES ($1)", schemaVersion)
		return err
	default:
		return fmt.Errorf("database schema version %d is not supported; recreate the database", version)
	}
}

// Database containers can become reachable after the application starts. Retry
// transport failures within the caller's startup deadline, but fail on SQL errors.
func connect(ctx context.Context, pool *pgxpool.Pool) error {
	ctx, cancelStartup := context.WithTimeout(ctx, 30*time.Second)
	defer cancelStartup()
	delay := 250 * time.Millisecond
	for {
		attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := pool.Ping(attempt)
		cancel()
		if err == nil {
			return nil
		}
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code != "57P03" {
			return err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		delay = min(2*delay, 2*time.Second)
	}
}

func (p *Postgres) Close() { p.pool.Close() }

// Apply merges a collection batch with the stored state of its connection in one
// transaction. A per-connection advisory lock serializes concurrent batches.
func (p *Postgres) Apply(ctx context.Context, batch Batch) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", batch.Connection.ID); err != nil {
		return err
	}
	queries := database.New(tx)
	previous, stored, err := loadConnection(ctx, queries, batch.Connection.ID)
	if err != nil {
		return err
	}
	if err := writeChanges(ctx, queries, merge(batch, previous, stored)); err != nil {
		return err
	}
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-p.retention), Valid: true}
	if err := queries.PruneObservations(ctx, cutoff); err != nil {
		return err
	}
	if err := queries.PruneActivity(ctx, cutoff); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// loadConnection returns the stored connection, or nil before its first batch, and its
// deployments ordered by ID.
func loadConnection(ctx context.Context, queries *database.Queries, id string) (*fleet.Connection, []fleet.Deployment, error) {
	var previous *fleet.Connection
	row, err := queries.GetConnection(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
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

func writeChanges(ctx context.Context, queries *database.Queries, changes changes) error {
	connection, err := json.Marshal(changes.connection)
	if err != nil {
		return err
	}
	if err := queries.UpsertConnection(
		ctx,
		database.UpsertConnectionParams{ID: changes.connection.ID, Projection: connection},
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
			database.UpsertDeploymentParams{ID: deployment.ID, ConnectionID: deployment.ConnectionID, Projection: projection},
		); err != nil {
			return err
		}
	}
	recordedAt := pgtype.Timestamptz{Time: changes.connection.LastAttemptAt, Valid: true}
	for _, deployment := range changes.observations {
		projection, err := json.Marshal(deployment)
		if err != nil {
			return err
		}
		if err := queries.InsertObservation(
			ctx,
			database.InsertObservationParams{DeploymentID: deployment.ID, RecordedAt: recordedAt, Projection: projection},
		); err != nil {
			return err
		}
	}
	for _, activity := range changes.activity {
		projection, err := json.Marshal(activity)
		if err != nil {
			return err
		}
		if err := queries.InsertActivity(
			ctx,
			database.InsertActivityParams{ID: activity.ID, RecordedAt: recordedAt, Projection: projection},
		); err != nil {
			return err
		}
	}
	if !changes.complete {
		return nil
	}
	return queries.DeleteMissingDeployments(ctx, database.DeleteMissingDeploymentsParams{ConnectionID: changes.connection.ID, Column2: ids})
}

func (p *Postgres) Snapshot(ctx context.Context) (fleet.Snapshot, error) {
	// A repeatable-read transaction keeps counts, topology, and details coherent.
	tx, err := p.pool.BeginTx(ctx, repeatableRead())
	if err != nil {
		return fleet.Snapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	result := fleet.Snapshot{
		CollectedAt:   time.Now().UTC(),
		Connections:   []fleet.Connection{},
		Deployments:   []fleet.Deployment{},
		Relationships: []fleet.Relationship{},
		Activity:      []fleet.Activity{},
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
	activity, err := q.ListActivity(ctx)
	if err != nil {
		return result, err
	}
	for _, row := range activity {
		var a fleet.Activity
		if err := json.Unmarshal(row, &a); err != nil {
			return result, err
		}
		result.Activity = append(result.Activity, a)
	}
	return result, tx.Commit(ctx)
}

func (p *Postgres) History(ctx context.Context, id string) ([]fleet.Observation, error) {
	rows, err := database.New(p.pool).GetHistory(ctx, id)
	if err != nil {
		return nil, err
	}
	result := []fleet.Observation{}
	for _, row := range rows {
		var d fleet.Deployment
		if err := json.Unmarshal(row.Projection, &d); err != nil {
			return nil, err
		}
		result = append(result, fleet.Observation{Deployment: d, RecordedAt: row.RecordedAt.Time})
	}
	return result, nil
}
