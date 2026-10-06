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

	"github.com/dc-tec/openbao-operator-console/internal/contract"
	"github.com/dc-tec/openbao-operator-console/internal/evidence"
	"github.com/dc-tec/openbao-operator-console/internal/store/database"
)

//go:embed migrations/001_initial.sql
var schema string

//go:embed migrations/002_configuration.sql
var configurationSchema string

//go:embed migrations/003_credentials.sql
var credentialSchema string

type Postgres struct {
	pool      *pgxpool.Pool
	retention time.Duration
}

func repeatableRead() pgx.TxOptions {
	return pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
}

func Open(ctx context.Context, dsn string, retention time.Duration) (*Postgres, error) {
	if retention <= 0 {
		return nil, fmt.Errorf("observation retention must be positive")
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
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(694263281)"); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS console_schema_migration (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return nil, err
	}
	var version int
	if err := tx.QueryRow(ctx, "SELECT COALESCE(MAX(version), 0) FROM console_schema_migration").Scan(&version); err != nil {
		return nil, err
	}
	if version > 3 {
		return nil, fmt.Errorf("database schema is newer than this application")
	}
	if version == 0 {
		if _, err := tx.Exec(ctx, schema); err != nil {
			return nil, fmt.Errorf("initialize schema: %w", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO console_schema_migration(version) VALUES (1)"); err != nil {
			return nil, err
		}
	}
	if version < 2 {
		if _, err := tx.Exec(ctx, configurationSchema); err != nil {
			return nil, fmt.Errorf("migrate configuration schema: %w", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO console_schema_migration(version) VALUES (2)"); err != nil {
			return nil, err
		}
	}
	if version < 3 {
		if _, err := tx.Exec(ctx, credentialSchema); err != nil {
			return nil, fmt.Errorf("migrate credential schema: %w", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO console_schema_migration(version) VALUES (3)"); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	success = true
	return &Postgres{pool: pool, retention: retention}, nil
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

func (p *Postgres) Apply(ctx context.Context, batch Batch) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", batch.Connection.Id); err != nil {
		return err
	}
	queries := database.New(tx)
	storedConnection, err := queries.GetConnection(ctx, batch.Connection.Id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil {
		var previousConnection contract.Connection
		if err := json.Unmarshal(storedConnection, &previousConnection); err != nil {
			return fmt.Errorf("decode stored connection: %w", err)
		}
		if batch.Connection.Collection != "current" {
			batch.Connection.LastSuccessAt = previousConnection.LastSuccessAt
		}
		PreserveOperatorEvidence(&batch.Connection, previousConnection)
	}
	previousRows, err := queries.GetConnectionDeployments(ctx, batch.Connection.Id)
	if err != nil {
		return err
	}
	previous := make(map[string]contract.Deployment)
	for _, row := range previousRows {
		var d contract.Deployment
		if err := json.Unmarshal(row, &d); err != nil {
			return fmt.Errorf("decode stored deployment: %w", err)
		}
		previous[d.Id] = d
	}
	if batch.Connection.Collection != "current" {
		batch.Deployments = []contract.Deployment{}
		for _, d := range previous {
			evidence.CollectionFailure(&d, contract.EvidenceCollection(batch.Connection.Collection), batch.Connection.LastAttemptAt)
			batch.Deployments = append(batch.Deployments, d)
			if batch.Connection.LastSuccessAt == nil || d.Evidence.CollectedAt.After(*batch.Connection.LastSuccessAt) {
				stamp := d.Evidence.CollectedAt
				batch.Connection.LastSuccessAt = &stamp
			}
		}
	}
	connection, err := json.Marshal(batch.Connection)
	if err != nil {
		return err
	}
	if err := queries.UpsertConnection(ctx, database.UpsertConnectionParams{ID: batch.Connection.Id, Projection: connection}); err != nil {
		return err
	}
	ids := []string{}
	for _, current := range batch.Deployments {
		old, exists := previous[current.Id]
		if exists {
			PreserveFirstSeen(&current, old)
		}
		projection, err := json.Marshal(current)
		if err != nil {
			return err
		}
		ids = append(ids, current.Id)
		if err := queries.UpsertDeployment(ctx, database.UpsertDeploymentParams{ID: current.Id, ConnectionID: current.ConnectionId, Projection: projection}); err != nil {
			return err
		}
		stamp := pgtype.Timestamptz{Time: batch.Connection.LastAttemptAt, Valid: true}
		// Resource version and collection changes retain meaningful observations.
		if !exists || current.ResourceVersion != old.ResourceVersion || current.Health != old.Health || current.Evidence.Collection != old.Evidence.Collection || !SameFindings(current, old) {
			if err := queries.InsertObservation(ctx, database.InsertObservationParams{DeploymentID: current.Id, RecordedAt: stamp, Projection: projection}); err != nil {
				return err
			}
			if activity := Transition(current, old, stamp.Time); exists && activity != nil {
				encoded, err := json.Marshal(activity)
				if err != nil {
					return err
				}
				if err := queries.InsertActivity(ctx, database.InsertActivityParams{ID: activity.Id, RecordedAt: stamp, Projection: encoded}); err != nil {
					return err
				}
			}
		}
	}
	if batch.Connection.Collection == "current" {
		if err := queries.DeleteMissingDeployments(ctx, database.DeleteMissingDeploymentsParams{ConnectionID: batch.Connection.Id, Column2: ids}); err != nil {
			return err
		}
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

func (p *Postgres) Snapshot(ctx context.Context) (contract.Snapshot, error) {
	// A repeatable-read transaction keeps counts, topology, and details coherent.
	tx, err := p.pool.BeginTx(ctx, repeatableRead())
	if err != nil {
		return contract.Snapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	result := contract.Snapshot{Mode: "live", CollectedAt: time.Now().UTC(), Connections: []contract.Connection{}, Deployments: []contract.Deployment{}, Relationships: []contract.Relationship{}, Activity: []contract.Activity{}}
	connections, err := q.ListConnections(ctx)
	if err != nil {
		return result, err
	}
	for _, row := range connections {
		var c contract.Connection
		if err := json.Unmarshal(row, &c); err != nil {
			return result, err
		}
		if c.Operators == nil {
			c.Operators = []contract.Operator{}
		}
		if c.Capabilities == nil {
			c.Capabilities = []contract.ApiCapability{}
		}
		result.Connections = append(result.Connections, c)
	}
	deployments, err := q.ListDeployments(ctx)
	if err != nil {
		return result, err
	}
	for _, row := range deployments {
		var d contract.Deployment
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
		var a contract.Activity
		if err := json.Unmarshal(row, &a); err != nil {
			return result, err
		}
		result.Activity = append(result.Activity, a)
	}
	return result, tx.Commit(ctx)
}

func (p *Postgres) History(ctx context.Context, id string) ([]contract.Observation, error) {
	rows, err := database.New(p.pool).GetHistory(ctx, id)
	if err != nil {
		return nil, err
	}
	result := []contract.Observation{}
	for _, row := range rows {
		var d contract.Deployment
		if err := json.Unmarshal(row.Projection, &d); err != nil {
			return nil, err
		}
		result = append(result, contract.Observation{Deployment: d, RecordedAt: row.RecordedAt.Time})
	}
	return result, nil
}
