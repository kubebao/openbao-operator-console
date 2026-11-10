package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	_ "modernc.org/sqlite"
)

// SQLite stores one console's state on a local filesystem. The lock file must remain
// beside the database for its entire lifetime, including backups and upgrades.
type SQLite struct {
	db        *sql.DB
	lock      *os.File
	retention time.Duration
}

func stringList(values []string) string {
	if values == nil {
		values = []string{}
	}
	encoded, _ := json.Marshal(values) // Strings always have a JSON representation.
	return string(encoded)
}

func lockSQLite(path string) (*os.File, string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return nil, "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return nil, "", err
	}
	absolute = filepath.Join(parent, filepath.Base(absolute))
	for _, name := range []string{absolute, absolute + ".lock"} {
		info, err := os.Lstat(name)
		if err == nil && !info.Mode().IsRegular() {
			return nil, "", errors.New("SQLite database and lock must be regular files")
		}
		if err == nil {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink != 1 {
				return nil, "", errors.New("SQLite database and lock must not have hard links")
			}
		}

		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
	}
	lock, err := os.OpenFile(absolute+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, "", err
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = lock.Close()
		return nil, "", errors.New("SQLite database is owned by another console or schema command; stop it first")
	}
	return lock, absolute, nil
}

func sqliteDB(path string, readonly bool) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: path}
	query := u.Query()
	if readonly {
		query.Set("mode", "ro")
	} else {
		query.Set("mode", "rw")
		query.Set("_txlock", "immediate")
	}
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "synchronous(FULL)")
	u.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

func OpenSQLite(ctx context.Context, path string, retention time.Duration) (*SQLite, error) {
	return openSQLite(ctx, path, retention, sqliteMigrations)
}

func openSQLite(ctx context.Context, path string, retention time.Duration, list []migration) (*SQLite, error) {
	if retention <= 0 {
		return nil, errors.New("observation retention must be positive")
	}
	lock, path, err := lockSQLite(path)
	if err != nil {
		return nil, err
	}
	p := &SQLite{lock: lock, retention: retention}
	success := false
	defer func() {
		if !success {
			p.Close()
		}
	}()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	p.db, err = sqliteDB(path, false)
	if err != nil {
		return nil, err
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("open SQLite transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	plan, err := sqlitePlan(ctx, tx, list)
	if err != nil {
		return nil, err
	}
	if err := applySQLiteSchema(ctx, tx, list, plan, false); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	// Change journal mode only after the existing schema has been accepted.
	var journal string
	if err := p.db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&journal); err != nil {
		return nil, err
	}
	if journal != "wal" {
		return nil, errors.New("SQLite filesystem does not support WAL")
	}
	success = true
	return p, nil
}

func (p *SQLite) Close() {
	if p.db != nil {
		_ = p.db.Close()
	}
	if p.lock != nil {
		_ = p.lock.Close()
	}
}
func (p *SQLite) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }

func sqlitePlan(ctx context.Context, tx *sql.Tx, list []migration) (SchemaPlan, error) {
	var tables, columns int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'").
		Scan(&tables); err != nil {
		return SchemaPlan{}, err
	}
	if tables == 0 {
		return planSchema(list, nil, true)
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('console_schema_migration')
 WHERE name IN ('version','lineage','checksum')`).
		Scan(&columns); err != nil {
		return SchemaPlan{}, err
	}
	if columns != 3 {
		return SchemaPlan{}, errors.New(
			"legacy or unrecognized development database; recreate it deliberately; no automatic reset is performed",
		)
	}
	rows, err := tx.QueryContext(ctx, "SELECT version,lineage,checksum FROM console_schema_migration ORDER BY version")
	if err != nil {
		return SchemaPlan{}, err
	}
	defer func() { _ = rows.Close() }()
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

func applySQLiteSchema(ctx context.Context, tx *sql.Tx, list []migration, plan SchemaPlan, upgrade bool) error {
	err := applySchema(ctx, func(ctx context.Context, query string, args ...interface{}) error {
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	}, list, plan, upgrade)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		return errors.New("SQLite foreign key validation failed")
	}
	return rows.Err()
}
