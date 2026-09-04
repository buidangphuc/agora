package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-sharing/internal/config"
	"github.com/buidangphuc/team-sharing/internal/grpcserver"
	"github.com/buidangphuc/team-sharing/internal/handler"
	"github.com/buidangphuc/team-sharing/internal/repository"
	"github.com/buidangphuc/team-sharing/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Share links live in Postgres. When the DB is unreachable, fall back to the
	// in-memory repo so the service still starts (mirrors team-notification's
	// mock-mode degradation) — short links then live only for the process.
	var repo repository.ShareLinkRepository
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("cannot connect to postgres-sharing, running in in-memory mode", "err", err)
		repo = repository.NewInMemoryShareLinkRepo()
	} else {
		defer pool.Close()
		repo = repository.NewPostgresShareLinkRepo(pool)
	}

	sharingHandler := handler.NewSharingHandler(service.NewShareService(repo))
	srv := grpcserver.New(cfg.GRPCPort, sharingHandler)

	go func() {
		logger.Info("starting team-sharing gRPC server", "port", cfg.GRPCPort)
		if err := srv.Start(); err != nil {
			logger.Error("server failed", "err", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down team-sharing...")
	srv.Stop()
}
