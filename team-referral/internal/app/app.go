package app

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/config"
	"github.com/buidangphuc/team-referral/internal/handler"
	"github.com/buidangphuc/team-referral/internal/interceptor"
	"github.com/buidangphuc/team-referral/internal/repository"
	"github.com/buidangphuc/team-referral/internal/service"
)

const serviceName = "team-referral"

func Run(ctx context.Context, s config.Settings) (err error) {
	logger := logx.New(s.Runtime, serviceName)

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

	pool, err := postgres.Open(ctx, s.Postgres)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", postgres.Checker(pool))

	h := handler.NewReferralHandler(service.NewReferralService(repository.NewPostgresReferralRepo(pool)))

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health,
		Extra: []grpc.ServerOption{grpc.UnaryInterceptor(interceptor.UnaryServerInterceptor())},
	})
	referralv1.RegisterReferralServiceServer(srv, h)

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, health, grpcx.Serve(srv, s.GRPC))
}
