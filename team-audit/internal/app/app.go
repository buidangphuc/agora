// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them. Read top to bottom to see what runs.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-audit/internal/audit"
	"github.com/buidangphuc/team-audit/internal/config"
	"github.com/buidangphuc/team-audit/internal/rpc"
	"github.com/buidangphuc/team-audit/internal/store"
)

const service = "team-audit"

// Run wires the service and runs it until ctx ends (main cancels it on
// SIGINT/SIGTERM) or a component fails. Build order: logger, telemetry, health,
// store, server. Everything opened goes on the stack; closers run in reverse
// order on shutdown and on an early failure.
//
// Migrations are not applied here: the deploy migrate job owns them.
func Run(ctx context.Context, s config.Settings) error {
	logger := logx.New(s.Runtime, service)
	slog.SetDefault(logger)

	var stack lifecycle.Stack

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	stack.Push(shutdownOTel)

	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		return errors.Join(fmt.Errorf("init health: %w", err), stack.Close(ctx))
	}

	// The audit trail lives in Postgres. Outside a local ENV an unreachable
	// database fails boot; locally OpenOrLocal returns no pool and the in-memory
	// store keeps the service usable, its events living only for the process.
	var events store.AuditRepository = store.NewInMemoryAuditRepo()
	pool, err := postgres.OpenOrLocal(ctx, s.Postgres, s.Runtime, logger)
	if err != nil {
		return errors.Join(fmt.Errorf("open database: %w", err), stack.Close(ctx))
	}
	if pool != nil {
		events = store.NewPostgresAuditRepo(pool)
		health.Register("postgres", postgres.Checker(pool))
		stack.Push(func(context.Context) error { pool.Close(); return nil })
	}

	handler, err := rpc.NewHandler(audit.NewService(events), logger)
	if err != nil {
		return errors.Join(fmt.Errorf("build handler: %w", err), stack.Close(ctx))
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, handler)

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, grpcx.Serve(srv, s.GRPC), lifecycle.Func(health.Run))
}
