// Command server is the gRPC entrypoint for team-domain (ListingService).
//
// It reuses the shared bootstrap: load settings -> init tracer -> open resources
// (Postgres + health) -> build the gRPC server -> serve -> graceful drain ->
// close resources, so there is one resource-lifecycle path, not a second
// ad-hoc one.
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

	"github.com/buidangphuc/team-domain/internal/audit"
	"github.com/buidangphuc/team-domain/internal/bootstrap"
	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/grpcserver"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/objects"
	"github.com/buidangphuc/team-domain/internal/observability"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/screen"
	"github.com/buidangphuc/team-domain/internal/store"
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

	// ── Tracing (ADR-0004) ──────────────────────────────────────────────
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

	// ── Resources (Postgres + health) ───────────────────────────────────
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

	// ── Wire the service and build the server ───────────────────────────
	repo := store.NewPostgresListingRepository(res.Pool)
	catRepo := store.NewPostgresCategoryRepository(res.Pool)
	store := objects.NewS3Storage(settings.Storage)

	// The service writes each listing and its domain event in one transaction
	// via the outbox (ADR-0002); the relayer publishes those rows. When the
	// outbox store is absent (DB disabled), fall back to a plain, event-less
	// service.
	var svc *listing.ListingService
	if res.Outbox != nil {
		// Reserve / release / sweep announce ListingStockChanged on the same
		// outbox, inside their own stock-change transaction.
		repo.WithStockEvents(res.Outbox, rpc.NewStockEventBuilder())
		txw := store.NewPgTxWriter(res.Pool, res.Outbox)
		svc = listing.NewListingServiceWithOutbox(repo, txw)
	} else {
		svc = listing.NewListingService(repo)
	}
	// Admin listing writes are audited in team-audit (fail closed); without an
	// address no writer is attached and admin update/delete return Unavailable.
	auditClient, err := audit.NewAuditClient(settings.Upstream.AuditAddr, time.Duration(settings.Upstream.CallTimeoutSeconds*float64(time.Second)))
	if err != nil {
		return fmt.Errorf("dial team-audit: %w", err)
	}
	if auditClient != nil {
		defer func() { _ = auditClient.Close() }()
		svc.WithAudit(auditClient)
		logger.Info("audit upstream enabled", slog.String("audit_addr", settings.Upstream.AuditAddr))
	} else {
		logger.Warn("UPSTREAM_AUDIT_ADDR unset; admin listing writes will be refused")
	}
	// Listing screening: keyword matcher plus the team-ai screener. Without an AI
	// address the AI side stays nil and publishes fail safe to pending_review.
	kw, err := screen.New(settings.Screen.TermsFile)
	if err != nil {
		return fmt.Errorf("load screen terms: %w", err)
	}
	aiClient, err := screen.NewAIClient(settings.Upstream.AIAddr, time.Duration(settings.Upstream.AITimeoutSeconds*float64(time.Second)))
	if err != nil {
		return fmt.Errorf("dial team-ai: %w", err)
	}
	if aiClient != nil {
		defer func() { _ = aiClient.Close() }()
		svc.WithScreener(kw, aiClient)
		logger.Info("ai screener enabled", slog.String("ai_addr", settings.Upstream.AIAddr))
	} else {
		svc.WithScreener(kw, nil)
		logger.Warn("UPSTREAM_AI_ADDR unset; published listings will go to review")
	}
	// Reservation timing (RESERVATION_TTL / RESERVATION_SWEEP_INTERVAL): parsed
	// once here so an invalid value is warned about (and defaulted) at boot.
	svc.WithReservationTTL(settings.ReservationTTL(logger))
	sweepInterval := settings.ReservationSweepInterval(logger)
	sfRepo := store.NewPostgresStorefrontRepository(res.Pool)
	sfSvc := listing.NewStorefrontService(sfRepo)
	bundleRepo := store.NewPostgresBundleRepository(res.Pool)
	bundleSvc := listing.NewBundleService(bundleRepo)
	h := rpc.NewListingHandler(svc, catRepo, store).
		WithStorefront(sfSvc).
		WithBundles(bundleSvc)

	// Reservation sweeper (AD3): release stock held past its TTL so a crashed
	// checkout never leaks inventory. Only meaningful with Postgres present, since
	// reservations are persisted there.
	if res.Pool != nil {
		res.StartReservationSweeper(svc, sweepInterval, logger)
	}

	srv := grpcserver.Build(settings, h, res.Health, logger)

	addr := net.JoinHostPort(settings.Server.Host, strconv.Itoa(settings.Server.Port))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("gRPC server listening", slog.String("addr", addr))
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
		res.Health.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		gracefulStop(srv, settings.Server.ShutdownGrace)
		return <-serveErr
	}
}

// gracefulStop drains in-flight RPCs, falling back to a hard stop if draining
// exceeds the configured grace period.
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
