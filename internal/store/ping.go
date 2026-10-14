package store

import "context"

// Ping reports whether the database accepts connections. It runs no query against the
// console schema, so a readiness probe stays cheap.
func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }
