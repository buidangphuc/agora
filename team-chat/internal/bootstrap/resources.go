package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/buidangphuc/team-chat/internal/config"
	"github.com/buidangphuc/team-chat/internal/events"
)

type Resources struct {
	Pool   *pgxpool.Pool
	Health *health.Server
	// Producer feeds the outbox relayer; nil when Kafka is disabled or unreachable.
	Producer *events.KafkaProducer
}

func OpenResources(ctx context.Context, cfg *config.Settings, logger *slog.Logger) (*Resources, error) {
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

	if cfg.Events.KafkaEnabled {
		prod, err := events.NewKafkaProducer(cfg.KafkaBrokers())
		if err != nil {
			logger.Warn("connect kafka failed, chat.events relayer disabled", slog.Any("err", err))
		} else {
			res.Producer = prod
			logger.Info("connected to kafka for chat events", slog.String("topic", cfg.Events.ChatTopic))
		}
	}

	return res, nil
}

func CloseResources(ctx context.Context, res *Resources) error {
	if res.Producer != nil {
		res.Producer.Close()
	}
	if res.Pool != nil {
		res.Pool.Close()
	}
	return nil
}
