package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-referral/internal/config"
	"github.com/buidangphuc/team-referral/internal/grpcserver"
	"github.com/buidangphuc/team-referral/internal/handler"
	"github.com/buidangphuc/team-referral/internal/repository"
	"github.com/buidangphuc/team-referral/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Referral data lives in Postgres. When the DB is unreachable, fall back to
	// the in-memory repository so the service still serves (mock mode) — the same
	// InMemory/Postgres split the unit tests use.
	var repo repository.ReferralRepository
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("cannot connect to postgres-referral, running in in-memory mode", "err", err)
		repo = repository.NewInMemoryReferralRepo()
	} else {
		repo = repository.NewPostgresReferralRepo(pool)
	}

	svc := service.NewReferralService(repo)
	referralHandler := handler.NewReferralHandler(svc)

	srv := grpcserver.New(cfg.GRPCPort, referralHandler)

	go func() {
		logger.Info("starting team-referral gRPC server", "port", cfg.GRPCPort)
		if err := srv.Start(); err != nil {
			logger.Error("server failed", "err", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down team-referral...")
	srv.Stop()
}
