package consumer

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
)

const (
	sweepBatch = 100
	retention  = 30 * 24 * time.Hour
)

type Sweeper struct {
	pool *pgxpool.Pool
	cfg  Config
}

func NewSweeper(pool *pgxpool.Pool, cfg Config) *Sweeper {
	return &Sweeper{pool: pool, cfg: cfg}
}

func (s *Sweeper) Sweep(ctx context.Context) (int, error) {
	total := 0
	for {
		n, err := s.sweepBatch(ctx)
		total += n
		if err != nil || n < sweepBatch {
			return total, err
		}
	}
}

func (s *Sweeper) sweepBatch(ctx context.Context) (int, error) {
	now := s.cfg.now()
	issued := 0
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT user_id FROM new_user_voucher_state
			WHERE issued_at IS NULL AND registered AND has_code AND verdict = '' AND registered_at <= $1
			ORDER BY registered_at LIMIT $2 FOR UPDATE SKIP LOCKED`, now.Add(-s.cfg.VerdictTimeout), sweepBatch)
		if err != nil {
			return fmt.Errorf("select timed-out users: %w", err)
		}
		users, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("read timed-out users: %w", err)
		}
		for _, u := range users {
			if err := issue(ctx, tx, s.cfg, u, s.cfg.WelcomeAmount, now); err != nil {
				return err
			}
		}
		issued = len(users)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return issued, nil
}

func (s *Sweeper) Run(ctx context.Context) error {
	interval := s.cfg.SweepInterval
	if interval <= 0 {
		interval = defaultSweepInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		n, err := s.Sweep(ctx)
		switch {
		case ctx.Err() != nil:
			return nil
		case err != nil:
			s.cfg.logger().ErrorContext(ctx, "new-user verdict sweep failed", "err", err)
		case n > 0:
			s.cfg.logger().InfoContext(ctx, "new-user vouchers issued after verdict timeout", "issued", n)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func Purge(ctx context.Context, pool *pgxpool.Pool, now time.Time) (int64, error) {
	cutoff := now.Add(-retention)
	var total int64
	err := postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `DELETE FROM promotion_processed_events WHERE processed_at < $1`, cutoff)
		if err != nil {
			return fmt.Errorf("purge processed events: %w", err)
		}
		total += ct.RowsAffected()
		ct, err = tx.Exec(ctx, `DELETE FROM new_user_voucher_state WHERE issued_at IS NOT NULL AND issued_at < $1`, cutoff)
		if err != nil {
			return fmt.Errorf("purge issued voucher state: %w", err)
		}
		total += ct.RowsAffected()
		return nil
	})
	return total, err
}
