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

	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/events"
	"github.com/buidangphuc/team-identity/internal/repository"
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
	res.Outbox = repository.NewOutboxStore(pool)

	// Event publisher (ADR-0002): real Kafka when enabled, else a no-op. The
	// relayer only runs with both a real producer and OUTBOX_ENABLED; with Kafka
	// off, revokes still record their outbox rows but nothing is relayed.
	if s.Events.KafkaEnabled {
		pub, err := events.NewKafkaPublisher(s.KafkaBrokers(), s.Events.IdentityTopic)
		if err != nil {
			_ = CloseResources(context.Background(), res)
			return nil, err
		}
		res.Publisher = pub
		if s.Outbox.Enabled {
			res.startRelayer(s, pub, logger)
		}
	} else {
		res.Publisher = events.NoopPublisher{}
	}

	res.Health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	res.startDBHealthCheck(logger)
	return res, nil
}

func CloseResources(_ context.Context, res *Resources) error {
	if res == nil {
		return nil
	}
	// Stop the relayer first: it produces via the publisher and reads the pool,
	// both torn down below. Wait for it to drain its current pass.
	if res.stopRelayer != nil {
		res.stopRelayer()
		if res.relayerDone != nil {
			<-res.relayerDone
			res.relayerDone = nil
		}
		res.stopRelayer = nil
	}
	if res.stopHealth != nil {
		res.stopHealth()
		res.stopHealth = nil
	}
	if res.Publisher != nil {
		res.Publisher.Close()
		res.Publisher = nil
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

// startRelayer launches the transactional-outbox relayer in its own goroutine. It
// is cancelled by CloseResources, which also waits on relayerDone so the current
// pass drains.
func (res *Resources) startRelayer(s *config.Settings, producer events.RawProducer, logger *slog.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	res.stopRelayer = cancel
	res.relayerDone = make(chan struct{})

	relayer := events.NewRelayer(res.Outbox, producer, logger, events.RelayerConfig{
		PollInterval: s.OutboxPollInterval(),
		BatchSize:    s.Outbox.BatchSize,
		LockSeconds:  s.Outbox.ClaimLockSeconds,
		MaxAttempts:  s.Outbox.MaxAttempts,
	})
	go func() {
		defer close(res.relayerDone)
		relayer.Run(ctx)
	}()
}
