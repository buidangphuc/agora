// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them. Read top to bottom to see what runs.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-chat/internal/chat"
	"github.com/buidangphuc/team-chat/internal/config"
	"github.com/buidangphuc/team-chat/internal/events"
	"github.com/buidangphuc/team-chat/internal/rpc"
	"github.com/buidangphuc/team-chat/internal/store"
)

const service = "team-chat"

// Publisher is what the outbox relayer produces to, plus the Close the app calls
// on shutdown.
type Publisher interface {
	outbox.Publisher
	Close()
}

// Options are the seams tests use; Run passes the zero value.
type Options struct {
	// NewPublisher builds the Kafka publisher. Default: kafkax.NewProducer.
	NewPublisher func(sdkconfig.Kafka) (Publisher, error)
}

// Run wires the service and runs it until ctx ends (main cancels it on
// SIGINT/SIGTERM) or a component fails. Build order: logger, telemetry, health,
// database, publisher, domain, server. Everything opened goes on the stack;
// closers run in reverse order on shutdown and on an early failure.
//
// Shutdown order: health drains, SHUTDOWN_DRAIN_DELAY passes, the gRPC
// server stops, then the outbox relayer stops, then the publisher and the pool
// close.
//
// Migrations are not applied here: the compose job team-chat-migrate does it.
func Run(ctx context.Context, s config.Settings) error {
	return run(ctx, s, Options{})
}

func run(ctx context.Context, s config.Settings, o Options) error {
	logger := logx.New(s.Runtime, service)
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

	// Threads and messages live in Postgres and there is no in-memory fallback,
	// so an unreachable database fails boot in every ENV.
	pool, err := postgres.Open(ctx, s.Postgres)
	if err != nil {
		return errors.Join(fmt.Errorf("open database: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", postgres.Checker(pool))

	newPublisher := o.NewPublisher
	if newPublisher == nil {
		newPublisher = newKafkaPublisher
	}
	publisher, err := newPublisher(s.Kafka)
	if err != nil {
		return errors.Join(fmt.Errorf("kafka publisher: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { publisher.Close(); return nil })

	// Every stored message writes its chat.events outbox row in the same
	// transaction (ADR-0013); the relayer below publishes it, so a Kafka outage
	// delays but never loses a notification.
	chatRepo, err := store.NewPostgresChatRepository(pool, store.WithMessageOutbox(events.BuildMessageOutboxRow))
	if err != nil {
		return errors.Join(err, stack.Close(ctx))
	}
	handler := rpc.NewChatHandler(chat.NewService(chatRepo, logger), logger)

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, handler)

	outboxStore, err := outbox.NewStore(pool, store.OutboxTable)
	if err != nil {
		return errors.Join(fmt.Errorf("outbox store: %w", err), stack.Close(ctx))
	}
	relayer, err := outbox.NewRelayer(outboxStore, publisher, outbox.RelayerOptions{
		Topic:  s.Events.ChatTopic,
		Config: s.Outbox,
		Logger: logger,
	})
	if err != nil {
		return errors.Join(fmt.Errorf("outbox relayer: %w", err), stack.Close(ctx))
	}

	// The relayer outlives the gRPC server so in-flight SendMessage calls that
	// enqueue rows are drained before it stops.
	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), relayer), lifecycle.Func(health.Run))
}

func newKafkaPublisher(c sdkconfig.Kafka) (Publisher, error) {
	p, err := kafkax.NewProducer(c)
	if err != nil {
		return nil, err
	}
	return p, nil
}
