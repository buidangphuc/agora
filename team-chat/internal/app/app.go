package app

import (
	"context"
	"fmt"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-chat/internal/chat"
	"github.com/buidangphuc/team-chat/internal/config"
	"github.com/buidangphuc/team-chat/internal/events"
	"github.com/buidangphuc/team-chat/internal/rpc"
	"github.com/buidangphuc/team-chat/internal/store"
)

const (
	serviceName          = "team-chat"
	publishedRetention   = 7 * 24 * time.Hour
	defaultPurgeInterval = time.Hour
	closeGrace           = 2 * time.Second
)

type Options struct {
	NewPublisher  func(sdkconfig.Kafka) (outbox.Publisher, error)
	PurgeInterval time.Duration
}

func Run(ctx context.Context, s config.Settings) error {
	return RunWithOptions(ctx, s, Options{})
}

func newKafkaPublisher(c sdkconfig.Kafka) (outbox.Publisher, error) {
	return kafkax.NewProducer(c)
}

func RunWithOptions(ctx context.Context, s config.Settings, o Options) (err error) {
	logger := logx.New(s.Runtime, serviceName)

	var stack lifecycle.Stack
	defer func() {
		if err != nil {
			_ = stack.Close(context.WithoutCancel(ctx))
		}
	}()

	shutdownOTel, err := otelx.Init(ctx, sdkconfig.OTel(s.OTel), serviceName)
	if err != nil {
		return fmt.Errorf("otel: %w", err)
	}
	stack.Push(shutdownOTel)

	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		return err
	}

	pool, err := postgres.Open(ctx, s.Postgres)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", postgres.Checker(pool))

	newPublisher := o.NewPublisher
	if newPublisher == nil {
		newPublisher = newKafkaPublisher
	}
	pub, err := newPublisher(s.Kafka)
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	if c, ok := pub.(interface{ Close() }); ok {
		stack.Push(func(context.Context) error { c.Close(); return nil })
	}

	repo := store.NewPostgresChatRepository(pool, store.WithMessageOutbox(events.BuildMessageOutboxRow))
	h, err := rpc.NewHandler(chat.NewService(repo, logger), logger)
	if err != nil {
		return err
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, h)

	outStore, err := outbox.NewStore(pool, store.OutboxTable)
	if err != nil {
		return err
	}
	relayer, err := outbox.NewRelayer(outStore, pub, outbox.RelayerOptions{
		Topic:  s.Events.ChatTopic,
		Config: s.Outbox,
		Logger: logger,
	})
	if err != nil {
		return err
	}

	interval := o.PurgeInterval
	if interval <= 0 {
		interval = defaultPurgeInterval
	}
	purge, err := lifecycle.Every(interval, func(ctx context.Context) error {
		_, err := outStore.Purge(ctx, publishedRetention)
		return err
	}, lifecycle.WithLogger(logger))
	if err != nil {
		return err
	}

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace + 2*closeGrace,
		CloseGrace:    closeGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, health, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), relayer, purge))
}
