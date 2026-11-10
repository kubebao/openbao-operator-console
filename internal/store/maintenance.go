package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
)

// MaintainSQLite inspects without creating state, or holds exclusive ownership while
// backing up and upgrading an existing database. A backup path must be unused.
func MaintainSQLite(ctx context.Context, path, action, backup string) (SchemaPlan, error) {
	return maintainSQLite(ctx, path, action, backup, sqliteMigrations)
}

func maintainSQLite(ctx context.Context, path, action, backup string, list []migration) (SchemaPlan, error) {
	if err := validateMaintenance(action); err != nil {
		return SchemaPlan{}, err
	}
	var lock *os.File
	var err error
	if action != "inspect" {
		lock, path, err = lockSQLite(path)
		if err != nil {
			return SchemaPlan{}, err
		}
		defer func() { _ = lock.Close() }()
	}
	db, err := sqliteDB(path, action == "inspect")
	if err != nil {
		return SchemaPlan{}, err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: action == "inspect"})
	if err != nil {
		return SchemaPlan{}, err
	}
	plan, err := sqlitePlan(ctx, tx, list)
	_ = tx.Rollback()
	if err != nil || action == "inspect" {
		return plan, err
	}
	if plan.Empty {
		return plan, errors.New("empty database; start the console to initialize it")
	}
	if action == "upgrade" && len(plan.Pending) == 0 {
		return plan, nil
	}
	if err := sqliteBackup(ctx, db, backup, list); err != nil {
		return plan, err
	}
	if action == "backup" {
		return plan, nil
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		return plan, err
	}
	defer func() { _ = tx.Rollback() }()
	// Recheck inside the transaction before executing any DDL.
	plan, err = sqlitePlan(ctx, tx, list)
	if err != nil {
		return plan, err
	}
	if err := applySQLiteSchema(ctx, tx, list, plan, true); err != nil {
		return plan, err
	}
	if err := sqliteIntegrity(ctx, tx); err != nil {
		return plan, err
	}
	if err := tx.Commit(); err != nil {
		return plan, err
	}
	plan.Current = plan.Supported
	plan.Pending = []int{}
	return plan, nil
}

type integrityReader interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func sqliteIntegrity(ctx context.Context, db integrityReader) error {
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return errors.New("SQLite integrity check failed")
	}
	return nil
}

func reserveBackup(path string) error {
	if path == "" {
		return errors.New("--backup must name a new backup file")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("reserve backup: %w", err)
	}
	return file.Close()
}

func sqliteBackup(ctx context.Context, db *sql.DB, path string, list []migration) error {
	if err := reserveBackup(path); err != nil {
		return err
	}
	// VACUUM INTO includes committed WAL content and produces a standalone database.
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("create SQLite backup: %w", err)
	}
	backup, err := sqliteDB(path, true)
	if err != nil {
		return err
	}
	defer func() { _ = backup.Close() }()
	if err := sqliteIntegrity(ctx, backup); err != nil {
		return err
	}
	tx, err := backup.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := sqlitePlan(ctx, tx, list); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		return errors.New("backup foreign key validation failed")
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return syncBackup(path)
}

// syncBackup verifies that the backup is readable and flushes it before DDL can run.
func syncBackup(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if _, err := io.Copy(io.Discard, file); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
}

// MaintainPostgres takes the same session ownership lock as the server. pg_dump's
// exported snapshot makes the pre-upgrade archive consistent. pg_restore decodes its
// complete SQL stream; operators must separately qualify restoration with their backup tooling.
func MaintainPostgres(ctx context.Context, dsn, action, backup string) (SchemaPlan, error) {
	return maintainPostgres(ctx, dsn, action, backup, migrations)
}

func maintainPostgres(ctx context.Context, dsn, action, backup string, list []migration) (SchemaPlan, error) {
	if err := validateMaintenance(action); err != nil {
		return SchemaPlan{}, err
	}
	pool, err := postgresPool(ctx, dsn, action != "inspect")
	if err != nil {
		return SchemaPlan{}, err
	}
	defer pool.Close()
	tx, err := pool.BeginTx(ctx, repeatableRead())
	if err != nil {
		return SchemaPlan{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	plan, err := postgresPlan(ctx, tx, list)
	if err != nil || action == "inspect" {
		return plan, err
	}
	if plan.Empty {
		return plan, errors.New("empty database; start the console to initialize it")
	}
	if action == "upgrade" && len(plan.Pending) == 0 {
		return plan, nil
	}
	if err := reserveBackup(backup); err != nil {
		return plan, err
	}
	var snapshot, schema string
	if err := tx.QueryRow(ctx, "SELECT pg_export_snapshot(), current_schema()").Scan(&snapshot, &schema); err != nil {
		return plan, err
	}
	// libpq does not accept pgx's search_path URI parameter. The archive is scoped
	// to the current schema, and the exported snapshot binds it to this inspection.
	endpoint, err := url.Parse(dsn)
	if err != nil {
		return plan, errors.New("invalid PostgreSQL URL")
	}
	query := endpoint.Query()
	query.Del("search_path")
	query.Del("password")
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return plan, errors.New("invalid PostgreSQL connection configuration")
	}
	if endpoint.User != nil {
		endpoint.User = url.User(endpoint.User.Username())
	}
	endpoint.RawQuery = query.Encode()
	// The executable is fixed; operator paths are separate argv values.
	//nolint:gosec // G204: no shell or executable selection is involved.
	dump := exec.CommandContext(
		ctx,
		"pg_dump",
		"--dbname", endpoint.String(),
		"--format=custom",
		"--no-owner",
		"--no-privileges",
		"--schema",
		`"`+strings.ReplaceAll(schema, `"`, `""`)+`"`,
		"--snapshot",
		snapshot,
		"--file",
		backup,
	)
	dump.Env = append(os.Environ(), "PGPASSWORD="+config.Password)
	if err := dump.Run(); err != nil {
		return plan, errors.New("PostgreSQL backup failed; check pg_dump availability, server compatibility, and connection access")
	}
	//nolint:gosec // G204: fixed executable with an operator-supplied archive path.
	verify := exec.CommandContext(ctx, "pg_restore", "--file=/dev/null", backup)
	if err := verify.Run(); err != nil {
		return plan, errors.New("PostgreSQL backup archive validation failed")
	}
	if err := syncBackup(backup); err != nil {
		return plan, err
	}
	if action == "backup" {
		return plan, nil
	}
	if err := tx.Rollback(ctx); err != nil {
		return plan, err
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		return plan, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	plan, err = postgresPlan(ctx, tx, list)
	if err != nil {
		return plan, err
	}
	if err := applySchema(ctx, func(ctx context.Context, query string, args ...interface{}) error {
		_, err := tx.Exec(ctx, query, args...)
		return err
	}, list, plan, true); err != nil {
		return plan, err
	}
	if err := tx.Commit(ctx); err != nil {
		return plan, err
	}
	plan.Current = plan.Supported
	plan.Pending = []int{}
	return plan, nil
}

func validateMaintenance(action string) error {
	switch action {
	case "inspect", "backup", "upgrade":
		return nil
	default:
		return errors.New("schema action must be inspect, backup, or upgrade")
	}
}
