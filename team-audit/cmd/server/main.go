package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-audit/internal/config"
	"github.com/buidangphuc/team-audit/internal/grpcserver"
	"github.com/buidangphuc/team-audit/internal/handler"
	"github.com/buidangphuc/team-audit/internal/repository"
	"github.com/buidangphuc/team-audit/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The audit trail lives in Postgres. If it is unreachable, fall back to the
	// in-memory repo so the service still starts (events are then non-durable) —
	// mirroring team-notification's mock-mode degradation.
	var repo repository.AuditRepository
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("cannot connect to postgres-audit, running in in-memory mode", "err", err)
		repo = repository.NewInMemoryAuditRepo()
	} else {
		repo = repository.NewPostgresAuditRepo(pool)
	}

	auditHandler := handler.NewAuditHandler(service.NewAuditService(repo))
	srv := grpcserver.New(cfg.GRPCPort, auditHandler)

	go func() {
		logger.Info("starting team-audit gRPC server", "port", cfg.GRPCPort)
		if err := srv.Start(); err != nil {
			logger.Error("server failed", "err", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down team-audit...")
	srv.Stop()
}
