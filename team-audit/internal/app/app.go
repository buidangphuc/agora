package app

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-audit/internal/audit"
	"github.com/buidangphuc/team-audit/internal/config"
	"github.com/buidangphuc/team-audit/internal/rpc"
	"github.com/buidangphuc/team-audit/internal/store"
)

const serviceName = "team-audit"

type built struct {
	pool     *pgxpool.Pool
	health   *healthx.Health
	srv      *grpc.Server
	closeAll func(context.Context) error
}

func Migrate(ctx context.Context, s config.Settings) error {
	return migrate.Up(ctx, s.Postgres.URL, store.Migrations())
}

func build(ctx context.Context, s config.Settings) (b *built, err error) {
	logger := logx.New(s.Runtime, serviceName)

	var stack lifecycle.Stack
	defer func() {
		if err != nil {
			_ = stack.Close(context.WithoutCancel(ctx))
		}
	}()

	shutdownOTel, err := otelx.Init(ctx, s.OTel, serviceName)
	if err != nil {
		return nil, fmt.Errorf("otel: %w", err)
	}
	stack.Push(shutdownOTel)

	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		return nil, err
	}

	pool, err := postgres.Open(ctx, s.Postgres)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", postgres.Checker(pool))

	healthCtx, stopHealth := context.WithCancel(context.WithoutCancel(ctx))
	healthDone := make(chan struct{})
	go func() { _ = health.Run(healthCtx); close(healthDone) }()
	stack.Push(func(context.Context) error { stopHealth(); <-healthDone; return nil })

	h, err := rpc.NewHandler(audit.NewService(store.NewPostgres(pool)), logger)
	if err != nil {
		return nil, err
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, h)

	return &built{pool: pool, health: health, srv: srv, closeAll: stack.Close}, nil
}

func Run(ctx context.Context, s config.Settings) error {
	b, err := build(ctx, s)
	if err != nil {
		return err
	}
	serveCfg := s.GRPC
	serveCfg.ShutdownGrace = s.ShutdownGrace / 2
	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){b.health.Drain},
		Closers:       []func(context.Context) error{b.closeAll},
		Logger:        logx.New(s.Runtime, serviceName),
	}, grpcx.Serve(b.srv, serveCfg))
}
