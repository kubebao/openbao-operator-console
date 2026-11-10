package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Every connection holds ownership for its lifetime. A replacement connection must
// reacquire it before accepting work. One connection also bounds write concurrency.
func postgresPool(ctx context.Context, dsn string, owner bool) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL connection configuration")
	}
	cfg.MaxConns = 1
	cfg.MaxConnLifetime = 0
	cfg.MaxConnIdleTime = 0
	if owner {
		cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			var acquired bool
			if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(
 hashtextextended(current_database() || ':' || current_schema(), $1))`, int64(migrationLock)).
				Scan(&acquired); err != nil {
				return err
			}
			if !acquired {
				return errDatabaseOwned
			}
			return nil
		}
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := connect(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func postgresPlan(ctx context.Context, tx pgx.Tx, list []migration) (SchemaPlan, error) {
	var tables, columns int
	if err := tx.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM pg_class WHERE relnamespace = current_schema()::regnamespace) +
 (SELECT count(*) FROM pg_proc WHERE pronamespace = current_schema()::regnamespace) +
 (SELECT count(*) FROM pg_type WHERE typnamespace = current_schema()::regnamespace)`).
		Scan(&tables); err != nil {
		return SchemaPlan{}, err
	}
	if tables == 0 {
		return planSchema(list, nil, true)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema()
 AND table_name = 'console_schema_migration' AND column_name IN ('version','lineage','checksum')`).
		Scan(&columns); err != nil {
		return SchemaPlan{}, err
	}
	if columns != 3 {
		return SchemaPlan{}, errors.New(
			"legacy or unrecognized development database; recreate it deliberately; no automatic reset is performed",
		)
	}
	rows, err := tx.Query(ctx, "SELECT version, lineage, checksum FROM console_schema_migration ORDER BY version")
	if err != nil {
		return SchemaPlan{}, err
	}
	defer rows.Close()
	records := []schemaRecord{}
	for rows.Next() {
		var r schemaRecord
		if err := rows.Scan(&r.version, &r.lineage, &r.checksum); err != nil {
			return SchemaPlan{}, err
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return SchemaPlan{}, err
	}
	return planSchema(list, records, false)
}

func migrate(ctx context.Context, tx pgx.Tx, list []migration) error {
	plan, err := postgresPlan(ctx, tx, list)
	if err != nil {
		return err
	}
	return applySchema(ctx, func(ctx context.Context, query string, args ...interface{}) error {
		_, err := tx.Exec(ctx, query, args...)
		return err
	}, list, plan, false)
}

var errDatabaseOwned = errors.New("database is owned by another console or schema command; stop it first")
