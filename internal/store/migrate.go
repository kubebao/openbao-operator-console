package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// migration is one numbered schema change. A database records every applied version in
// console_schema_migration.
type migration struct {
	version    int
	statements string
}

//go:embed migrations/0001_baseline.sql
var baseline string

//go:embed migrations/0002_pagination_indexes.sql
var paginationIndexes string

// migrations are applied in order. Versions start at 1 and increase by one. Append new
// migrations as files in migrations/ so that sqlc reads the same schema. Never change a
// released migration.
var migrations = []migration{
	{version: 1, statements: baseline},
	{version: 2, statements: paginationIndexes},
}

// migrationLock serializes concurrent starts against one database.
const migrationLock = 694263281

// migrate applies every migration newer than the database version in the caller's
// transaction. A database newer than the binary is refused: downgrades are not supported.
func migrate(ctx context.Context, tx pgx.Tx, list []migration) error {
	if err := validMigrations(list); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLock); err != nil {
		return fmt.Errorf("lock schema migrations: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		"CREATE TABLE IF NOT EXISTS console_schema_migration (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())",
	); err != nil {
		return fmt.Errorf("create schema migration table: %w", err)
	}
	var current int
	if err := tx.QueryRow(ctx, "SELECT COALESCE(MAX(version), 0) FROM console_schema_migration").Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	latest := list[len(list)-1].version
	if current > latest {
		return fmt.Errorf(
			"database schema version %d is newer than version %d supported by this console; downgrades are not supported, "+
				"so run a console release that supports version %d or restore the pre-upgrade database backup",
			current,
			latest,
			current,
		)
	}
	for _, step := range list[current:] {
		if _, err := tx.Exec(ctx, step.statements); err != nil {
			return fmt.Errorf("apply schema migration %d: %w", step.version, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO console_schema_migration(version) VALUES ($1)", step.version); err != nil {
			return fmt.Errorf("record schema migration %d: %w", step.version, err)
		}
	}
	return nil
}

func validMigrations(list []migration) error {
	if len(list) == 0 {
		return errors.New("no schema migrations defined")
	}
	for i, step := range list {
		if step.version != i+1 {
			return fmt.Errorf("schema migration %d is out of order", step.version)
		}
	}
	return nil
}
