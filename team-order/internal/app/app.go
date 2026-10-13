// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them. Read top to bottom to see what runs.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/flags"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-order/internal/config"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/upstream"
)

const service = "team-order"

// Run wires the service and runs it until ctx ends (main cancels it on
// SIGINT/SIGTERM) or a component fails. Build order: logger, telemetry, health,
// database, flags, upstream clients, repositories and services, Kafka, server.
// Everything opened goes on the stack; closers run in reverse order on shutdown
// and on an early failure.
//
// Storage: an unreachable database fails boot unless ENV is local or test
// (postgres.OpenOrLocal). There the in-memory repositories keep the service
// usable; orders, carts and sagas then live only for the process, and the outbox
// relayer and the payment consumer, which need Postgres, do not run.
//
// Shutdown order: health drains, SHUTDOWN_DRAIN_DELAY passes, the gRPC
// server stops, then the relayer, the payment consumer and the reservation
// sweeper stop (lifecycle.StopAfter), then the Kafka clients, upstream
// connections and the pool close.
//
// Migrations are not applied here: the compose job team-order-migrate does it.
func Run(ctx context.Context, s config.Settings) error {
	if err := s.Postgres.Validate(); err != nil {
		return fmt.Errorf("team-order owns the orders DB: %w", err)
	}
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
		return errors.Join(fmt.Errorf("init health: %w", err), stack.Close(ctx))
	}

	// Fail fast before any port opens: outside ENV=local/test the service never
	// falls back to the in-memory repositories (orders would vanish on restart).
	pool, err := postgres.OpenOrLocal(ctx, s.Postgres, s.Runtime, logger)
	if err != nil {
		return errors.Join(fmt.Errorf("open database: %w", err), stack.Close(ctx))
	}
	if pool == nil {
		logger.WarnContext(ctx, "running with IN-MEMORY repositories: orders, carts and sagas are NOT durable and are lost on restart",
			"env", s.Env)
	} else {
		stack.Push(func(context.Context) error { pool.Close(); return nil })
		health.Register("postgres", postgres.Checker(pool))
	}

	// Feature flags: a flag-system outage must not stop the service, so a client
	// that cannot be set up degrades to the fail-open defaults.
	featureFlags, closeFlags := flags.NewOrDisabled(ctx, s.Flags, logger)
	stack.Push(closeFlags)

	// Upstream clients dial lazily: an unreachable peer fails the call, not the boot.
	upstreamClients, err := upstream.Dial(s.Upstream.DomainAddr, s.Upstream.IdentityAddr)
	if err != nil {
		return errors.Join(fmt.Errorf("dial upstream services: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { upstreamClients.Close(); return nil })

	// team-promotion serves checkout voucher redemption. An unset
	// UPSTREAM_PROMOTION_ADDR yields a nil client, so checkout degrades to its
	// no-voucher path instead of failing to boot.
	promoClients, err := upstream.DialPromotion(s.Upstream.PromotionAddr)
	if err != nil {
		logger.WarnContext(ctx, "could not set up the promotion client; voucher redemption disabled", "err", err)
		promoClients = nil
	}
	if promoClients != nil {
		stack.Push(func(context.Context) error { promoClients.Close(); return nil })
	}

	repos := newRepositories(pool)
	cartSvc := order.NewCartService(repos.carts, repos.orders, upstreamClients.Listing, logger)

	reservationTTL, warn := s.ReservationTTL()
	if warn != "" {
		logger.WarnContext(ctx, warn)
	}
	sweepInterval, warn := s.ReservationSweepInterval()
	if warn != "" {
		logger.WarnContext(ctx, warn)
	}
	orderOpts := []order.Option{order.WithReservationTTL(reservationTTL)}
	if pool != nil {
		// Durable saga/reservation store (AD3): a crashed checkout's stock is swept
		// and released, and compensation is not best-effort in-memory.
		orderOpts = append(orderOpts, order.WithSagaRepository(store.NewPostgresSagaRepository(pool)))
	}
	if promoClients != nil {
		orderOpts = append(orderOpts, order.WithPromotionClient(promoClients.Voucher))
	}
	orderSvc := order.NewOrderService(repos.orders, repos.carts, repos.returns, repos.shipments,
		upstreamClients.Listing, upstreamClients.Address, logger, orderOpts...)

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv,
		rpc.NewCartHandler(cartSvc, logger),
		rpc.NewOrderHandler(orderSvc, upstreamClients.Address, logger, rpc.WithFeatureFlags(featureFlags)))

	// Workers stop only after the gRPC server has returned, so in-flight RPCs that
	// enqueue outbox rows or touch reservations are drained first.
	var workers []lifecycle.Component
	if pool != nil {
		w, err := newWorkers(s, pool, repos.pgOrders, promoClients, orderSvc, sweepInterval, logger, &stack)
		if err != nil {
			return errors.Join(err, stack.Close(ctx))
		}
		workers = w
		logger.InfoContext(ctx, "background workers enabled",
			"order_topic", s.Events.OrderTopic, "payment_topic", s.Events.PaymentTopic,
			"consumer_group", s.Events.ConsumerGroup, "reservation_ttl", reservationTTL.String(),
			"sweep_interval", sweepInterval.String())
	}

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), workers...), lifecycle.Func(health.Run))
}

