package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/buidangphuc/team-order/internal/bootstrap"
	"github.com/buidangphuc/team-order/internal/config"
	"github.com/buidangphuc/team-order/internal/consumer"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/grpcserver"
	"github.com/buidangphuc/team-order/internal/handler"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "team-order: %v\n", err)
		os.Exit(1)
	}
}

// reservationSettings resolves RESERVATION_TTL and RESERVATION_SWEEP_INTERVAL,
// logging a warning naming each unusable variable (it falls back to the default;
// boot never fails on these).
func reservationSettings(settings *config.Settings, logger *slog.Logger) (ttl, interval time.Duration) {
	ttl, warn := settings.ReservationTTL()
	if warn != "" {
		logger.Warn(warn)
	}
	interval, warn = settings.ReservationSweepInterval()
	if warn != "" {
		logger.Warn(warn)
	}
	return ttl, interval
}

// runReservationSweeper releases stock held by reservations past their TTL (AD3)
// on a fixed interval until ctx is cancelled. A sweep error is transient (a DB
// blip) — it is logged and retried on the next tick.
func runReservationSweeper(ctx context.Context, svc *service.OrderService, interval time.Duration, logger *slog.Logger) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			released, err := svc.SweepExpiredReservations(ctx, now)
			if err != nil {
				logger.Warn("reservation sweep failed", slog.Any("err", err))
				continue
			}
			if released > 0 {
				logger.Info("released expired reservations", slog.Int("released", released))
			}
		}
	}
}

