// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them. Read top to bottom to see what runs.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/rpc"
	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/store"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"
)

const service = "hello"

// Run wires the service and runs it until SIGINT/SIGTERM, ctx ending, or a
// component failing. Build order: logger, telemetry, health, database, server.
// Everything opened goes on the stack; closers run in reverse order on shutdown
// and on an early failure.
func Run(ctx context.Context, s config.Settings) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := logx.New(s.Runtime, service)
	slog.SetDefault(logger)

	var stack lifecycle.Stack

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	stack.Push(shutdownOTel)

	health := healthx.New(healthx.DefaultInterval, logger)

	if s.Postgres.Enabled {
		if err := openDatabase(ctx, s, health, &stack); err != nil {
			if !s.IsLocal() {
				return errors.Join(fmt.Errorf("database: %w", err), stack.Close(ctx))
			}
			logger.WarnContext(ctx, "database unavailable, continuing without it", "err", err)
		}
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, rpc.NewHandler(logger))

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace(),
		DrainDelay:    s.DrainDelay(),
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, grpcx.Serve(srv, s.GRPC), lifecycle.Func(health.Run))
}

// openDatabase applies the migrations, opens the pool, probes it in health and
// pushes its closer. Migrations run first so the pool never sees an old schema.
func openDatabase(ctx context.Context, s config.Settings, health *healthx.Health, stack *lifecycle.Stack) error {
	migrations, err := store.Migrations()
	if err != nil {
		return err
	}
	if err := migrate.Up(ctx, s.Postgres.URL, migrations); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	pool, err := pgx.Open(ctx, s.Postgres)
	if err != nil {
		return err
	}
	health.Register("postgres", pgx.Checker(pool))
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	return nil
}
