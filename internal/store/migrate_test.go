package store

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kubebao/openbao-operator-console/internal/localauth"
)

type schemaHarness struct {
	open     func([]migration) (testRepository, error)
	maintain func(string, string, []migration) (SchemaPlan, error)
	list     []migration
}

func schemaDatabase(t *testing.T) schemaHarness {
	t.Helper()
	ctx := context.Background()
	if strings.HasSuffix(t.Name(), "/postgres") {
		dsn := isolatedDSN(t)
		return schemaHarness{
			open: func(list []migration) (testRepository, error) { return open(ctx, dsn, time.Hour, list) },
			maintain: func(action, backup string, list []migration) (SchemaPlan, error) {
				return maintainPostgres(ctx, dsn, action, backup, list)
			},
			list: migrations,
		}
	}
	path := filepath.Join(t.TempDir(), "console.db")
	return schemaHarness{
		open: func(list []migration) (testRepository, error) { return openSQLite(ctx, path, time.Hour, list) },
		maintain: func(action, backup string, list []migration) (SchemaPlan, error) {
			return maintainSQLite(ctx, path, action, backup, list)
		},
		list: sqliteMigrations,
	}
}

func TestSchemaInitializationOwnershipAndRestart(t *testing.T) {
	storageBackends(t, func(t *testing.T) {
		h := schemaDatabase(t)
		p, err := h.open(h.list)
		if err != nil {
			t.Fatal(err)
		}
		if other, err := h.open(h.list); err == nil {
			other.Close()
			t.Fatal("a second owner was accepted")
		}
		plan, err := h.maintain("inspect", "", h.list)
		if err != nil || plan.Current != 1 || plan.Empty || len(plan.Pending) != 0 {
			t.Fatalf("plan: %+v, %v", plan, err)
		}
		p.Close()
		p, err = h.open(h.list)
		if err != nil {
			t.Fatal(err)
		}
		p.Close()
	})
}

func TestSchemaUpgradeRequiresOfflineBackupAndRollsBackFailure(t *testing.T) {
	storageBackends(t, schemaUpgradeRollback)
}

