// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them. Read top to bottom to see what runs.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/rpc"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"
)

const (
	service       = "hello"
	probeInterval = 5 * time.Second
)

// Run loads the settings, wires the service and runs it until ctx is done or a
// component fails.
func Run(ctx context.Context) error {
	s, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := logx.New(s.Runtime, service)

	// Closers run in reverse order: the database closes before telemetry flushes.
	var closers []func(context.Context) error

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	closers = append(closers, shutdownOTel)

	health := healthx.New(probeInterval, logger)

	if s.Postgres.Enabled {
		pool, err := pgx.Open(ctx, s.Postgres)
		switch {
		case err == nil:
			health.Register("postgres", pgx.Checker(pool))
			closers = append(closers, func(context.Context) error { pool.Close(); return nil })
		case s.IsLocal():
			logger.WarnContext(ctx, "database unreachable, continuing without it", "err", err)
		default:
			return errors.Join(fmt.Errorf("open database: %w", err), shutdownOTel(ctx))
		}
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, rpc.NewHandler(logger))

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace(),
		OnShutdown:    []func(){health.Drain},
		Closers:       closers,
		Logger:        logger,
	}, grpcx.Serve(srv, s.GRPC), lifecycle.Func(health.Run))
}
