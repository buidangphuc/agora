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

	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/events"
	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

// OpenResources validates requirements up front, opens core infra (Postgres),
// runs the enabled addons in order, and installs a health server with a DB
// dependency check. Mirrors team-ai's open_application_resources.
func OpenResources(ctx context.Context, s *config.Settings, logger *slog.Logger) (*Resources, error) {
	if err := validateRequirements(s); err != nil {
		return nil, err
	}
	res := &Resources{Health: health.NewServer()}

	if s.Database.Enabled {
		pool, err := openPostgres(ctx, s)
		if err != nil {
			return nil, err
		}
		res.Pool = pool
		// The transactional outbox store lives over the same pool; the service
		// enqueues into it in the same tx as each listing write.
		res.Outbox = repository.NewOutboxStore(pool)
	}

	// Event publisher (ADR-0002): real Kafka when enabled, else a no-op. It is now
	// the relayer's produce path, not the request path.
	if s.Events.KafkaEnabled {
		pub, err := events.NewKafkaPublisher(s.KafkaBrokers(), s.Events.ListingTopic)
		if err != nil {
			_ = CloseResources(context.Background(), res)
			return nil, err
		}
		res.Publisher = pub

		// Outbox relayer: drains pending rows to Kafka. Only started when the
		// outbox is enabled AND we have both a store and a real producer; with
		// Kafka disabled, rows are recorded but never relayed.
		if s.Outbox.Enabled && res.Outbox != nil {
			res.startRelayer(s, pub, logger)
		}
	} else {
		res.Publisher = events.NoopPublisher{}
	}

	for _, a := range defaultAddons() {
		if !a.Enabled(s) {
			continue
		}
		if err := a.Open(ctx, s, res); err != nil {
			_ = CloseResources(context.Background(), res) // unwind what already opened
			return nil, fmt.Errorf("addon %s open: %w", a.Name(), err)
		}
		res.addons = append(res.addons, a)
	}

	// Overall SERVING once core infra is up; the monitor flips it on DB trouble.
	res.Health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	if res.Pool != nil {
		res.startDBHealthCheck(logger)
	}
	return res, nil
}

// CloseResources tears everything down in reverse order (addons, then Postgres),
// mirroring close_application_resources.
func CloseResources(ctx context.Context, res *Resources) error {
	if res == nil {
		return nil
	}
	// Stop the reservation sweeper first — it reads the pool, torn down below.
	if res.stopSweeper != nil {
		res.stopSweeper()
		if res.sweeperDone != nil {
			<-res.sweeperDone
			res.sweeperDone = nil
		}
		res.stopSweeper = nil
	}
	// Stop the relayer next — it produces via the publisher and reads the pool,
	// both of which are torn down below. Wait for it to drain its current pass.
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
	var errs []error
	for i := len(res.addons) - 1; i >= 0; i-- {
		if err := res.addons[i].Close(ctx, res); err != nil {
			errs = append(errs, fmt.Errorf("addon %s close: %w", res.addons[i].Name(), err))
		}
	}
	res.addons = nil
	if res.Publisher != nil {
		res.Publisher.Close()
		res.Publisher = nil
	}
	if res.Pool != nil {
		res.Pool.Close()
		res.Pool = nil
	}
	return errors.Join(errs...)
}

// validateRequirements enforces cross-capability invariants, mirroring
// validate_core_resource_requirements. team-domain's core function needs its DB.
func validateRequirements(s *config.Settings) error {
	if !s.Database.Enabled {
		return errors.New("team-domain requires DATABASE_ENABLED=true (it owns the listings DB)")
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

// startRelayer launches the transactional-outbox relayer in its own goroutine,
// mirroring startDBHealthCheck's start/stop shape. It is cancelled by
// CloseResources, which also waits on relayerDone so the current pass drains.
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

// StartReservationSweeper launches the reservation sweeper (AD3 domain side) in
// its own goroutine, mirroring startRelayer's start/stop shape. It releases stock
// held by reservations past their TTL so a crashed checkout never leaks
// inventory. Called by main.go once the ListingService is wired, and only when
// Postgres is present (reservations live in the DB). A non-positive interval uses
// the sweeper's own default. Cancelled by CloseResources, which waits on
// sweeperDone.
func (res *Resources) StartReservationSweeper(svc *service.ListingService, interval time.Duration, logger *slog.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	res.stopSweeper = cancel
	res.sweeperDone = make(chan struct{})

	sweeper := service.NewReservationSweeper(svc, interval, logger)
	go func() {
		defer close(res.sweeperDone)
		sweeper.Run(ctx)
	}()
}

// startDBHealthCheck runs a background poll that flips the overall health status
// to NOT_SERVING when the DB is unreachable — the Go analogue of team-ai's
// health DependencyCheck. Cancelled by CloseResources.
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
