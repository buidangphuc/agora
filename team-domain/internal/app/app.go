// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them. Read top to bottom to see what runs.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"

	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/events"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/objects"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/store"
)

const service = "team-domain"

// Options are the seams tests use; Run passes the zero value.
type Options struct {
	// NewPublisher builds the Kafka publisher when KAFKA_ENABLED=true.
	// Default: events.NewKafkaPublisher.
	NewPublisher func(brokers []string, topic string) (events.Publisher, error)
	// LogOut receives the service log. Default: os.Stdout.
	LogOut io.Writer
}

// Run wires the service and runs it until ctx ends (main cancels it on
// SIGINT/SIGTERM) or a component fails. Build order: logger, telemetry, health,
// database, publisher, domain, server. Everything opened goes on the stack;
// closers run in reverse order on shutdown and on an early failure.
//
// Shutdown order: health drains, SHUTDOWN_DRAIN_DELAY_SECONDS passes, the gRPC
// server stops, then the relayer and the sweeper stop, then the publisher and
// the pool close.
//
// Migrations are not applied here: the compose job team-domain-migrate does it.
func Run(ctx context.Context, s config.Settings) error {
	return run(ctx, s, Options{})
}

func run(ctx context.Context, s config.Settings, o Options) error {
	if !s.Postgres.Enabled {
		return errors.New("team-domain requires DATABASE_ENABLED=true (it owns the listings DB)")
	}
	out := o.LogOut
	if out == nil {
		out = os.Stdout
	}
	logger := logx.NewTo(out, s.Runtime, service)
	slog.SetDefault(logger)

	var stack lifecycle.Stack

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	stack.Push(shutdownOTel)

	health := healthx.New(healthx.DefaultInterval, logger)

	pool, err := pgx.Open(ctx, s.Postgres)
	if err != nil {
		return errors.Join(fmt.Errorf("open database: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", pgx.Checker(pool))

	// The transactional outbox lives over the same pool; each write enqueues its
	// event in its own transaction (ADR-0002) and the relayer publishes the rows.
	outbox := store.NewOutboxStore(pool)

	var publisher events.Publisher
	if s.Events.KafkaEnabled {
		newPublisher := o.NewPublisher
		if newPublisher == nil {
			newPublisher = newKafkaPublisher
		}
		publisher, err = newPublisher(s.KafkaBrokers(), s.Events.ListingTopic)
		if err != nil {
			return errors.Join(fmt.Errorf("kafka publisher: %w", err), stack.Close(ctx))
		}
		stack.Push(func(context.Context) error { publisher.Close(); return nil })
	}

	repo := store.NewPostgresListingRepository(pool)
	// Reserve / release / sweep announce ListingStockChanged on the same outbox,
	// inside their own stock-change transaction.
	repo.WithStockEvents(outbox, rpc.NewStockEventBuilder())
	svc := listing.NewListingServiceWithOutbox(repo, store.NewPgTxWriter(pool, outbox))
	svc.WithReservationTTL(s.ReservationTTL(logger))

	handler := rpc.NewListingHandler(svc, store.NewPostgresCategoryRepository(pool), objects.NewS3Storage(s.Storage)).
		WithStorefront(listing.NewStorefrontService(store.NewPostgresStorefrontRepository(pool))).
		WithBundles(listing.NewBundleService(store.NewPostgresBundleRepository(pool)))

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, handler)

	serverStopped := make(chan struct{})
	serve := grpcx.Serve(srv, s.GRPC)
	components := []lifecycle.Component{
		lifecycle.Func(func(ctx context.Context) error {
			defer close(serverStopped)
			return serve.Run(ctx)
		}),
		lifecycle.Func(health.Run),
		// Reservation sweeper (AD3): releases stock held past its TTL so a crashed
		// checkout never leaks inventory.
		afterServer(serverStopped, listing.NewReservationSweeper(svc, s.ReservationSweepInterval(logger), logger)),
	}
	// Outbox relayer: runs only when the outbox is enabled and a real producer
	// exists; with Kafka off, rows are recorded but never relayed.
	if publisher != nil && s.Outbox.Enabled {
		relayer := events.NewRelayer(outbox, publisher, logger, events.RelayerConfig{
			PollInterval: s.OutboxPollInterval(),
			BatchSize:    s.Outbox.BatchSize,
			LockSeconds:  s.Outbox.ClaimLockSeconds,
			MaxAttempts:  s.Outbox.MaxAttempts,
		})
		components = append(components, afterServer(serverStopped, relayer))
	}

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace(),
		DrainDelay:    s.DrainDelay(),
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, components...)
}

func newKafkaPublisher(brokers []string, topic string) (events.Publisher, error) {
	return events.NewKafkaPublisher(brokers, topic)
}

// afterServer runs c until the gRPC server has stopped, so in-flight RPCs that
// enqueue outbox rows are drained before the relayer and the sweeper stop.
func afterServer(serverStopped <-chan struct{}, c lifecycle.Component) lifecycle.Component {
	return lifecycle.Func(func(ctx context.Context) error {
		runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		defer cancel()
		go func() {
			<-serverStopped
			cancel()
		}()
		return c.Run(runCtx)
	})
}
