// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"

	"github.com/your-org/team-service/internal/config"
	"github.com/your-org/team-service/internal/handler"
	"github.com/your-org/team-service/internal/repository"
	"github.com/your-org/team-service/internal/service"
)

const serviceName = "team-service"

// Run wires the service and runs it until ctx ends or a component fails.
func Run(ctx context.Context, s config.Settings) error {
	logger := logx.New(s.Runtime, serviceName)
	slog.SetDefault(logger)

	var stack lifecycle.Stack

	shutdownOTel, err := otelx.Init(ctx, s.OTel, serviceName)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	stack.Push(shutdownOTel)

	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		return errors.Join(fmt.Errorf("init health: %w", err), stack.Close(ctx))
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	h, err := handler.NewListingHandler(logger, service.NewListingService(repository.NewInMemoryListingRepository()))
	if err != nil {
		return errors.Join(fmt.Errorf("build handler: %w", err), stack.Close(ctx))
	}
	handler.Register(srv, h)

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, grpcx.Serve(srv, s.GRPC), lifecycle.Func(health.Run))
}
