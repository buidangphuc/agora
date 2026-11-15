package retention

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-verification/internal/objects"
)

const defaultBatchSize = 100

type Config struct {
	Retention time.Duration
	Interval  time.Duration
	BatchSize int
}

type SweepResult struct{ Documents int }

type Sweeper struct {
	pool   *pgxpool.Pool
	objs   objects.Store
	cfg    Config
	logger *slog.Logger
}

func NewSweeper(pool *pgxpool.Pool, objs objects.Store, cfg Config, logger *slog.Logger) *Sweeper {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	return &Sweeper{pool: pool, objs: objs, cfg: cfg, logger: logger}
}

func (s *Sweeper) SweepOnce(ctx context.Context, now time.Time) (SweepResult, error) {
	cutoff := now.Add(-s.cfg.Retention)
	var res SweepResult
	var errs []error
	for {
		keys, err := s.due(ctx, cutoff)
		if err != nil {
			return res, errors.Join(append(errs, err)...)
		}
		if len(keys) == 0 {
			break
		}
		progressed := 0
		for _, k := range keys {
			if err := s.objs.Delete(ctx, k); err != nil {
				errs = append(errs, err)
				continue
			}
			if _, err := s.pool.Exec(ctx, `UPDATE kyc_documents SET deleted_at = $2 WHERE object_key = $1 AND deleted_at IS NULL`, k, now); err != nil {
				errs = append(errs, err)
				continue
			}
			progressed++
		}
		res.Documents += progressed
		if progressed == 0 || len(keys) < s.cfg.BatchSize {
			break
		}
	}
	if len(errs) > 0 {
		s.logger.WarnContext(ctx, "kyc retention sweep incomplete", "failures", len(errs), "deleted", res.Documents)
	}
	return res, errors.Join(errs...)
}

func (s *Sweeper) due(ctx context.Context, cutoff time.Time) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.object_key
		FROM kyc_documents d
		JOIN kyc_submissions s ON s.id = d.submission_id
		WHERE d.deleted_at IS NULL
		  AND ((s.status = 'REJECTED' AND s.reviewed_at <= $1) OR s.superseded_at <= $1)
		ORDER BY d.object_key
		LIMIT $2`, cutoff, s.cfg.BatchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
