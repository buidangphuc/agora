package pgx

import (
	"context"
	"errors"
	"fmt"
	"time"

	jpgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

const pingTimeout = 5 * time.Second

// Open creates the pool and pings it, because pgxpool.New alone connects lazily.
// It returns an error, with no pool, when the database is not reachable within 5s.
func Open(ctx context.Context, c config.Postgres) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(c.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if c.MaxConns > 0 {
		cfg.MaxConns = c.MaxConns
	}

	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Checker returns a health check that pings the pool. It matches healthx.Check.
func Checker(pool *pgxpool.Pool) func(context.Context) error {
	return pool.Ping
}

// WithTx runs fn in a transaction: commit when fn returns nil, rollback otherwise.
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(jpgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		// A failed rollback must not hide the error that caused it.
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, jpgx.ErrTxClosed) {
			return errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
