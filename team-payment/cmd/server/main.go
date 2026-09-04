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

	"github.com/buidangphuc/team-payment/internal/bootstrap"
	"github.com/buidangphuc/team-payment/internal/config"
	"github.com/buidangphuc/team-payment/internal/grpcserver"
	"github.com/buidangphuc/team-payment/internal/handler"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
	"github.com/buidangphuc/team-payment/internal/upstream"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "team-payment: %v\n", err)
		os.Exit(1)
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
	logger := slog.New(logHandler).With(slog.String("service", "team-payment"))

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

	// Upstream order client
	orderClient, orderConn, err := upstream.DialOrderService(settings.Upstream.OrderAddr)
	if err != nil {
		logger.Warn("failed to dial order service immediately", slog.Any("err", err))
	} else if orderConn != nil {
		defer orderConn.Close()
	}

	var paymentRepo repository.PaymentRepository
	var walletRepo repository.WalletRepository
	var ledgerRepo repository.LedgerRepository
	if res.Pool != nil {
		paymentRepo = repository.NewPostgresPaymentRepository(res.Pool)
		walletRepo = repository.NewPostgresWalletRepository(res.Pool)
		ledgerRepo = repository.NewPostgresLedgerRepository(res.Pool)
	} else {
		paymentRepo = repository.NewInMemoryPaymentRepository()
		walletRepo = repository.NewInMemoryWalletRepository()
		ledgerRepo = repository.NewInMemoryLedgerRepository()
	}

	// Wire the transactional outbox writer (AD4): on settle, payment=PAID and the
	// PaymentSettled outbox row commit atomically, and the relayer publishes to
	// "payment.events". Without a DB (TxWriter nil) settle degrades to a status-only
	// update. The old synchronous order.UpdateOrderStatus RPC is gone — the order
	// transition is driven by the emitted event, consumed by team-order.
	var svcOpts []service.Option
	if res.TxWriter != nil {
		svcOpts = append(svcOpts, service.WithTxWriter(res.TxWriter))
	}
	svcOpts = append(svcOpts, service.WithLedgerRepo(ledgerRepo))
	paymentSvc := service.NewPaymentService(paymentRepo, walletRepo, orderClient, logger, svcOpts...)
	paymentHandler := handler.NewPaymentHandler(paymentSvc, logger)

	srv := grpcserver.Build(settings, paymentHandler, res.Health, logger)

	addr := net.JoinHostPort(settings.Server.Host, strconv.Itoa(settings.Server.Port))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("payment gRPC server listening", slog.String("addr", addr))
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
