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
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-engagement/internal/config"
	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/events"
	"github.com/buidangphuc/team-engagement/internal/rpc"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

const service = "team-engagement"

const (
	// outboxRetention is how long published outbox rows are kept before they are
	// deleted; the cleanup runs once an hour.
	outboxRetention = 7 * 24 * time.Hour
	outboxCleanup   = time.Hour

	// produceTimeout bounds one produce, so a dead broker fails the relay pass
	// (retried with backoff) instead of hanging it.
	produceTimeout = 10 * time.Second
)

// Producer is the Kafka producer the service needs: the outbox relayer publishes
// through it and the feed consumer parks poison records with it.
// *kafkax.Producer satisfies it.
type Producer interface {
	outbox.Publisher
	consume.DLQProducer
	Close()
}

// Options are the seams tests use; Run passes the zero value.
type Options struct {
	// NewProducer builds the Kafka producer. Default: kafkax.NewProducer.
	NewProducer func(k sdkconfig.Kafka, opts ...kgo.Opt) (Producer, error)
	// LogOut receives the service log. Default: os.Stdout.
	LogOut io.Writer
}

// Run wires the service and runs it until ctx ends (main cancels it on
// SIGINT/SIGTERM) or a component fails. Build order: logger, telemetry, health,
// database, upstream client, domain and handler, server, Kafka. Everything opened
// goes on the stack; closers run in reverse order on shutdown and on an early
// failure.
//
// The database is required in every ENV: an unreachable one fails boot before any
// port opens.
//
// Shutdown order: health drains, SHUTDOWN_DRAIN_DELAY_SECONDS passes, the gRPC
// server stops, then the outbox relayer, the outbox cleanup and the listing.events
// consumer stop (lifecycle.StopAfter), then the Kafka clients, the upstream
// connection and the pool close.
//
// Migrations are not applied here: the compose job team-engagement-migrate does it.
func Run(ctx context.Context, s config.Settings) error {
	return run(ctx, s, Options{})
}

func run(ctx context.Context, s config.Settings, o Options) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("team-engagement settings: %w", err)
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
		return errors.Join(fmt.Errorf("team-engagement owns engagement_db: open database: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", pgx.Checker(pool))

	// An empty UPSTREAM_ORDER_ADDR yields a nil client: verified-purchase
	// enrichment is off and CreateDispute fails closed.
	orderClient, err := upstream.NewOrderClient(s.Upstream.OrderAddr, time.Duration(s.Upstream.CallTimeoutSeconds*float64(time.Second)))
	if err != nil {
		return errors.Join(fmt.Errorf("dial team-order: %w", err), stack.Close(ctx))
	}
	var reviewVerifier engagement.OrderVerifier
	var handlerOpts []rpc.Option
	if orderClient != nil {
		stack.Push(func(context.Context) error { return orderClient.Close() })
		reviewVerifier = orderClient
		handlerOpts = append(handlerOpts, rpc.WithOrderParties(orderClient))
		logger.InfoContext(ctx, "verified-purchase enrichment enabled", "order_addr", s.Upstream.OrderAddr)
	} else {
		logger.WarnContext(ctx, "UPSTREAM_ORDER_ADDR unset; verified-purchase enrichment disabled and CreateDispute fails closed")
	}

	engagementRepo := store.NewPostgresRepository(pool)
	h := rpc.NewEngagementHandler(
		engagementRepo,
		engagement.NewReviewService(store.NewPostgresReviewRepository(pool), reviewVerifier, logger),
		engagement.NewQAService(store.NewPostgresQARepository(pool), logger),
		engagement.NewDisputeService(store.NewPostgresDisputeRepository(pool), logger),
		engagement.NewCollectionService(store.NewPostgresCollectionRepository(pool), logger),
		handlerOpts...)

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, h)

	// One producer serves the outbox relayer and the consumer's DLQ.
	newProducer := o.NewProducer
	if newProducer == nil {
		newProducer = newKafkaProducer
	}
	producer, err := newProducer(s.Kafka, kgo.RecordDeliveryTimeout(produceTimeout))
	if err != nil {
		return errors.Join(fmt.Errorf("kafka producer: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { producer.Close(); return nil })

	consumerClient, err := events.NewListingClient(s.Kafka, s.Events.ConsumerGroup, s.Events.ListingTopic)
	if err != nil {
		return errors.Join(fmt.Errorf("kafka consumer: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { consumerClient.Close(); return nil })

	// Workers stop only after the gRPC server has returned, so in-flight RPCs that
	// enqueue outbox rows are drained first. A fatal consumer error (a record that
	// can neither be handled nor parked to the DLQ) stops the service, so the
	// orchestrator restarts it and the group resumes from its last commit.
	workers := []lifecycle.Component{
		// Publishes engagement facts to engagement.events.
		outbox.NewRelayer(outbox.NewStore(pool, store.OutboxTable), producer, outbox.RelayerOptions{
			Topic: s.Events.EventsTopic, Config: s.Outbox, Logger: logger,
		}),
		events.NewRetention(pool, store.OutboxTable, outboxRetention, outboxCleanup, logger),
		// Fills the follow feed from listing.events.
		events.NewListingConsumer(consumerClient, engagementRepo, producer, s.Events.ListingTopic, logger),
	}
	logger.InfoContext(ctx, "background workers enabled",
		"listing_topic", s.Events.ListingTopic, "consumer_group", s.Events.ConsumerGroup,
		"events_topic", s.Events.EventsTopic, "outbox_poll_interval", s.Outbox.PollInterval.String())

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace(),
		DrainDelay:    s.DrainDelay(),
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), workers...), lifecycle.Func(health.Run))
}

func newKafkaProducer(k sdkconfig.Kafka, opts ...kgo.Opt) (Producer, error) {
	p, err := kafkax.NewProducer(k, opts...)
	if err != nil {
		return nil, err
	}
	return p, nil
}
