// Package bootstrap owns team-identity's resource lifecycle: open its Postgres
// (identity_db) + a health server with a DB dependency check, tear down in
// reverse. Mirrors team-domain, minus events/addons.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/buidangphuc/team-engagement/internal/config"
	"github.com/buidangphuc/team-engagement/internal/engagement"
)

func OpenResources(ctx context.Context, s *config.Settings, logger *slog.Logger) (*Resources, error) {
	if !s.Database.Enabled {
		return nil, errors.New("team-identity requires DATABASE_ENABLED=true (it owns identity_db)")
	}
	res := &Resources{Health: health.NewServer()}

	pool, err := openPostgres(ctx, s)
	if err != nil {
		return nil, err
	}
	res.Pool = pool

	res.Health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	res.startDBHealthCheck(logger)
	return res, nil
}

func CloseResources(_ context.Context, res *Resources) error {
	if res == nil {
		return nil
	}
	if res.stopHealth != nil {
		res.stopHealth()
		res.stopHealth = nil
	}
	if res.Pool != nil {
		res.Pool.Close()
		res.Pool = nil
	}
	return nil
}

func openPostgres(ctx context.Context, s *config.Settings) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(s.Database.URL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	if s.Database.MaxConns > 0 {
		cfg.MaxConns = s.Database.MaxConns
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

func (res *Resources) startDBHealthCheck(logger *slog.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	res.stopHealth = cancel
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pingCtx, c := context.WithTimeout(ctx, 3*time.Second)
				err := res.Pool.Ping(pingCtx)
				c()
				if err != nil {
					logger.Warn("db health check failed", slog.Any("err", err))
					res.Health.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
					continue
				}
				res.Health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
			}
		}
	}()
}

// RunReportSweepLoop sweeps once at startup, then on every tick, until ctx ends.
// A failing run is logged and does not stop the loop.
func RunReportSweepLoop(ctx context.Context, repo engagement.ReportSweeper, retention time.Duration, ticks <-chan time.Time, now func() time.Time, logger *slog.Logger) {
	run := func() {
		n, err := engagement.SweepReports(ctx, repo, now().Add(-retention))
		if err != nil {
			logger.Error("report retention sweep failed", slog.Int64("redacted", n), slog.Any("err", err))
			return
		}
		logger.Info("report retention sweep done", slog.Int64("redacted", n))
	}
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			run()
		}
	}
}
