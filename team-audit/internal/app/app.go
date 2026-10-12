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
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"

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

	health := healthx.New(healthx.DefaultInterval, logger)

	// The audit trail lives in Postgres. Outside ENV=local an unreachable
	// database fails boot; locally the in-memory store keeps the service usable,
	// and its events live only for the process.
	var events store.AuditRepository = store.NewInMemoryAuditRepo()
	pool, err := pgx.Open(ctx, s.Postgres)
	switch {
	case err == nil:
		events = store.NewPostgresAuditRepo(pool)
		health.Register("postgres", pgx.Checker(pool))
		stack.Push(func(context.Context) error { pool.Close(); return nil })
	case s.IsLocal():
		logger.WarnContext(ctx, "database unreachable, using the in-memory store", "err", err)
	default:
		return errors.Join(fmt.Errorf("open database: %w", err), stack.Close(ctx))
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, rpc.NewHandler(audit.NewService(events), logger))

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace(),
		DrainDelay:    s.DrainDelay(),
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, grpcx.Serve(srv, s.GRPC), lifecycle.Func(health.Run))
}
