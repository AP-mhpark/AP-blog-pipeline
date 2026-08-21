// Package store is the pgx-based DB access layer for the pipeline schema
// (apps/api/migrations/0001_init_schema.up.sql). No ORM: queries are plain
// SQL, errors are returned rather than panicked (see root CLAUDE.md coding
// style).
//
// UUID handling: pgx v5 exchanges uuid columns in binary by default, so
// scanning one straight into a Go string produces garbage. Rather than pull
// in pgtype.UUID or a third-party adapter, every SELECT casts id columns to
// text (`id::text`) so they scan cleanly into plain strings; INSERT/WHERE
// parameters bind Go strings directly, which works because Postgres accepts
// text-formatted UUIDs as input to a uuid column.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a lookup by ID matches no row.
var ErrNotFound = errors.New("not found")

// Store wraps a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// New connects to Postgres using connString (see DATABASE_URL in
// apps/api/.env.example).
func New(ctx context.Context, connString string) (*Store, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

// Close releases all pooled connections.
func (s *Store) Close() {
	s.pool.Close()
}
