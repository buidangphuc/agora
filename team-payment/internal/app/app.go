// Package app is the composition root: it builds every dependency in order, wires
// them together and runs them. Read top to bottom to see what runs.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

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
	sdkupstream "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/upstream"

	"github.com/buidangphuc/team-payment/internal/config"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/rpc"
	"github.com/buidangphuc/team-payment/internal/settlement"
	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/internal/upstream"
)

const service = "team-payment"

// outboxTable is the transactional outbox of payment events (migration 0003).
const outboxTable = "payment_outbox_events"

// Producer is the Kafka producer the outbox relayer and the dead-letter path share.
// *kafkax.Producer satisfies it.
type Producer interface {
	outbox.Publisher
	consume.DLQProducer
	Close()
}

// Options are the seams tests use; Run passes the zero value.
type Options struct {
	// NewProducer builds the Kafka producer. Default: kafkax.NewProducer.
	NewProducer func(sdkconfig.Kafka) (Producer, error)
	// LogOut receives the service log. Default: os.Stdout.
	LogOut io.Writer
}

// Run wires the service and runs it until ctx ends (main cancels it on SIGINT/SIGTERM)
// or a component fails. Build order: logger, telemetry, health, database, producer,
// order client, domain, consumer, server. Everything opened goes on the stack; closers
// run in reverse order on shutdown and on an early failure.
//
// Shutdown order: health drains, SHUTDOWN_DRAIN_DELAY passes, the gRPC server
// stops, then the outbox relayer and the settlement consumer stop, then the Kafka
// clients, the order connection and the pool close.
//
// Migrations are not applied here: the compose job team-payment-migrate does it.
func Run(ctx context.Context, s config.Settings) error {
	return RunWith(ctx, s, Options{})
}

// RunWith is Run with test seams.
func RunWith(ctx context.Context, s config.Settings, o Options) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := s.RequireNoMockPayments(); err != nil {
		return err
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

	// An unreachable database fails boot in every environment: there is no in-memory mode.
	pool, err := postgres.Open(ctx, s.Postgres)
	if err != nil {
		return errors.Join(fmt.Errorf("open database: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", postgres.Checker(pool))
	logger.InfoContext(ctx, "connected to postgres")

	newProducer := o.NewProducer
	if newProducer == nil {
		newProducer = func(c sdkconfig.Kafka) (Producer, error) { return kafkax.NewProducer(c) }
	}
	producer, err := newProducer(s.Kafka)
	if err != nil {
		return errors.Join(fmt.Errorf("kafka producer: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { producer.Close(); return nil })

	orderConn, err := sdkupstream.NewClient(s.Upstream.OrderAddr)
	if err != nil {
		return errors.Join(fmt.Errorf("order client: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { return orderConn.Close() })

	// Settle writes payment=PAID and the PaymentSettled outbox row in one transaction
	// (AD4); the relayer publishes the row to payment.events.
	outboxStore, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		return errors.Join(fmt.Errorf("outbox store: %w", err), stack.Close(ctx))
	}
	ledger := store.NewPostgresLedgerRepository(pool)
	svc := payment.NewService(
		store.NewPostgresPaymentRepository(pool),
		store.NewPostgresWalletRepository(pool),
		upstream.NewOrderClient(orderConn),
		logger,
		payment.WithTxWriter(store.NewPgTxWriter(pool, outboxStore)),
		payment.WithLedgerRepo(ledger),
		payment.WithSettlementLedger(store.NewPostgresSettlementLedger(pool)),
		payment.WithPayoutHold(s.Payout.Window),
	)
	logger.InfoContext(ctx, "payout hold window", "window", s.Payout.Window.String(), "enabled", s.Payout.Window > 0)

	// Seller-ledger consumer of order.events. A group with no committed offset starts at
	// the latest offset (design D10): historical OrderPaidEvents were credited inline by
	// the old binary and must not be credited again.
	consumerClient, err := kafkax.NewConsumerClient(s.Kafka, s.Events.SettlementGroup, []string{s.Events.OrderTopic},
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()))
	if err != nil {
		return errors.Join(fmt.Errorf("settlement consumer: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { consumerClient.Close(); return nil })
	consumer, err := consume.NewConsumer(consumerClient, settlement.New(svc, logger).Handle, producer,
		consume.Options{DLQTopic: s.Events.SettlementDLQTopic, Logger: logger})
	if err != nil {
		return errors.Join(fmt.Errorf("settlement consumer: %w", err), stack.Close(ctx))
	}

	relayer, err := outbox.NewRelayer(outboxStore, producer, outbox.RelayerOptions{
		Topic: s.Events.PaymentTopic, Config: s.Outbox, Logger: logger,
	})
	if err != nil {
		return errors.Join(fmt.Errorf("outbox relayer: %w", err), stack.Close(ctx))
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, rpc.NewPaymentHandler(svc, logger, rpc.WithMockPayments(s.Mock.MockPayments)))

	// The relayer and the consumer run until the gRPC server has stopped, so in-flight
	// RPCs that enqueue outbox rows are drained before they stop.
	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	},
		lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), relayer, consumer),
		lifecycle.Func(health.Run),
	)
}
