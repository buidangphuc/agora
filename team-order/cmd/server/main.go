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
		orderRepo = repository.NewPostgresOrderRepository(res.Pool)
		returnRepo = repository.NewPostgresReturnRepository(res.Pool)
		shipmentRepo = repository.NewPostgresShipmentRepository(res.Pool)
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
	var orderOpts []service.OrderServiceOption
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
	kcfg := bootstrap.KafkaConfigFromEnv()
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
				if err := paymentConsumer.Run(ctx, pk.Reader(), pk.DLQ(), consumer.RunConfig{DLQTopic: kcfg.DLQTopic}); err != nil &&
					!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					logger.Error("payment consumer stopped with error", slog.Any("err", err))
				}
			}()
		}
	}

	// Reservation sweeper (AD3): periodically release stock held past its TTL so a
	// crashed checkout never leaks inventory. Gated on Postgres like the saga repo.
	if res.Pool != nil {
		go runReservationSweeper(ctx, orderSvc, time.Minute, logger)
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
