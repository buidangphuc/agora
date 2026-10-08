// Command server is the gRPC entrypoint for team-engagement (EngagementService).
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
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/buidangphuc/team-engagement/internal/bootstrap"
	"github.com/buidangphuc/team-engagement/internal/config"
	"github.com/buidangphuc/team-engagement/internal/consumer"
	"github.com/buidangphuc/team-engagement/internal/grpcserver"
	"github.com/buidangphuc/team-engagement/internal/handler"
	"github.com/buidangphuc/team-engagement/internal/observability"
	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited with error", slog.Any("err", err))
		os.Exit(1)
	}
}

func run() error {
	settings, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	logger := newLogger(settings)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observability.InitTracer(ctx, settings)
	if err != nil {
		return fmt.Errorf("init tracer: %w", err)
	}
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(sctx); err != nil {
			logger.Warn("tracer shutdown", slog.Any("err", err))
		}
	}()

	res, err := bootstrap.OpenResources(ctx, settings, logger)
	if err != nil {
		return fmt.Errorf("open resources: %w", err)
	}
	defer func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := bootstrap.CloseResources(cctx, res); err != nil {
			logger.Warn("close resources", slog.Any("err", err))
		}
	}()

	orderClient, err := upstream.NewOrderClient(settings.Upstream.OrderAddr, time.Duration(settings.Upstream.CallTimeoutSeconds*float64(time.Second)))
	if err != nil {
		return fmt.Errorf("dial team-order: %w", err)
	}
	if orderClient != nil {
		defer func() { _ = orderClient.Close() }()
		logger.Info("verified-purchase enrichment enabled", slog.String("order_addr", settings.Upstream.OrderAddr))
	} else {
		logger.Info("UPSTREAM_ORDER_ADDR unset; verified-purchase enrichment disabled")
	}

	reviewRepo := repository.NewPostgresReviewRepository(res.Pool)
	var reviewVerifier service.OrderVerifier
	if orderClient != nil {
		reviewVerifier = orderClient
	}
	reviewSvc := service.NewReviewService(reviewRepo, reviewVerifier, logger)
	qaRepo := repository.NewPostgresQARepository(res.Pool)
	qaSvc := service.NewQAService(qaRepo, logger)
	disputeRepo := repository.NewPostgresDisputeRepository(res.Pool)
	disputeSvc := service.NewDisputeService(disputeRepo, logger)
	collectionRepo := repository.NewPostgresCollectionRepository(res.Pool)
	collectionSvc := service.NewCollectionService(collectionRepo, logger)
	engagementRepo := repository.NewPostgresRepository(res.Pool)
	h := handler.NewEngagementHandler(engagementRepo, reviewSvc, qaSvc, disputeSvc, collectionSvc)
	srv := grpcserver.Build(settings, h, res.Health, logger)

	addr := net.JoinHostPort(settings.Server.Host, strconv.Itoa(settings.Server.Port))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("engagement gRPC server listening", slog.String("addr", addr))
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	// listing.events consumer: fills the follow feed. A fatal consumer error (a
	// record that can neither be handled nor parked to the DLQ) stops the service
	// so the orchestrator restarts it and the group resumes from its last commit.
	consumerErr := make(chan error, 1)
	if settings.Kafka.Enabled {
		cons, err := consumer.New(settings.KafkaBrokers(), settings.Kafka.ConsumerGroup, settings.Kafka.ListingTopic)
		if err != nil {
			return fmt.Errorf("kafka consumer: %w", err)
		}
		defer cons.Close()
		logger.Info("follow-feed consumer started",
			slog.String("topic", settings.Kafka.ListingTopic),
			slog.String("group", settings.Kafka.ConsumerGroup))
		go func() {
			// Run returns nil on shutdown; only an unexpected stop is reported.
			if err := cons.Run(ctx, consumer.ListingEventHandler(engagementRepo), logger); err != nil || ctx.Err() == nil {
				consumerErr <- err
			}
		}()
	} else {
		logger.Info("KAFKA_ENABLED=false; follow feed is not fed from listing.events")
	}

	select {
	case err := <-consumerErr:
		if err == nil {
			err = errors.New("follow-feed consumer stopped")
		} else {
			err = fmt.Errorf("follow-feed consumer: %w", err)
		}
		gracefulStop(srv, settings.Server.ShutdownGrace)
		<-serveErr
		return err
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining")
		res.Health.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		gracefulStop(srv, settings.Server.ShutdownGrace)
		return <-serveErr
	}
}

func gracefulStop(srv *grpc.Server, graceSeconds float64) {
	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Duration(graceSeconds * float64(time.Second))):
		srv.Stop()
	}
}

func newLogger(s *config.Settings) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(s.Runtime.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if s.Runtime.LogJSON {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h)
}