// repositories are the stores the services share: Postgres when a pool is open,
// in-memory otherwise.
type repositories struct {
	carts     store.CartRepository
	orders    store.OrderRepository
	returns   store.ReturnRepository
	shipments store.ShipmentRepository
	// pgOrders is the Postgres order repository, nil with the in-memory stores. The
	// PaymentSettled consumer runs its transition inside its own transaction.
	pgOrders *store.PostgresOrderRepository
}

func newRepositories(pool *pgxpool.Pool) repositories {
	if pool == nil {
		return repositories{
			carts:     store.NewInMemoryCartRepository(),
			orders:    store.NewInMemoryOrderRepository(),
			returns:   store.NewInMemoryReturnRepository(),
			shipments: store.NewInMemoryShipmentRepository(),
		}
	}
	// Every first transition to PAID, and every won claim to CANCELLED, writes its
	// order.events outbox row in the same transaction (ADR-0013); the relayer
	// publishes it.
	orders := store.NewPostgresOrderRepository(pool,
		store.WithPaidOutbox(events.BuildPaidOutboxRow),
		store.WithCancelledOutbox(events.BuildCancelledOutboxRow))
	return repositories{
		carts:    store.NewPostgresCartRepository(pool),
		orders:   orders,
		pgOrders: orders,
		// A won APPROVED -> REFUNDED writes ReturnRefunded to the outbox in the
		// transition's own transaction (team-payment applies it).
		returns: store.NewPostgresReturnRepository(pool, store.WithReturnOutbox(events.BuildReturnRefundedOutboxRow)),
		// OrderShipped is written to the outbox in the shipment's own transaction.
		shipments: store.NewPostgresShipmentRepository(pool, store.WithShipmentOutbox(events.BuildShippedOutboxRow)),
	}
}

// newWorkers builds the components that need Postgres: the order.events outbox
// relayer (ADR-0013), the PaymentSettled consumer (AD4) and the reservation
// sweeper (AD3). The Kafka clients go on the stack.
func newWorkers(
	s config.Settings,
	pool *pgxpool.Pool,
	orders *store.PostgresOrderRepository,
	promo *upstream.PromotionClients,
	orderSvc *order.Service,
	sweepInterval time.Duration,
	logger *slog.Logger,
	stack *lifecycle.Stack,
) ([]lifecycle.Component, error) {
	producer, err := kafkax.NewProducer(s.Kafka)
	if err != nil {
		return nil, fmt.Errorf("kafka producer: %w", err)
	}
	stack.Push(func(context.Context) error { producer.Close(); return nil })

	queue, err := outbox.NewStore(pool, store.OutboxTable)
	if err != nil {
		return nil, fmt.Errorf("outbox store: %w", err)
	}
	relayer, err := outbox.NewRelayer(queue, producer, outbox.RelayerOptions{
		Topic: s.Events.OrderTopic, Config: s.Outbox, Logger: logger,
	})
	if err != nil {
		return nil, fmt.Errorf("outbox relayer: %w", err)
	}

	consumerClient, err := kafkax.NewConsumerClient(s.Kafka, s.Events.ConsumerGroup, []string{s.Events.PaymentTopic})
	if err != nil {
		return nil, fmt.Errorf("kafka consumer: %w", err)
	}
	stack.Push(func(context.Context) error { consumerClient.Close(); return nil })

	var consumerOpts []events.PaymentConsumerOption
	if promo != nil {
		consumerOpts = append(consumerOpts, events.WithVoucherCommitter(promo.Voucher))
	}
	handler := events.NewPaymentConsumer(store.NewPostgresTransactor(pool, orders), logger, consumerOpts...)
	paymentConsumer, err := consume.NewConsumer(consumerClient, handler.Handle, producer, consume.Options{
		DLQTopic: s.Events.PaymentDLQ, Logger: logger,
	})
	if err != nil {
		return nil, fmt.Errorf("payment consumer: %w", err)
	}

	sweeper, err := reservationSweeper(orderSvc, sweepInterval, logger)
	if err != nil {
		return nil, err
	}
	return []lifecycle.Component{relayer, paymentConsumer, sweeper}, nil
}

// reservationSweeper releases stock held by reservations past their TTL (AD3) on a
// fixed interval until ctx is cancelled, so a crashed checkout never leaks
// inventory. A sweep error is transient (a DB blip): lifecycle.Every logs it and
// runs again on the next tick.
func reservationSweeper(svc *order.Service, interval time.Duration, logger *slog.Logger) (lifecycle.Component, error) {
	sweeper, err := lifecycle.Every(interval, func(ctx context.Context) error {
		released, err := svc.SweepExpiredReservations(ctx, time.Now())
		if err != nil {
			return fmt.Errorf("reservation sweep: %w", err)
		}
		if released > 0 {
			logger.InfoContext(ctx, "released expired reservations", "released", released)
		}
		return nil
	}, lifecycle.WithLogger(logger))
	if err != nil {
		return nil, fmt.Errorf("reservation sweeper: %w", err)
	}
	return sweeper, nil
}