func schemaUpgradeRollback(t *testing.T) {
	h := schemaDatabase(t)
	first := []migration{{version: 1, statements: "CREATE TABLE migration_probe (id integer PRIMARY KEY)"}}
	p, err := h.open(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := execFixture(context.Background(), p, "INSERT INTO migration_probe(id) VALUES (42)"); err != nil {
		t.Fatal(err)
	}
	p.Close()
	upgraded := append(
		slices.Clone(first),
		migration{version: 2, statements: "ALTER TABLE migration_probe ADD COLUMN label text NOT NULL DEFAULT 'kept'"},
	)
	if p, err := h.open(upgraded); err == nil {
		p.Close()
		t.Fatal("startup silently upgraded state")
	}
	plan, err := h.maintain("inspect", "", upgraded)
	if err != nil || !slices.Equal(plan.Pending, []int{2}) {
		t.Fatalf("upgrade plan: %+v %v", plan, err)
	}
	if _, err := h.maintain("upgrade", "", upgraded); err == nil {
		t.Fatal("upgrade without backup succeeded")
	}
	// A failed chain rolls back both earlier DDL and migration history.
	broken := append(slices.Clone(upgraded), migration{version: 3, statements: "ALTER TABLE missing_table ADD COLUMN label text"})
	failedBackup := filepath.Join(t.TempDir(), "before-failure")
	if _, err := h.maintain("upgrade", failedBackup, broken); err == nil {
		t.Fatal("invalid migration succeeded")
	}
	p, err = h.open(first)
	if err != nil {
		t.Fatal(err)
	}
	// The rolled-back column can be added again; retained data still satisfies a CHECK.
	if err := execFixture(
		context.Background(),
		p,
		"ALTER TABLE migration_probe ADD COLUMN label text NOT NULL DEFAULT 'rollback' CHECK(label = 'rollback' AND id = 42)",
	); err != nil {
		t.Fatal("failed upgrade retained DDL or lost data", err)
	}
	if err := execFixture(context.Background(), p, "ALTER TABLE migration_probe DROP COLUMN label"); err != nil {
		t.Fatal(err)
	}
	p.Close()
	backup := filepath.Join(t.TempDir(), "before-upgrade")
	plan, err = h.maintain("upgrade", backup, upgraded)
	if err != nil || plan.Current != 2 || len(plan.Pending) != 0 {
		t.Fatalf("upgrade: %+v %v", plan, err)
	}
	info, err := os.Stat(backup)
	if err != nil || info.Size() == 0 || info.Mode().Perm() != 0o600 {
		t.Fatal("missing or insecure backup", err)
	}
	p, err = h.open(upgraded)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := execFixture(context.Background(), p, "INSERT INTO migration_probe(id,label) VALUES (43,'new')"); err != nil {
		t.Fatal(err)
	}
	p.Close()
	if old, err := h.open(first); err == nil {
		old.Close()
		t.Fatal("older binary accepted newer state")
	} else if !strings.Contains(err.Error(), "newer than") {
		t.Fatal("wrong downgrade failure", err)
	}
}

func TestSchemaHistoryTamperingAndLegacyDatabaseAreRefused(t *testing.T) {
	storageBackends(t, func(t *testing.T) {
		for _, query := range []string{
			"UPDATE console_schema_migration SET checksum = 'changed'",
			"UPDATE console_schema_migration SET lineage = 'old-development'",
			"DROP TABLE console_schema_migration",
			"ALTER TABLE console_schema_migration DROP COLUMN checksum",
			"DELETE FROM console_schema_migration",
		} {
			t.Run(query, func(t *testing.T) {
				// Keep the backend suffix visible to the factory.
				backend := "sqlite"
				if strings.Contains(t.Name(), "/postgres/") {
					backend = "postgres"
				}
				t.Run(backend, func(t *testing.T) {
					h := schemaDatabase(t)
					p, err := h.open(h.list)
					if err != nil {
						t.Fatal(err)
					}
					if err := execFixture(context.Background(), p, query); err != nil {
						t.Fatal(err)
					}
					p.Close()
					if p, err := h.open(h.list); err == nil {
						p.Close()
						t.Fatal("unrecognized history was accepted")
					}
				})
			})
		}
	})
}

func TestMigrationListMustBeConsecutive(t *testing.T) {
	for _, list := range [][]migration{nil, {{version: 1}, {version: 3}}, {{version: 2}}} {
		if err := validMigrations(list); err == nil {
			t.Fatal("invalid migration list accepted")
		}
	}
	if err := validMigrations(migrations); err != nil {
		t.Fatal(err)
	}
	_, err := planSchema(migrations, []schemaRecord{{version: 2, lineage: schemaLineage, checksum: checksum(migrations[0])}}, false)
	if err == nil {
		t.Fatal("nonconsecutive history accepted")
	}
}

func TestSQLiteBackupIncludesWALAndRestoresDurableState(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "source.db")
	p, err := OpenSQLite(ctx, path, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("encrypted-settings-fixture")
	if _, err := p.SaveSettings(ctx, 0, payload, "admin", "initial", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := p.db.ExecContext(ctx, "PRAGMA wal_autocheckpoint=0"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SaveSettings(ctx, 1, []byte("latest-wal-ciphertext"), "admin", "change", nil, nil); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "restore.db")
	if err := sqliteBackup(ctx, p.db, backup, sqliteMigrations); err != nil {
		t.Fatal(err)
	}
	if err := sqliteBackup(ctx, p.db, backup, sqliteMigrations); !errors.Is(err, os.ErrExist) {
		t.Fatal("existing backup was overwritten", err)
	}
	p.Close()
	restored, err := OpenSQLite(ctx, backup, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	record, err := restored.LoadSettings(ctx)
	if err != nil || record.Revision != 2 || string(record.Payload) != "latest-wal-ciphertext" {
		t.Fatalf("restored stale state: %+v %v", record, err)
	}
}

func TestPostgresBackupRestoresConfigurationAndCredentials(t *testing.T) {
	ctx := context.Background()
	dsn := isolatedDSN(t)
	p, err := Open(ctx, dsn, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.SaveSettings(ctx, 0, []byte("ciphertext-before-backup"), "admin", "initial", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := p.CreateLocalCredential(
		ctx,
		localauth.Record{AccountID: "admin", Revision: 1, Payload: []byte("credential-before-backup")},
	); err != nil {
		t.Fatal(err)
	}
	p.Close()
	archive := filepath.Join(t.TempDir(), "state.dump")
	if _, err := MaintainPostgres(ctx, dsn, "backup", archive); err != nil {
		t.Fatal(err)
	}
	p, err = Open(ctx, dsn, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.SaveSettings(ctx, 1, []byte("later-ciphertext"), "admin", "change", nil, nil); err != nil {
		t.Fatal(err)
	}
	p.Close()
	endpoint, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := endpoint.Query()
	query.Del("search_path")
	endpoint.RawQuery = query.Encode()
	endpoint.User = url.User(endpoint.User.Username())
	restore := exec.CommandContext(ctx, "pg_restore", "--clean", "--if-exists", "--no-owner", "--dbname", endpoint.String(), archive)
	restore.Env = append(os.Environ(), "PGPASSWORD="+config.Password)
	if output, err := restore.CombinedOutput(); err != nil {
		t.Fatal("restore fixture backup", err, string(output))
	}
	p, err = Open(ctx, dsn, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	record, err := p.LoadSettings(ctx)
	if err != nil || record.Revision != 1 || string(record.Payload) != "ciphertext-before-backup" {
		t.Fatal("restore lost configuration", err)
	}
	credential, err := p.GetLocalCredential(ctx, "admin")
	if err != nil || credential.Revision != 1 || string(credential.Payload) != "credential-before-backup" {
		t.Fatal("restore lost credential", err)
	}
}

func TestUnrecognizedViewIsNotAnEmptyDatabase(t *testing.T) {
	storageBackends(t, func(t *testing.T) {
		h := schemaDatabase(t)
		p, err := h.open([]migration{{version: 1, statements: "CREATE VIEW existing_view AS SELECT 1 AS id"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := execFixture(context.Background(), p, "DROP TABLE console_schema_migration"); err != nil {
			t.Fatal(err)
		}
		p.Close()
		if p, err := h.open(h.list); err == nil {
			p.Close()
			t.Fatal("a database containing a view was treated as empty")
		}
	})
}

func TestSQLiteInspectionDoesNotInitializeMissingStateAndRejectsAliases(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "missing", "console.db")
	if _, err := MaintainSQLite(ctx, path, "unknown", ""); err == nil {
		t.Fatal("unknown schema action accepted")
	}
	if _, err := MaintainSQLite(ctx, path, "inspect", ""); err == nil {
		t.Fatal("missing database was inspected successfully")
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("inspection created a state directory", err)
	}
	path = filepath.Join(directory, "console.db")
	p, err := OpenSQLite(ctx, path, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	p.Close()
	alias := filepath.Join(directory, "alias.db")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if p, err := OpenSQLite(ctx, alias, time.Hour); err == nil {
		p.Close()
		t.Fatal("hard link bypassed ownership")
	}
}
