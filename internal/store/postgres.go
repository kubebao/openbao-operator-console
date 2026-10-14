package store

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/store/database"
)

// pruneInterval is the period of the history retention job.
const pruneInterval = time.Hour

type Postgres struct {
	pool      *pgxpool.Pool
	retention time.Duration
}

func repeatableRead() pgx.TxOptions {
	return pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
}

// Open connects to the database and applies pending schema migrations.
func Open(ctx context.Context, dsn string, retention time.Duration) (*Postgres, error) {
	return open(ctx, dsn, retention, migrations)
}

func open(ctx context.Context, dsn string, retention time.Duration, list []migration) (*Postgres, error) {
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
		return nil, fmt.Errorf("begin schema migrations: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := migrate(ctx, tx, list); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit schema migrations: %w", err)
	}
	success = true
	return &Postgres{pool: pool, retention: retention}, nil
}

// Database containers can become reachable after the application starts. Retry
// transport failures within the caller's startup deadline, but fail on SQL errors.
// A deadline error names the cause of the last attempt.
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
			return fmt.Errorf("%s: %w", connectFailure(err), ctx.Err())
		case <-timer.C:
		}
		delay = min(2*delay, 2*time.Second)
	}
}

// connectFailure describes a failed connection attempt. It omits the driver
// message, which contains connection parameters.
func connectFailure(err error) string {
	var (
		databaseError    *pgconn.PgError
		unknownAuthority x509.UnknownAuthorityError
		hostname         x509.HostnameError
		invalid          x509.CertificateInvalidError
		verification     *tls.CertificateVerificationError
		lookup           *net.DNSError
	)
	switch {
	case errors.As(err, &databaseError):
		return "database is starting up"
	case errors.As(err, &unknownAuthority):
		return "server certificate is not signed by a trusted CA"
	case errors.As(err, &hostname):
		return "server certificate does not match the host name"
	case errors.As(err, &invalid) && invalid.Reason == x509.Expired:
		return "server certificate is expired or not yet valid"
	case errors.As(err, &invalid), errors.As(err, &verification):
		return "server certificate is not valid"
	case errors.As(err, &lookup):
		return "host name lookup failed"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.Is(err, context.DeadlineExceeded):
		return "connection attempt timed out"
	default:
		return "database is unreachable"
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
	return tx.Commit(ctx)
}

// Prune deletes observations and activity recorded before now minus the retention.
func (p *Postgres) Prune(ctx context.Context, now time.Time) error {
	queries := database.New(p.pool)
	cutoff := pgtype.Timestamptz{Time: now.Add(-p.retention), Valid: true}
	if err := queries.PruneObservations(ctx, cutoff); err != nil {
		return fmt.Errorf("prune observations: %w", err)
	}
	if err := queries.PruneActivity(ctx, cutoff); err != nil {
		return fmt.Errorf("prune activity: %w", err)
	}
	return nil
}

// RunPruning applies the history retention at start and then hourly until ctx ends.
func (p *Postgres) RunPruning(ctx context.Context, logger *slog.Logger) {
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
			database.InsertObservationParams{
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
		if err := queries.InsertActivity(ctx, database.InsertActivityParams{
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
	return queries.DeleteMissingDeployments(ctx, database.DeleteMissingDeploymentsParams{ConnectionID: changes.connection.ID, Column2: ids})
}

// RetainConnections deletes the inventory, history, and activity of every stored
// connection that is not enrolled, in one transaction. A connection ID enrolled again
// later starts without inherited evidence.
func (p *Postgres) RetainConnections(ctx context.Context, enrolled []string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin connection removal: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := database.New(tx)
	removed, err := queries.ListRemovedConnections(ctx, enrolled)
	if err != nil {
		return fmt.Errorf("list removed connections: %w", err)
	}
	if len(removed) == 0 {
		return nil
	}
	for _, id := range removed {
		// Apply holds the same lock, so a batch of a removed connection cannot interleave.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", id); err != nil {
			return fmt.Errorf("lock removed connection: %w", err)
		}
	}
	for _, remove := range []func(context.Context, []string) error{
		queries.DeleteConnectionActivity,
		queries.DeleteConnectionObservations,
		queries.DeleteConnectionDeployments,
		queries.DeleteConnections,
	} {
		if err := remove(ctx, removed); err != nil {
			return fmt.Errorf("delete removed connections: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit connection removal: %w", err)
	}
	return nil
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
	return result, tx.Commit(ctx)
}
