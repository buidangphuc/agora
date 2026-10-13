package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	pgx "github.com/jackc/pgx/v5"
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

	// The pool's background warm-up and health checks use the ctx given to
	// NewWithConfig, so it must outlive this call; Close stops them.
	pool, err := pgxpool.NewWithConfig(context.WithoutCancel(ctx), cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// OpenOrLocal is Open with a local-only fallback. It returns the pool when the
// database is reachable. When it is not, it returns (nil, nil) and logs a
// warning if rt.IsLocal(), so the caller can use an in-memory store; in any
// other environment it returns the error. A nil logger means slog.Default().
func OpenOrLocal(ctx context.Context, c config.Postgres, rt config.Runtime, logger *slog.Logger) (*pgxpool.Pool, error) {
	pool, err := Open(ctx, c)
	if err == nil {
		return pool, nil
	}
	if !rt.IsLocal() {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger.WarnContext(ctx, "database unreachable, continuing without it", "err", err)
	return nil, nil //nolint:nilnil // contract: a nil pool means "run without a database"
}

// Checker returns a health check that pings the pool. It matches healthx.Check.
func Checker(pool *pgxpool.Pool) func(context.Context) error {
	return pool.Ping
}

// rollbackTimeout bounds the rollback that releases the connection when fn
// fails, panics or exits its goroutine.
const rollbackTimeout = 5 * time.Second

// WithTx runs fn in a transaction: commit when fn returns nil, rollback
// otherwise. A deferred rollback also runs when fn panics or calls
// runtime.Goexit, so the connection always returns to the pool; a panic
// propagates. The rollback uses a context that survives the cancellation of ctx
// and is bounded by a timeout, so a cancelled request still releases its
// connection and a dead server cannot block the caller forever.
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		rbErr := tx.Rollback(rbCtx)
		// A failed rollback must not hide the error that caused it; on a panic or
		// Goexit there is no error to join to and the unwinding continues.
		if rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) && err != nil {
			err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	committed = true
	return nil
}
