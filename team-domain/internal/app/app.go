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

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/objects"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/store"
)

const service = "team-domain"

// Publisher is what the outbox relayer publishes through, plus the close handle
// the app registers on the stack. *kafkax.Producer satisfies it.
type Publisher interface {
	outbox.Publisher
	Close()
}

// Options are the seams tests use; Run passes the zero value.
type Options struct {
	// NewPublisher builds the Kafka publisher. Default: kafkax.NewProducer.
	NewPublisher func(sdkconfig.Kafka) (Publisher, error)
	// LogOut receives the service log. Default: os.Stdout.
	LogOut io.Writer
}

// Run wires the service and runs it until ctx ends (main cancels it on
// SIGINT/SIGTERM) or a component fails. Build order: logger, telemetry, health,
// database, publisher, domain, server. Everything opened goes on the stack;
// closers run in reverse order on shutdown and on an early failure.
//
// Shutdown order: health drains, SHUTDOWN_DRAIN_DELAY passes, the gRPC
// server stops, then the relayer and the sweeper stop, then the publisher and
// the pool close.
//
// Migrations are not applied here: the compose job team-domain-migrate does it.
func Run(ctx context.Context, s config.Settings) error {
	return run(ctx, s, Options{})
}

func run(ctx context.Context, s config.Settings, o Options) error {
	if err := s.Postgres.Validate(); err != nil {
		return fmt.Errorf("team-domain owns the listings DB: %w", err)
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

	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		return errors.Join(fmt.Errorf("health: %w", err), stack.Close(ctx))
	}

	pool, err := postgres.Open(ctx, s.Postgres)
	if err != nil {
		return errors.Join(fmt.Errorf("open database: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", postgres.Checker(pool))

	// The transactional outbox lives over the same pool; each write enqueues its
	// event in its own transaction (ADR-0002) and the relayer publishes the rows.
	outboxStore, err := outbox.NewStore(pool, store.OutboxTable)
	if err != nil {
		return errors.Join(fmt.Errorf("outbox store: %w", err), stack.Close(ctx))
	}

	newPublisher := o.NewPublisher
	if newPublisher == nil {
		newPublisher = newKafkaPublisher
	}
	publisher, err := newPublisher(s.Kafka)
	if err != nil {
		return errors.Join(fmt.Errorf("kafka publisher: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { publisher.Close(); return nil })

	repo := store.NewPostgresListingRepository(pool)
	// Reserve / release / sweep announce ListingStockChanged on the same outbox,
	// inside their own stock-change transaction.
	repo.WithStockEvents(outboxStore, rpc.NewStockEventBuilder())
	svc := listing.NewListingServiceWithOutbox(repo, store.NewPgTxWriter(pool, outboxStore))
	svc.WithReservationTTL(s.ReservationTTL(logger))

	handler := rpc.NewListingHandler(svc, store.NewPostgresCategoryRepository(pool), objects.NewS3Storage(s.Storage)).
		WithStorefront(listing.NewStorefrontService(store.NewPostgresStorefrontRepository(pool))).
		WithBundles(listing.NewBundleService(store.NewPostgresBundleRepository(pool)))

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, handler)

	// Outbox relayer: publishes the rows each write recorded (ADR-0002).
	relayer, err := outbox.NewRelayer(outboxStore, publisher, outbox.RelayerOptions{
		Topic:  s.Events.ListingTopic,
		Config: s.Outbox,
		Logger: logger,
	})
	if err != nil {
		return errors.Join(fmt.Errorf("outbox relayer: %w", err), stack.Close(ctx))
	}
	// Reservation sweeper (AD3): releases stock held past its TTL so a crashed
	// checkout never leaks inventory.
	sweeper := listing.NewReservationSweeper(svc, s.ReservationSweepInterval(logger), logger)

	// The relayer and the sweeper keep running until the gRPC server has stopped,
	// so in-flight RPCs that enqueue outbox rows are drained before they stop.
	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), relayer, sweeper), lifecycle.Func(health.Run))
}

func newKafkaPublisher(c sdkconfig.Kafka) (Publisher, error) {
	return kafkax.NewProducer(c)
}
