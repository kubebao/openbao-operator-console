package store

import (
	"context"
	"fmt"
	"github.com/kubebao/openbao-operator-console/internal/localauth"
	"github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/view"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testRepository interface {
	settings.Repository
	localauth.Repository
	view.Observations
	Apply(context.Context, Batch) error
	RetainConnections(context.Context, []string) error
	Prune(context.Context, time.Time) error
	Close()
}

func storageBackends(t *testing.T, test func(*testing.T)) {
	t.Helper()
	t.Run("sqlite", test)
	t.Run("postgres", test)
}

func isolatedDatabase(t *testing.T) func() testRepository {
	return isolatedRepository(t, 7*24*time.Hour)
}

func isolatedRepository(t *testing.T, retention time.Duration) func() testRepository {
	t.Helper()
	if !strings.HasSuffix(t.Name(), "/postgres") {
		path := filepath.Join(t.TempDir(), "console.db")
		return func() testRepository {
			t.Helper()
			p, err := OpenSQLite(context.Background(), path, retention)
			if err != nil {
				t.Fatal(err)
			}
			return p
		}
	}
	dsn := isolatedDSN(t)
	return func() testRepository {
		t.Helper()
		p, err := Open(context.Background(), dsn, retention)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
}

// execFixture inserts approved test projections using each backend's timestamp encoding.
func execFixture(ctx context.Context, p testRepository, query string, args ...interface{}) error {
	switch p := p.(type) {
	case *Postgres:
		_, err := p.pool.Exec(ctx, query, args...)
		return err
	case *SQLite:
		for i, arg := range args {
			if at, ok := arg.(time.Time); ok {
				args[i] = at.UnixMicro()
			}
		}
		_, err := p.db.ExecContext(ctx, query, args...)
		return err
	default:
		return fmt.Errorf("unsupported fixture backend")
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
