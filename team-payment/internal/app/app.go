package app

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/mockdata"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-payment/internal/config"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/rpc"
	"github.com/buidangphuc/team-payment/internal/settlement"
	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/internal/upstream"
)

const (
	serviceName   = "team-payment"
	purgeInterval = time.Hour
	closeGrace    = 2 * time.Second
)

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

	mocks, err := mockdata.New(s.MockData, "payment-provider")
	if err != nil {
		return err
	}
	mocks.LogActive(logger)

	pool, err := postgres.Open(ctx, s.Postgres)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", postgres.Checker(pool))

	orderClient, orderConn, err := upstream.DialOrderService(s.Upstream.OrderAddr)
	if err != nil {
		return err
	}
	stack.Push(func(context.Context) error { return orderConn.Close() })

	auditConn, err := auditx.Dial(sdkconfig.Audit{Addr: s.Upstream.AuditAddr}, "service-"+serviceName, "audit.write")
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	var writer auditx.Writer
	if auditConn != nil {
		stack.Push(func(context.Context) error { return auditConn.Close() })
		writer = upstream.NewAuditWriter(auditConn)
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

	svc, err := payment.NewService(
		store.NewPostgresPaymentRepository(pool),
		store.NewPostgresWalletRepository(pool),
		orderClient,
		logger,
		store.NewPgTxWriter(pool, outStore),
		store.NewPostgresLedgerRepository(pool),
		store.NewPostgresSettlementLedger(pool),
		payment.WithPayoutHold(s.Payout.Window),
		payment.WithProvider(payment.NewProvider(mocks)),
	)
	if err != nil {
		return err
	}
	logger.InfoContext(ctx, "payout hold window", "window", s.Payout.Window.String(), "enabled", s.Payout.Window > 0)

	h, err := rpc.NewHandler(svc, auditx.NewGate(writer, logger), logger)
	if err != nil {
		return err
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, h)

	relayer, err := outbox.NewRelayer(outStore, producer, outbox.RelayerOptions{
		Topic:  s.Events.PaymentTopic,
		Config: s.Outbox,
		Logger: logger,
	})
	if err != nil {
		return err
	}

	purge, err := lifecycle.Every(purgeInterval, func(ctx context.Context) error {
		_, err := store.PurgeOutbox(ctx, outStore)
		return err
	}, lifecycle.RunAtStart(), lifecycle.WithLogger(logger))
	if err != nil {
		return err
	}

	var fp *kafkax.Fingerprinter
	if s.LogKeyFP.Secret != "" {
		if fp, err = kafkax.NewFingerprinter(s.LogKeyFP.Secret); err != nil {
			return fmt.Errorf("log key fingerprint: %w", err)
		}
	}
	client, err := kafkax.NewConsumerClient(s.Kafka, s.Events.ConsumerGroup, []string{s.Events.OrderTopic},
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()), kgo.AllowAutoTopicCreation())
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	stack.Push(func(context.Context) error { client.Close(); return nil })
	consumer, err := consume.NewConsumer(client, settlement.NewHandler(svc, logger).Handle, producer,
		consume.Options{DLQTopic: s.Events.DLQTopic, Logger: logger, KeyFP: fp})
	if err != nil {
		return fmt.Errorf("consumer %s: %w", s.Events.ConsumerGroup, err)
	}

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace + 2*closeGrace,
		CloseGrace:    closeGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, health, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), relayer, purge, consumer))
}
