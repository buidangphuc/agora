package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-referral/internal/config"
	"github.com/buidangphuc/team-referral/internal/consumer"
	"github.com/buidangphuc/team-referral/internal/referral"
	"github.com/buidangphuc/team-referral/internal/rpc"
	"github.com/buidangphuc/team-referral/internal/store"
)

const serviceName = "team-referral"

func Run(ctx context.Context, s config.Settings) (err error) {
	logger := logx.New(s.Runtime, serviceName)

	if len(s.Kafka.BrokerList()) == 0 {
		return errors.New("kafka: KAFKA_BROKERS is required")
	}
	capLocation, err := s.Referral.Location()
	if err != nil {
		return fmt.Errorf("referral: REFERRAL_CAP_TIMEZONE: %w", err)
	}

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

	repo := store.NewPostgresReferralRepo(pool)
	cfg := referral.Config{
		RewardXU:            s.Referral.RewardXU,
		DailyCap:            s.Referral.DailyCap,
		LifetimeCap:         s.Referral.LifetimeCap,
		RedeemMaxAccountAge: s.Referral.RedeemMaxAccountAge,
		SweepInterval:       s.Referral.SweepInterval,
		CapLocation:         capLocation,
	}

	h, err := rpc.NewHandler(referral.NewService(repo, referral.WithConfig(cfg)), logger)
	if err != nil {
		return err
	}

	producer, err := kafkax.NewProducer(s.Kafka, kgo.AllowAutoTopicCreation())
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	stack.Push(func(context.Context) error { producer.Close(); return nil })

	outStore, err := outbox.NewStore(pool, store.OutboxTable)
	if err != nil {
		return err
	}
	relayer, err := outbox.NewRelayer(outStore, producer, outbox.RelayerOptions{
		Topic:  s.Referral.EventsTopic,
		Config: s.Outbox,
		Logger: logger,
	})
	if err != nil {
		return err
	}

	handlerCfg := consumer.Config{RewardXU: cfg.RewardXU, LifetimeCap: cfg.LifetimeCap, Logger: logger}
	identity, err := newConsumer(s, &stack, producer, logger,
		s.Referral.IdentityGroup, s.Referral.IdentityTopic, s.Referral.IdentityDLQTopic,
		consumer.NewIdentityHandler(repo, handlerCfg))
	if err != nil {
		return err
	}
	orders, err := newConsumer(s, &stack, producer, logger,
		s.Referral.OrderGroup, s.Referral.OrderTopic, s.Referral.OrderDLQTopic,
		consumer.NewOrderHandler(repo, handlerCfg))
	if err != nil {
		return err
	}
	sweeper := referral.NewSweeper(repo, referral.WithConfig(cfg), referral.WithLogger(logger))

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, h)

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, health, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), relayer, identity, orders, sweeper))
}

func newConsumer(s config.Settings, stack *lifecycle.Stack, dlq consume.DLQProducer, logger *slog.Logger, group, topic, dlqTopic string, handler consume.Handler) (*consume.Consumer, error) {
	client, err := kafkax.NewConsumerClient(s.Kafka, group, []string{topic}, kgo.AllowAutoTopicCreation())
	if err != nil {
		return nil, fmt.Errorf("kafka: %w", err)
	}
	stack.Push(func(context.Context) error { client.Close(); return nil })
	c, err := consume.NewConsumer(client, handler, dlq, consume.Options{DLQTopic: dlqTopic, Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("kafka consumer %s: %w", group, err)
	}
	return c, nil
}
