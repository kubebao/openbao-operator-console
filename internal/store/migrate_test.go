package store

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

func appliedVersions(t *testing.T, p *Postgres) []int {
	t.Helper()
	rows, err := p.pool.Query(context.Background(), "SELECT version FROM console_schema_migration ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	versions := []int{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return versions
}

func equalVersions(got []int, want ...int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestFreshDatabaseAppliesEveryMigration(t *testing.T) {
	dsn := isolatedDSN(t)
	ctx := context.Background()
	p, err := Open(ctx, dsn, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if versions := appliedVersions(t, p); !equalVersions(versions, 1, 2) || len(migrations) != 2 {
		t.Fatalf("applied versions = %v", versions)
	}
	// A restart applies nothing twice.
	again, err := Open(ctx, dsn, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	again.Close()
	if versions := appliedVersions(t, p); !equalVersions(versions, 1, 2) {
		t.Fatalf("restart changed applied versions to %v", versions)
	}
}

func TestUpgradeAppliesOnlyPendingMigrations(t *testing.T) {
	dsn := isolatedDSN(t)
	ctx := context.Background()
	first := []migration{
		{version: 1, statements: "CREATE TABLE migration_probe (id integer PRIMARY KEY)"},
	}
	p, err := open(ctx, dsn, time.Hour, first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.pool.Exec(ctx, "INSERT INTO migration_probe(id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	p.Close()
	// Re-running migration 1 would fail because its table exists.
	upgraded := append(slices.Clone(first),
		migration{version: 2, statements: "ALTER TABLE migration_probe ADD COLUMN label text NOT NULL DEFAULT 'kept'"},
		migration{version: 3, statements: "CREATE INDEX migration_probe_label ON migration_probe(label)"},
	)
	p, err = open(ctx, dsn, time.Hour, upgraded)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if versions := appliedVersions(t, p); !equalVersions(versions, 1, 2, 3) {
		t.Fatalf("applied versions = %v", versions)
	}
	var label string
	if err := p.pool.QueryRow(ctx, "SELECT label FROM migration_probe WHERE id = 1").Scan(&label); err != nil || label != "kept" {
		t.Fatalf("upgrade lost data: %v %q", err, label)
	}
}

func TestPaginationMigrationPreservesExistingHistory(t *testing.T) {
	dsn := isolatedDSN(t)
	ctx := context.Background()
	p, err := open(ctx, dsn, time.Hour, migrations[:1])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.pool.Exec(ctx, `INSERT INTO console_observation(deployment_id, connection_id, recorded_at, projection)
VALUES ('bao', 'cluster', now(), '{"resourceVersion":"kept"}')`); err != nil {
		t.Fatal(err)
	}
	p.Close()
	p, err = Open(ctx, dsn, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	rows, err := firstHistory(p, ctx, "bao")
	if err != nil || len(rows) != 1 || rows[0].Deployment.ResourceVersion != "kept" {
		t.Fatal("the pagination migration lost existing history")
	}
	for _, name := range []string{"console_observation_deployment", "console_activity_scope"} {
		var definition string
		if err := p.pool.QueryRow(ctx, "SELECT pg_get_indexdef($1::regclass)", name).Scan(&definition); err != nil ||
			!strings.Contains(definition, "id DESC") {
			t.Fatalf("pagination index %s was not upgraded: %s, %v", name, definition, err)
		}
	}
}

func TestFailedMigrationLeavesTheDatabaseUnchanged(t *testing.T) {
	dsn := isolatedDSN(t)
	ctx := context.Background()
	first := []migration{{version: 1, statements: "CREATE TABLE migration_probe (id integer PRIMARY KEY)"}}
	p, err := open(ctx, dsn, time.Hour, first)
	if err != nil {
		t.Fatal(err)
	}
	p.Close()
	broken := append(slices.Clone(first),
		migration{version: 2, statements: "CREATE TABLE migration_partial (id integer)"},
		migration{version: 3, statements: "ALTER TABLE missing_table ADD COLUMN label text"},
	)
	if _, err := open(ctx, dsn, time.Hour, broken); err == nil || !strings.Contains(err.Error(), "apply schema migration 3") {
		t.Fatalf("broken migration error = %v", err)
	}
	p, err = open(ctx, dsn, time.Hour, first)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	var partial *string
	if err := p.pool.QueryRow(ctx, "SELECT to_regclass('migration_partial')::text").Scan(&partial); err != nil || partial != nil {
		t.Fatalf("a failed upgrade kept earlier steps: %v %v", err, partial)
	}
	if versions := appliedVersions(t, p); !equalVersions(versions, 1) {
		t.Fatalf("applied versions = %v", versions)
	}
}

func TestNewerDatabaseSchemaIsRefused(t *testing.T) {
	dsn := isolatedDSN(t)
	ctx := context.Background()
	newer := []migration{
		{version: 1, statements: "CREATE TABLE migration_probe (id integer PRIMARY KEY)"},
		{version: 2, statements: "ALTER TABLE migration_probe ADD COLUMN label text"},
	}
	p, err := open(ctx, dsn, time.Hour, newer)
	if err != nil {
		t.Fatal(err)
	}
	p.Close()
	_, err = open(ctx, dsn, time.Hour, newer[:1])
	if err == nil || !strings.Contains(err.Error(), "database schema version 2 is newer than version 1") ||
		!strings.Contains(err.Error(), "downgrades are not supported") {
		t.Fatalf("older binary error = %v", err)
	}
}

func TestMigrationListMustBeConsecutive(t *testing.T) {
	for name, list := range map[string][]migration{
		"empty":     nil,
		"gap":       {{version: 1}, {version: 3}},
		"not first": {{version: 2}},
	} {
		if err := validMigrations(list); err == nil {
			t.Fatalf("%s migration list was accepted", name)
		}
	}
	if err := validMigrations(migrations); err != nil {
		t.Fatal(err)
	}
}
