package store

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// isolatedDatabase returns a constructor for repositories in a schema that exists only for
// this test. It skips without an isolated console_test database.
func isolatedDatabase(t *testing.T) func() *Postgres {
	t.Helper()
	dsn := isolatedDSN(t)
	return func() *Postgres {
		t.Helper()
		repository, err := Open(context.Background(), dsn, 7*24*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return repository
	}
}

// isolatedDSN returns a connection URL whose search path is a schema that exists only for
// this test. It skips without an isolated console_test database.
func isolatedDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("CONSOLE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CONSOLE_TEST_DATABASE_URL to an isolated console_test database")
	}
	endpoint, err := url.Parse(dsn)
	if err != nil || endpoint.Path != "/console_test" {
		t.Fatal("integration test requires an isolated database named console_test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Close)
	name := fmt.Sprintf("store_test_%d", time.Now().UnixNano())
	schema := pgx.Identifier{name}.Sanitize()
	if _, err := owner.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = owner.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	query := endpoint.Query()
	query.Set("search_path", name)
	endpoint.RawQuery = query.Encode()
	return endpoint.String()
}
