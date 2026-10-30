package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/flags"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-order/internal/config"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/upstream"

	auditv1 "github.com/buidangphuc/team-order/generated/platform/audit/v1"
)

const (
	serviceName      = "team-order"
	servicePrincipal = "service-team-order"
	auditScope       = "audit.write"
	outboxTable      = "order_outbox_events"
	closeGrace       = 2 * time.Second
)

var wrapComponent = func(_ string, c lifecycle.Component) lifecycle.Component { return c }

type reservationSweeper interface {
	SweepExpiredReservations(ctx context.Context, now time.Time) (int, error)
}

type lifecycleSweeper interface {
	SweepLifecycle(ctx context.Context, now time.Time)
}

func reservationSweep(svc reservationSweeper, interval time.Duration, logger *slog.Logger) (lifecycle.Component, error) {
	return lifecycle.Every(interval, func(ctx context.Context) error {
		released, err := svc.SweepExpiredReservations(ctx, time.Now())
		if err != nil {
			return fmt.Errorf("reservation sweep: %w", err)
		}
		if released > 0 {
			logger.InfoContext(ctx, "released expired reservations", "released", released)
		}
		return nil
	}, lifecycle.WithLogger(logger))
}

func lifecycleSweep(svc lifecycleSweeper, interval time.Duration, logger *slog.Logger) (lifecycle.Component, error) {
	return lifecycle.Every(interval, func(ctx context.Context) error {
		svc.SweepLifecycle(ctx, time.Now())
		return nil
	}, lifecycle.WithLogger(logger))
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

	evaluator, err := flags.New(ctx, s.Flags, logger)
	if err != nil {
		return fmt.Errorf("flags: %w", err)
	}
	stack.Push(evaluator.Close)

	clients, err := upstream.DialAll(s)
	if err != nil {
		return err
	}
	stack.Push(func(context.Context) error { clients.Close(); return nil })

	auditConn, err := auditx.Dial(sdkconfig.Audit{Addr: s.Audit.Addr}, servicePrincipal, auditScope)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	if auditConn == nil {
		return errors.New("audit: UPSTREAM_AUDIT_ADDR is required")
	}
	stack.Push(func(context.Context) error { return auditConn.Close() })
	gate := auditx.NewGate(upstream.NewAuditWriter(auditv1.NewAuditServiceClient(auditConn)), logger)

	pub, err := kafkax.NewProducer(s.Kafka, kgo.AllowAutoTopicCreation())
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	stack.Push(func(context.Context) error { pub.Close(); return nil })

	outStore, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		return err
	}

	warn := func(msg string) {
		if msg != "" {
			logger.WarnContext(ctx, msg)
		}
	}
	reservationTTL, msg := s.ReservationTTL()
	warn(msg)
	sweepInterval, msg := s.ReservationSweepInterval()
	warn(msg)
	paymentDeadline, msg := s.PaymentDeadline()
	warn(msg)
	reminderBefore, msg := s.PaymentReminderBefore()
	warn(msg)
	lifecycleInterval, msg := s.LifecycleSweepInterval()
	warn(msg)

	cartRepo := store.NewPostgresCartRepository(pool)
	orderRepo := store.NewPostgresOrderRepository(pool, outStore,
		store.WithPaidOutbox(events.BuildPaidOutboxRow),
		store.WithCancelledOutbox(events.BuildCancelledOutboxRow),
		store.WithPlacedOutbox(events.BuildPlacedOutboxRow),
		store.WithReminderOutbox(events.BuildPaymentReminderOutboxRow),
		store.WithCompletedOutbox(events.BuildCompletedOutboxRow),
		store.WithAutoCompleteAfter(s.AutoCompleteAfter()),
		store.WithReturnWindow(s.ReturnWindow()),
		store.WithShipByAfter(s.ShipByAfter()))
	returnRepo := store.NewPostgresReturnRepository(pool, outStore,
		store.WithReturnOutbox(events.BuildReturnRefundedOutboxRow),
		store.WithReturnRequestedOutbox(events.BuildReturnRequestedOutboxRow),
		store.WithReturnAutoApproveAfter(s.ReturnAutoApproveAfter()))
	shipmentRepo := store.NewPostgresShipmentRepository(pool, outStore, store.WithShipmentOutbox(events.BuildShippedOutboxRow))

	cartSvc, err := order.NewCartService(cartRepo, orderRepo, clients.Listing, logger, order.WithFlashSaleClient(clients.Flash))
	if err != nil {
		return err
	}
	orderSvc, err := order.NewOrderService(orderRepo, cartRepo, returnRepo, shipmentRepo, clients.Listing, clients.Address, logger,
		order.WithReservationTTL(reservationTTL),
		order.WithPaymentDeadline(paymentDeadline),
		order.WithPaymentReminderBefore(reminderBefore),
		order.WithSagaRepository(store.NewPostgresSagaRepository(pool)),
		order.WithPromotionClient(clients.Voucher),
		order.WithOrderFlashSaleClient(clients.Flash))
	if err != nil {
		return err
	}
	orderHandler, err := rpc.NewOrderHandler(orderSvc, clients.Address, evaluator, gate, logger)
	if err != nil {
		return err
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, rpc.NewCartHandler(cartSvc, logger), orderHandler)

	relayer, err := events.NewRelayer(outStore, pub, s, logger)
	if err != nil {
		return err
	}
	purge, err := events.NewOutboxPurger(outStore, logger)
	if err != nil {
		return err
	}

	fp, err := kafkax.NewFingerprinter(s.LogKeyFP.Secret)
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	paymentClient, err := kafkax.NewConsumerClient(s.Kafka, s.Events.PaymentGroup, []string{s.Events.PaymentTopic}, kgo.AllowAutoTopicCreation())
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	stack.Push(func(context.Context) error { paymentClient.Close(); return nil })
	paymentHandler := events.NewPaymentHandler(orderRepo, pool, logger, events.WithVoucherCommitter(clients.Voucher))
	paymentConsumer, err := events.NewPaymentConsumer(paymentClient, paymentHandler.Handle, pub, fp, s)
	if err != nil {
		return err
	}

	disputeClient, err := kafkax.NewConsumerClient(s.Kafka, s.Events.DisputeGroup, []string{s.Events.EngagementTopic}, kgo.AllowAutoTopicCreation())
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	stack.Push(func(context.Context) error { disputeClient.Close(); return nil })
	disputeHandler, err := events.NewDisputeHandler(orderRepo, returnRepo, gate, pool, logger)
	if err != nil {
		return err
	}
	disputeConsumer, err := events.NewDisputeConsumer(disputeClient, disputeHandler.Handle, pub, fp, s)
	if err != nil {
		return err
	}

	reservations, err := reservationSweep(orderSvc, sweepInterval, logger)
	if err != nil {
		return err
	}
	logger.InfoContext(ctx, "reservation sweeper starting",
		"reservation_ttl", reservationTTL.String(),
		"sweep_interval", sweepInterval.String())
	lifecycles, err := lifecycleSweep(orderSvc, lifecycleInterval, logger)
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
	}, health, lifecycle.StopAfter(wrapComponent("server", grpcx.Serve(srv, s.GRPC)),
		wrapComponent("relayer", relayer),
		wrapComponent("payment-consumer", paymentConsumer),
		wrapComponent("dispute-consumer", disputeConsumer),
		wrapComponent("reservation-sweep", reservations),
		wrapComponent("lifecycle-sweep", lifecycles),
		wrapComponent("outbox-purge", purge)))
}
