package app

import (
	"context"
	"fmt"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/mockdata"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-verification/internal/config"
	"github.com/buidangphuc/team-verification/internal/rpc"
	"github.com/buidangphuc/team-verification/internal/store"
	"github.com/buidangphuc/team-verification/internal/upstream"
	"github.com/buidangphuc/team-verification/internal/verification"
)

const serviceName = "team-verification"

func Run(ctx context.Context, s config.Settings) (err error) {
	logger := logx.New(s.Runtime, serviceName)

	if err := s.Audit.Validate(); err != nil {
		return err
	}

	var stack lifecycle.Stack
	defer func() {
		if err != nil {
			_ = stack.Close(context.WithoutCancel(ctx))
		}
	}()

	shutdownOTel, err := otelx.Init(ctx, s.OTel, serviceName)
	if err != nil {
		return fmt.Errorf("otel: %w", err)
	}
	stack.Push(shutdownOTel)

	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		return err
	}

	mocks, err := mockdata.New(s.MockData)
	if err != nil {
		return err
	}
	mocks.LogActive(logger)

	pool, err := postgres.Open(ctx, s.Postgres)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", postgres.Checker(pool))

	auditConn, err := auditx.Dial(sdkconfig.Audit{Addr: s.Audit.Addr}, "service-"+serviceName, "audit.write")
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	stack.Push(func(context.Context) error { return auditConn.Close() })

	svc := verification.NewVerificationService(store.NewPostgresKycRepo(pool))
	h, err := rpc.NewHandler(svc, upstream.NewAuditWriter(auditConn), logger)
	if err != nil {
		return err
	}

	grpcCfg := sdkconfig.GRPC{
		Host: s.Host, Port: s.Port, ShutdownGrace: s.ShutdownGrace,
		DrainDelay: s.DrainDelay, ReflectionEnabled: s.ReflectionEnabled,
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: grpcCfg, Logger: logger, Health: health})
	rpc.Register(srv, h)

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, health, grpcx.Serve(srv, grpcCfg))
}
