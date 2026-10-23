package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-notification/internal/alerts"
	"github.com/buidangphuc/team-notification/internal/config"
	"github.com/buidangphuc/team-notification/internal/events"
	"github.com/buidangphuc/team-notification/internal/inbox"
	"github.com/buidangphuc/team-notification/internal/names"
	"github.com/buidangphuc/team-notification/internal/prefs"
	"github.com/buidangphuc/team-notification/internal/rpc"
	"github.com/buidangphuc/team-notification/internal/store"
)

const (
	serviceName = "team-notification"
	closeGrace  = 2 * time.Second
)

type inboxStore struct{ *store.Notifications }

func (s inboxStore) MarkAsRead(ctx context.Context, id, userID string) error {
	err := s.Notifications.MarkAsRead(ctx, id, userID)
	if errors.Is(err, store.ErrNotFound) {
		return inbox.ErrNotFound
	}
	return err
}

func consumer(s config.Settings, logger *slog.Logger, fp *kafkax.Fingerprinter, dlq consume.DLQProducer, stack *lifecycle.Stack, topic, dlqTopic, group string, h consume.Handler) (lifecycle.Component, error) {
	client, err := kafkax.NewConsumerClient(s.Kafka, group, []string{topic}, kgo.AllowAutoTopicCreation())
	if err != nil {
		return nil, fmt.Errorf("kafka: %w", err)
	}
	stack.Push(func(context.Context) error { client.Close(); return nil })
	c, err := consume.NewConsumer(client, h, dlq, consume.Options{DLQTopic: dlqTopic, Logger: logger, KeyFP: fp})
	if err != nil {
		return nil, fmt.Errorf("consumer %s: %w", group, err)
	}
	return c, nil
}

func Run(ctx context.Context, s config.Settings) (err error) {
	logger := logx.New(s.Runtime, serviceName)

	var stack lifecycle.Stack
	defer func() {
		if err != nil {
			_ = stack.Close(context.WithoutCancel(ctx))
		}
	}()

	shutdownOTel, err := otelx.Init(ctx, s.OTel, serviceName)
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

	n := s.Notification
	resolver, err := names.New(n.DomainAddr, n.IdentityAddr, logger)
	if err != nil {
		return fmt.Errorf("names: %w", err)
	}
	stack.Push(func(context.Context) error { resolver.Close(); return nil })

	h, err := rpc.NewHandler(inboxStore{store.NewNotifications(pool)},
		alerts.NewService(store.NewAlerts(pool)), prefs.NewService(store.NewPrefs(pool)), logger)
	if err != nil {
		return err
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, h)

	var fp *kafkax.Fingerprinter
	if s.LogKeyFP.Secret != "" {
		if fp, err = kafkax.NewFingerprinter(s.LogKeyFP.Secret); err != nil {
			return fmt.Errorf("log key fingerprint: %w", err)
		}
	}
	dlq, err := kafkax.NewProducer(s.Kafka)
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	stack.Push(func(context.Context) error { dlq.Close(); return nil })

	specs := []struct {
		topic, dlq, group string
		handler           consume.Handler
	}{
		{n.ListingTopic, n.ListingDLQTopic, n.ListingConsumerGroup, events.NewListingHandler(pool, n.LowStockThreshold, logger)},
		{n.ChatTopic, n.ChatDLQTopic, n.ChatConsumerGroup, events.NewChatHandler(pool, resolver, logger)},
		{n.OrderTopic, n.OrderDLQTopic, n.OrderConsumerGroup, events.NewOrderHandler(pool, logger)},
	}
	components := []lifecycle.Component{health, grpcx.Serve(srv, s.GRPC)}
	for _, sp := range specs {
		c, err := consumer(s, logger, fp, dlq, &stack, sp.topic, sp.dlq, sp.group, sp.handler)
		if err != nil {
			return err
		}
		components = append(components, c)
	}

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace + 2*closeGrace,
		CloseGrace:    closeGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, components...)
}
