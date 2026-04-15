package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	db "github.com/buildhive/buildhive/internal/store/db"
)

// Store wraps sqlc Queries with a connection pool.
type Store struct {
	*db.Queries
	pool *pgxpool.Pool
}

// New opens a pgx connection pool and returns a Store.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	return &Store{Queries: db.New(sqlDB), pool: pool}, nil
}

// Close releases all pool connections.
func (s *Store) Close() {
	s.pool.Close()
}
