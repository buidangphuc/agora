// Command server is the gRPC entrypoint for team-identity (AuthService).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/buidangphuc/team-identity/internal/bootstrap"
	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/grpcserver"
	"github.com/buidangphuc/team-identity/internal/handler"
	"github.com/buidangphuc/team-identity/internal/observability"
	"github.com/buidangphuc/team-identity/internal/repository"
	"github.com/buidangphuc/team-identity/internal/service"
	"github.com/buidangphuc/team-identity/internal/token"
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

	repo := repository.NewPostgresUserRepository(res.Pool)
	addrRepo := repository.NewPostgresAddressRepository(res.Pool)
	sessionRepo := repository.NewPostgresSessionRepository(res.Pool)

	signer, err := token.NewSigner(settings.JWT.PrivateKey, settings.JWT.KID)
	if err != nil {
		return fmt.Errorf("build token signer: %w", err)
	}
	authSvc := service.NewAuthService(repo, signer, time.Duration(settings.JWT.TTLSeconds)*time.Second)

	// Seed a default admin (dev convenience).
	if err := authSvc.EnsureAdmin(ctx, "admin", "admin123"); err != nil {
		logger.Warn("seed admin", slog.Any("err", err))
	}

	srv := grpcserver.Build(
		settings,
		handler.NewAuthHandler(authSvc),
		handler.NewAddressHandler(addrRepo, logger),
		handler.NewSessionHandler(sessionRepo, logger),
		res.Health,
		logger,
	)

	addr := net.JoinHostPort(settings.Server.Host, strconv.Itoa(settings.Server.Port))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("identity gRPC server listening", slog.String("addr", addr))
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	// JWKS HTTP listener (ADR-0006): identity is otherwise gRPC-only. This tiny,
	// unauthenticated, read-only server publishes the signer's RSA public key(s)
	// at /.well-known/jwks.json so the edge can verify RS256 tokens by kid. It is
	// started alongside gRPC and drained on shutdown. No auth, no business logic.
	jwksAddr := net.JoinHostPort(settings.Server.Host, strconv.Itoa(settings.JWT.JWKSHTTPPort))
	jwksSrv := &http.Server{
		Addr:              jwksAddr,
		Handler:           token.JWKSHandler(token.PublicKey{Kid: signer.KID(), Key: signer.PublicKey()}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info("identity JWKS HTTP server listening",
			slog.String("addr", jwksAddr), slog.String("path", token.JWKSPath), slog.String("kid", signer.KID()))
		if err := jwksSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	drainJWKS := func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := jwksSrv.Shutdown(sctx); err != nil {
			logger.Warn("jwks server shutdown", slog.Any("err", err))
		}
	}

	select {
	case err := <-serveErr:
		drainJWKS()
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining")
		res.Health.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		drainJWKS()
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
