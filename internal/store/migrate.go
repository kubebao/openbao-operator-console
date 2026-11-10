package store

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
)

// Schema versions are independent of application versions. Released steps are immutable.
type migration struct {
	version    int
	statements string
}

//go:embed migrations/0001_baseline.sql
var baseline string

//go:embed sqlite/migrations/0001_baseline.sql
var sqliteBaseline string

var (
	migrations       = []migration{{version: 1, statements: baseline}}
	sqliteMigrations = []migration{{version: 1, statements: sqliteBaseline}}
)

const (
	schemaLineage = "console-state-v1"
	migrationLock = 694263281
	metadataDDL   = `CREATE TABLE console_schema_migration (
 version integer PRIMARY KEY,
 lineage text NOT NULL,
 checksum text NOT NULL,
 applied_at text NOT NULL DEFAULT CURRENT_TIMESTAMP
)`
)

type schemaRecord struct {
	version           int
	lineage, checksum string
}

// SchemaPlan describes a validated migration history without changing the database.
type SchemaPlan struct {
	Current   int   `json:"current"`
	Supported int   `json:"supported"`
	Pending   []int `json:"pending"`
	Empty     bool  `json:"empty"`
}

func checksum(step migration) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(step.statements)))
}

func planSchema(list []migration, records []schemaRecord, empty bool) (SchemaPlan, error) {
	plan := SchemaPlan{Supported: len(list), Pending: []int{}, Empty: empty}
	if err := validMigrations(list); err != nil {
		return plan, err
	}
	if len(records) == 0 && !empty {
		return plan, errors.New("unrecognized or legacy development database; recreate it deliberately or restore a compatible backup")
	}
	if len(records) > len(list) {
		return plan, fmt.Errorf(
			"database schema version %d is newer than version %d; downgrades are not supported; restore the pre-upgrade backup",
			len(records),
			len(list),
		)
	}
	for i, record := range records {
		if record.version != i+1 || record.lineage != schemaLineage || record.checksum != checksum(list[i]) {
			return plan, fmt.Errorf("schema history mismatch at version %d; refuse to change this database", record.version)
		}
	}
	plan.Current = len(records)
	for _, step := range list[len(records):] {
		plan.Pending = append(plan.Pending, step.version)
	}
	return plan, nil
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

type schemaExec func(context.Context, string, ...interface{}) error

// applySchema runs only inside an ownership-protected transaction. Startup may initialize
// an empty database, but an existing schema requires the offline upgrade command.
func applySchema(ctx context.Context, exec schemaExec, list []migration, plan SchemaPlan, upgrade bool) error {
	if len(plan.Pending) == 0 {
		return nil
	}
	if !plan.Empty && !upgrade {
		return fmt.Errorf(
			"database schema %d requires an offline upgrade to %d; stop the console and run --schema=upgrade --backup=<new-file>",
			plan.Current,
			plan.Supported,
		)
	}
	if plan.Empty {
		if err := exec(ctx, metadataDDL); err != nil {
			return fmt.Errorf("create schema history: %w", err)
		}
	}
	for _, step := range list[plan.Current:] {
		if err := exec(ctx, step.statements); err != nil {
			return fmt.Errorf("apply schema migration %d: %w", step.version, err)
		}
		if err := exec(
			ctx,
			"INSERT INTO console_schema_migration(version, lineage, checksum) VALUES ($1, $2, $3)",
			step.version,
			schemaLineage,
			checksum(step),
		); err != nil {
			return fmt.Errorf("record schema migration %d: %w", step.version, err)
		}
	}
	return nil
}
