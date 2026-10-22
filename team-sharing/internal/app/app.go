package app

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-sharing/internal/config"
	"github.com/buidangphuc/team-sharing/internal/rpc"
	"github.com/buidangphuc/team-sharing/internal/sharelink"
	"github.com/buidangphuc/team-sharing/internal/store"
)

const service = "team-sharing"

func Run(ctx context.Context, s config.Settings) error {
	return run(ctx, s, nil)
}

func RunWithRepository(ctx context.Context, s config.Settings, repo sharelink.Repository) error {
	return run(ctx, s, repo)
}

func run(ctx context.Context, s config.Settings, repo sharelink.Repository) (err error) {
	logger := logx.New(s.Runtime, service)

	var stack lifecycle.Stack
	defer func() {
		if err != nil {
			_ = stack.Close(context.WithoutCancel(ctx))
		}
	}()

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("otel: %w", err)
	}
	stack.Push(shutdownOTel)

	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		return err
	}

	pool, err := postgres.Open(ctx, s.Postgres)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", postgres.Checker(pool))

	if repo == nil {
		repo = store.NewPostgres(pool)
	}
	handler, err := rpc.NewHandler(sharelink.NewService(repo), logger)
	if err != nil {
		return err
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{
		Config: s.GRPC, Logger: logger, Health: health,
		Extra: []grpc.ServerOption{grpc.UnaryInterceptor(dropAnonymousPrincipal)},
	})
	rpc.Register(srv, handler)

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, health, grpcx.Serve(srv, s.GRPC))
}

func dropAnonymousPrincipal(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		id, typ := first(md, "x-principal-id"), first(md, "x-principal-type")
		if id == "anonymous" || typ == "anonymous" {
			md = md.Copy()
			md.Delete("x-principal-id")
			ctx = metadata.NewIncomingContext(ctx, md)
		}
	}
	return next(ctx, req)
}

func first(md metadata.MD, key string) string {
	if v := md.Get(key); len(v) > 0 {
		return v[0]
	}
	return ""
}
