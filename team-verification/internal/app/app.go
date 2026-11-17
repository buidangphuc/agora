package app

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/mockdata"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-verification/internal/config"
	"github.com/buidangphuc/team-verification/internal/events"
	"github.com/buidangphuc/team-verification/internal/objects"
	"github.com/buidangphuc/team-verification/internal/retention"
	"github.com/buidangphuc/team-verification/internal/rpc"
	"github.com/buidangphuc/team-verification/internal/store"
	"github.com/buidangphuc/team-verification/internal/upstream"
	"github.com/buidangphuc/team-verification/internal/verification"
)

const (
	serviceName   = "team-verification"
	kycRetention  = 90 * 24 * time.Hour
	sweepInterval = time.Hour
)

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

	objs, err := objects.NewMinio(s.Storage)
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}

	pub, err := kafkax.NewProducer(s.Kafka, kgo.AllowAutoTopicCreation())
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	stack.Push(func(context.Context) error { pub.Close(); return nil })

	client, err := kafkax.NewConsumerClient(s.Kafka, s.Events.IdentityGroup, []string{s.Events.IdentityTopic}, kgo.AllowAutoTopicCreation())
	if err != nil {
		return err
	}
	stack.Push(func(context.Context) error { client.Close(); return nil })
	erased := events.NewUserErasedConsumer(pool, objs, logger)
	consumer, err := consume.NewConsumer(client, erased.Handle, pub, consume.Options{
		DLQTopic: s.Events.IdentityDLQTopic,
		Logger:   logger,
	})
	if err != nil {
		return err
	}

	sweeper := retention.NewSweeper(pool, objs, retention.Config{Retention: kycRetention, Interval: sweepInterval}, logger)
	sweep, err := lifecycle.Every(sweepInterval, func(ctx context.Context) error {
		_, err := sweeper.SweepOnce(ctx, time.Now())
		return err
	}, lifecycle.RunAtStart(), lifecycle.WithLogger(logger))
	if err != nil {
		return err
	}

	svc := verification.NewVerificationService(store.NewPostgresKycRepo(pool), verification.WithObjects(objs))
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
	}, health, lifecycle.StopAfter(grpcx.Serve(srv, grpcCfg), sweep, consumer))
}
