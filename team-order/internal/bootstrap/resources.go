package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/buidangphuc/team-order/internal/config"
	"github.com/buidangphuc/team-order/internal/featureflags"
)

type Resources struct {
	Pool   *pgxpool.Pool
	Health *health.Server
	Flags  *featureflags.Client
}

func InitResources(ctx context.Context, cfg *config.Settings, logger *slog.Logger) (*Resources, error) {
	res := &Resources{
		Health: health.NewServer(),
	}
	res.Health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	if cfg.Database.Enabled {
		poolCfg, err := pgxpool.ParseConfig(cfg.Database.URL)
		if err != nil {
			return nil, fmt.Errorf("parse db url: %w", err)
		}
		poolCfg.MaxConns = cfg.Database.MaxConns

		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		pool, err := pgxpool.NewWithConfig(pingCtx, poolCfg)
		if err != nil {
			return nil, fmt.Errorf("connect db pool: %w", err)
		}
		if err := pool.Ping(pingCtx); err != nil {
			pool.Close()
			return nil, fmt.Errorf("ping db: %w", err)
		}
		res.Pool = pool
		logger.Info("connected to postgres", slog.String("db", poolCfg.ConnConfig.Database))
	}

	// Feature flags: open the OpenFeature/Flipt client next to the pgx pool. If
	// Flipt is disabled or unreachable, degrade to the fail-open default client
	// so boot is never blocked (a flag-system outage must not stop the service).
	flags, err := featureflags.New(ctx, featureflags.Config{
		Enabled:       cfg.FeatureFlags.Enabled,
		FliptAddr:     cfg.FeatureFlags.FliptAddr,
		EvalTimeoutMS: cfg.FeatureFlags.EvalTimeoutMS,
	}, logger)
	if err != nil {
		logger.Warn("feature flags unavailable; failing open to defaults", slog.Any("err", err))
		flags = featureflags.Disabled(logger)
	}
	res.Flags = flags

	return res, nil
}

func CloseResources(ctx context.Context, res *Resources) error {
	if res.Flags != nil {
		_ = res.Flags.Close(ctx)
	}
	if res.Pool != nil {
		res.Pool.Close()
	}
	return nil
}
