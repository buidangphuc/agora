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

	"github.com/buidangphuc/team-promotion/internal/bootstrap"
	"github.com/buidangphuc/team-promotion/internal/config"
	"github.com/buidangphuc/team-promotion/internal/grpcserver"
	"github.com/buidangphuc/team-promotion/internal/handler"
	"github.com/buidangphuc/team-promotion/internal/repository"
	"github.com/buidangphuc/team-promotion/internal/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "team-promotion: %v\n", err)
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
	logger := slog.New(logHandler).With(slog.String("service", "team-promotion"))

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

	// Repositories: Postgres-backed when a pool is present (DATABASE_ENABLED=true),
	// otherwise the in-memory backends — mirrors team-order's branch so the service
	// boots without a database in local/test.
	var (
		voucherRepo      repository.VoucherRepository
		reservationRepo  repository.ReservationRepository
		flashSaleRepo    repository.FlashSaleRepository
		subscriptionRepo repository.SubscriptionRepository
		adCampaignRepo   repository.AdCampaignRepository
	)
	if res.Pool != nil {
		voucherRepo = repository.NewPostgresVoucherRepository(res.Pool)
		reservationRepo = repository.NewPostgresReservationRepository(res.Pool)
		flashSaleRepo = repository.NewPostgresFlashSaleRepository(res.Pool)
		subscriptionRepo = repository.NewPostgresSubscriptionRepository(res.Pool)
		adCampaignRepo = repository.NewPostgresAdCampaignRepository(res.Pool)
	} else {
		voucherRepo = repository.NewInMemoryVoucherRepository()
		reservationRepo = repository.NewInMemoryReservationRepository()
		flashSaleRepo = repository.NewInMemoryFlashSaleRepository()
		subscriptionRepo = repository.NewInMemorySubscriptionRepository()
		adCampaignRepo = repository.NewInMemoryAdCampaignRepository()
	}

	voucherSvc := service.NewVoucherService(voucherRepo, reservationRepo, res.Producer, res.Flags, logger)
	flashSaleSvc := service.NewFlashSaleService(flashSaleRepo, res.Producer, res.Flags, logger)
	subscriptionSvc := service.NewSubscriptionService(subscriptionRepo, logger)
	sponsoredSvc := service.NewSponsoredService(adCampaignRepo, logger)

	voucherHandler := handler.NewVoucherHandler(voucherSvc, logger)
	flashSaleHandler := handler.NewFlashSaleHandler(flashSaleSvc, logger)
	subscriptionHandler := handler.NewSubscriptionHandler(subscriptionSvc, logger)
	sponsoredHandler := handler.NewSponsoredHandler(sponsoredSvc, logger)

	srv := grpcserver.Build(settings, voucherHandler, flashSaleHandler, subscriptionHandler, sponsoredHandler, res.Health, logger)

	addr := net.JoinHostPort(settings.Server.Host, strconv.Itoa(settings.Server.Port))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("promotion gRPC server listening", slog.String("addr", addr))
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
