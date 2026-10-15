package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-verification/internal/config"
	"github.com/buidangphuc/team-verification/internal/grpcserver"
	"github.com/buidangphuc/team-verification/internal/handler"
	"github.com/buidangphuc/team-verification/internal/repository"
	"github.com/buidangphuc/team-verification/internal/service"
	"github.com/buidangphuc/team-verification/internal/upstream"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logger.Error("invalid config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// KYC submissions live in Postgres; fall back to an in-memory repo when the
	// database is unavailable so the service still boots for local/mock runs
	// (mirrors team-notification's degrade-to-mock behavior).
	var repo repository.KycRepository
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("cannot connect to postgres-verification, running in mock mode", "err", err)
		repo = repository.NewInMemoryKycRepo()
	} else {
		repo = repository.NewPostgresKycRepo(pool)
	}

	svc := service.NewVerificationService(repo)
	auditClient, err := upstream.NewAuditClient(cfg.AuditAddr, cfg.CallTimeout)
	if err != nil {
		logger.Error("cannot build audit client", "err", err)
		os.Exit(1)
	}
	var audit handler.AuditWriter // stays a nil interface when unset (no typed nil)
	if auditClient != nil {
		audit = auditClient
	} else {
		logger.Warn("UPSTREAM_AUDIT_ADDR not set: KYC reviews will be refused (fail closed)")
	}
	verificationHandler := handler.NewVerificationHandler(svc, audit)

	srv := grpcserver.New(cfg.GRPCPort, verificationHandler)

	go func() {
		logger.Info("starting team-verification gRPC server", "port", cfg.GRPCPort)
		if err := srv.Start(); err != nil {
			logger.Error("server failed", "err", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down team-verification...")
	srv.Stop()
	_ = auditClient.Close()
}
