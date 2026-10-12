// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them. Read top to bottom to see what runs.
package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

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

// Run wires the service and runs it until ctx ends (main cancels it on
// SIGINT/SIGTERM) or a component fails. Build order: logger, telemetry, health,
// database, server. Everything opened goes on the stack; closers run in reverse
// order on shutdown and on an early failure.
func Run(ctx context.Context, s config.Settings) error {
	migrations, err := store.Migrations()
	if err != nil {
		return err
	}
	return run(ctx, s, migrations)
}

// run is Run with the migrations injected, so a test can supply a broken set.
func run(ctx context.Context, s config.Settings, migrations fs.FS) error {
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
		pool, err := pgx.Open(ctx, s.Postgres)
		switch {
		case err == nil:
			// A migration failure is fatal in every environment.
			if err := migrate.Up(ctx, s.Postgres.URL, migrations); err != nil {
				pool.Close()
				return errors.Join(fmt.Errorf("migrate: %w", err), stack.Close(ctx))
			}
			health.Register("postgres", pgx.Checker(pool))
			stack.Push(func(context.Context) error { pool.Close(); return nil })
		case s.IsLocal():
			logger.WarnContext(ctx, "database unreachable, continuing without it", "err", err)
		default:
			return errors.Join(fmt.Errorf("open database: %w", err), stack.Close(ctx))
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