func run() error {
	settings, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var handlerOpts slog.HandlerOptions
	if settings.Runtime.LogLevel == "debug" {
		handlerOpts.Level = slog.LevelDebug
	} else {
		handlerOpts.Level = slog.LevelInfo
	}
	var logHandler slog.Handler
	if settings.Runtime.LogJSON {
		logHandler = slog.NewJSONHandler(os.Stdout, &handlerOpts)
	} else {
		logHandler = slog.NewTextHandler(os.Stdout, &handlerOpts)
	}
	logger := slog.New(logHandler).With(slog.String("service", "team-order"))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	res, err := bootstrap.InitResources(ctx, settings, logger)
	if err != nil {
		return fmt.Errorf("init resources: %w", err)
	}
	// Fail fast before any port opens: staging/prod must never fall back to the
	// in-memory repositories (orders would vanish on restart).
	if err := settings.RequireDurableStorage(res.Pool != nil); err != nil {
		_ = bootstrap.CloseResources(context.Background(), res)
		return err
	}
	if res.Pool == nil {
		logger.Warn("running with IN-MEMORY repositories: orders, carts and sagas are NOT durable and are lost on restart",
			slog.String("env", settings.Runtime.Env))
	}
	defer func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := bootstrap.CloseResources(cctx, res); err != nil {
			logger.Warn("close resources", slog.Any("err", err))
		}
	}()

	// Dial upstream services (team-domain and team-identity)
	upstreamClients, err := upstream.Dial(settings.Upstream.DomainAddr, settings.Upstream.IdentityAddr)
	if err != nil {
		logger.Warn("could not connect to upstream services immediately, proceeding with lazy dials", slog.Any("err", err))
	} else {
		defer upstreamClients.Close()
	}

	// Dial team-promotion for checkout voucher redemption (W1-T2). An unset
	// UPSTREAM_PROMOTION_ADDR yields a nil client, so checkout degrades to its
	// existing no-voucher path rather than failing to boot.
	promoClients, err := upstream.DialPromotion(settings.Upstream.PromotionAddr)
	if err != nil {
		logger.Warn("could not connect to promotion service; voucher redemption disabled", slog.Any("err", err))
	} else if promoClients != nil {
		defer promoClients.Close()
	}

	var cartRepo repository.CartRepository
	var orderRepo repository.OrderRepository
	var returnRepo repository.ReturnRepository
	var shipmentRepo repository.ShipmentRepository
	if res.Pool != nil {
		cartRepo = repository.NewPostgresCartRepository(res.Pool)
		// Every first transition to PAID, and every won claim to CANCELLED, writes
		// its order.events outbox row in the same transaction (ADR-0013); the
		// relayer below publishes it.
		orderRepo = repository.NewPostgresOrderRepository(res.Pool,
			repository.WithPaidOutbox(events.BuildPaidOutboxRow),
			repository.WithCancelledOutbox(events.BuildCancelledOutboxRow))
		// A won APPROVED -> REFUNDED writes ReturnRefunded to the outbox in the
		// transition's own transaction (team-payment applies it).
		returnRepo = repository.NewPostgresReturnRepository(res.Pool, repository.WithReturnOutbox(events.BuildReturnRefundedOutboxRow))
		// OrderShipped is written to the outbox in the shipment's own transaction.
		shipmentRepo = repository.NewPostgresShipmentRepository(res.Pool, repository.WithShipmentOutbox(events.BuildShippedOutboxRow))
	} else {
		cartRepo = repository.NewInMemoryCartRepository()
		orderRepo = repository.NewInMemoryOrderRepository()
		returnRepo = repository.NewInMemoryReturnRepository()
		shipmentRepo = repository.NewInMemoryShipmentRepository()
	}

	cartSvc := service.NewCartService(cartRepo, orderRepo, upstreamClients.Listing, logger)

	// Durable saga/reservation store (AD3): persist reservation state in Postgres
	// so a crashed checkout's stock is swept and released, and compensation is not
	// best-effort in-memory. Falls back to the in-memory store when DB is disabled.
	reservationTTL, sweepInterval := reservationSettings(settings, logger)
	orderOpts := []service.OrderServiceOption{service.WithReservationTTL(reservationTTL)}
	if res.Pool != nil {
		orderOpts = append(orderOpts, service.WithSagaRepository(repository.NewPostgresSagaRepository(res.Pool)))
	}
	if promoClients != nil {
		orderOpts = append(orderOpts, service.WithPromotionClient(promoClients.Voucher))
	}
	orderSvc := service.NewOrderService(orderRepo, cartRepo, returnRepo, shipmentRepo, upstreamClients.Listing, upstreamClients.Address, logger, orderOpts...)

	cartHandler := handler.NewCartHandler(cartSvc, logger)
	orderHandler := handler.NewOrderHandler(orderSvc, upstreamClients.Address, logger, handler.WithFeatureFlags(res.Flags))

	// PaymentSettled consumer (AD4): drive orders to PAID from the event-carried
	// PaymentSettled on "payment.events", idempotently, with a DLQ for poison
	// records (AD1). Only when Postgres is present (dedupe ledger lives there) and
	// Kafka is enabled. Cancelled by ctx on shutdown.
	kcfg := bootstrap.KafkaConfigFromSettings(settings)

	// order.events outbox relayer (ADR-0013): drains pending OrderPaid rows to
	// Kafka, keyed by order_id. Needs Postgres (the outbox lives there), Kafka and
	// OUTBOX_ENABLED; otherwise rows are recorded but not relayed.
	if res.Pool != nil && kcfg.Enabled && settings.Outbox.Enabled {
		producer, err := bootstrap.NewOrderEventsProducer(kcfg)
		if err != nil {
			logger.Warn("order.events relayer disabled: kafka client init failed", slog.Any("err", err))
		} else {
			defer producer.Close()
			relayer := events.NewRelayer(
				repository.NewPgOutboxRepository(res.Pool),
				producer,
				events.RelayerConfig{
					Topic:        kcfg.OrderTopic,
					PollInterval: settings.OutboxPollInterval(),
					BatchSize:    settings.Outbox.BatchSize,
					LockDuration: time.Duration(settings.Outbox.ClaimLockSeconds) * time.Second,
					MaxAttempts:  settings.Outbox.MaxAttempts,
				},
				logger,
			)
			go func() {
				logger.Info("order.events outbox relayer starting",
					slog.String("topic", kcfg.OrderTopic),
					slog.Duration("poll_interval", settings.OutboxPollInterval()),
					slog.Int("batch_size", settings.Outbox.BatchSize))
				relayer.Run(ctx)
			}()
		}
	}

	if res.Pool != nil && kcfg.Enabled {
		pk, err := bootstrap.NewPaymentKafka(kcfg)
		if err != nil {
			logger.Warn("payment consumer disabled: kafka client init failed", slog.Any("err", err))
		} else {
			defer pk.Close()
			dedupe := repository.NewPostgresProcessedEventRepository(res.Pool)
			var consumerOpts []consumer.PaymentConsumerOption
			if promoClients != nil {
				consumerOpts = append(consumerOpts, consumer.WithVoucherCommitter(promoClients.Voucher))
			}
			paymentConsumer := consumer.NewPaymentConsumer(orderRepo, dedupe, logger, consumerOpts...)
			go func() {
				logger.Info("payment consumer starting",
					slog.String("topic", kcfg.Topic), slog.String("group", kcfg.ConsumerGroup))
				err := paymentConsumer.Run(ctx, pk.Reader(), pk.DLQ(), consumer.RunConfig{DLQTopic: kcfg.DLQTopic})
				// Run only returns on shutdown; any other exit leaves orders unpaid.
				if ctx.Err() == nil {
					logger.Error("payment consumer stopped while the service is running", slog.Any("err", err))
				}
			}()
		}
	}

	// Reservation sweeper (AD3): periodically release stock held past its TTL so a
	// crashed checkout never leaks inventory. Gated on Postgres like the saga repo.
	if res.Pool != nil {
		logger.Info("reservation sweeper starting",
			slog.String("reservation_ttl", reservationTTL.String()),
			slog.String("sweep_interval", sweepInterval.String()))
		go runReservationSweeper(ctx, orderSvc, sweepInterval, logger)
	}

	srv := grpcserver.Build(settings, cartHandler, orderHandler, res.Health, logger)

	addr := net.JoinHostPort(settings.Server.Host, strconv.Itoa(settings.Server.Port))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("order gRPC server listening", slog.String("addr", addr))
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining")
	}

	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		logger.Info("server stopped gracefully")
	case <-time.After(time.Duration(settings.Server.ShutdownGrace) * time.Second):
		logger.Warn("shutdown deadline exceeded, forcing stop")
		srv.Stop()
	}

	return nil
}
