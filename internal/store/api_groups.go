package store

import (
	"context"
	"errors"

	"github.com/kubebao/openbao-operator-console/internal/store/database"
	"github.com/kubebao/openbao-operator-console/internal/store/sqlitedb"
)

// RetainedConnectionIDs bounds the legacy identity backfill. Connection rows exist
// while their observations or history can still be retained by the console.
func (p *Postgres) RetainedConnectionIDs(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 10001 {
		return nil, errors.New("invalid retained connection limit")
	}
	return database.New(p.pool).RetainedConnectionIDs(ctx, int32(limit))
}

func (p *SQLite) RetainedConnectionIDs(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 10001 {
		return nil, errors.New("invalid retained connection limit")
	}
	return sqlitedb.New(p.db).RetainedConnectionIDs(ctx, int64(limit))
}
