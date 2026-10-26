package app

import (
	"context"
	"errors"
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
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-engagement/internal/config"
	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/events"
	"github.com/buidangphuc/team-engagement/internal/rpc"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

const (
	serviceName        = "team-engagement"
	outboxTable        = "engagement_outbox_events"
	publishedRetention = 7 * 24 * time.Hour
	purgeInterval      = time.Hour
	closeGrace         = 2 * time.Second
)

type Options struct {
	NewPublisher func(sdkconfig.Kafka) (outbox.Publisher, error)
	Now          func() time.Time
	SweepReports func(ctx context.Context, cutoff time.Time) (int64, error)
}

func Run(ctx context.Context, s config.Settings) error {
	return RunWithOptions(ctx, s, Options{})
}

func newKafkaPublisher(c sdkconfig.Kafka) (outbox.Publisher, error) {
	return kafkax.NewProducer(c, kgo.AllowAutoTopicCreation())
}

func RunWithOptions(ctx context.Context, s config.Settings, o Options) (err error) {
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

	orderClient, err := upstream.NewOrderClient(s.Upstream.OrderAddr)
	if err != nil {
		return err
	}
	stack.Push(func(context.Context) error { return orderClient.Close() })

	auditConn, err := auditx.Dial(sdkconfig.Audit{Addr: s.Upstream.AuditAddr}, "service-"+serviceName, "audit.write")
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	if auditConn == nil {
		return errors.New("audit: UPSTREAM_AUDIT_ADDR is required")
	}
	stack.Push(func(context.Context) error { return auditConn.Close() })
	gate := auditx.NewGate(upstream.NewAuditWriter(auditConn), logger)

	newPublisher := o.NewPublisher
	if newPublisher == nil {
		newPublisher = newKafkaPublisher
	}
	pub, err := newPublisher(s.Kafka)
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	if c, ok := pub.(interface{ Close() }); ok {
		stack.Push(func(context.Context) error { c.Close(); return nil })
	}
	dlq, ok := pub.(consume.DLQProducer)
	if !ok {
		return errors.New("kafka: publisher cannot produce to the DLQ")
	}

	outStore, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		return err
	}

	repo := store.NewPostgresRepository(pool, outStore)
	reportRepo := store.NewPostgresReportRepository(pool)
	h, err := rpc.NewHandler(
		repo,
		engagement.NewReviewService(store.NewPostgresReviewRepository(pool, outStore), orderClient, gate, logger),
		engagement.NewQAService(store.NewPostgresQARepository(pool), logger),
		engagement.NewDisputeService(store.NewPostgresDisputeRepository(pool, outStore), gate, logger),
		engagement.NewCollectionService(store.NewPostgresCollectionRepository(pool), logger),
		engagement.NewReportService(reportRepo, gate, logger),
		orderClient,
		orderClient,
		logger,
	)
	if err != nil {
		return err
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, h)

	relayer, err := outbox.NewRelayer(outStore, pub, outbox.RelayerOptions{
		Topic:  s.Events.EventsTopic,
		Config: s.Outbox,
		Logger: logger,
	})
	if err != nil {
		return err
	}

	purge, err := lifecycle.Every(purgeInterval, func(ctx context.Context) error {
		_, err := outStore.Purge(ctx, publishedRetention)
		return err
	}, lifecycle.WithLogger(logger))
	if err != nil {
		return err
	}

	now := o.Now
	if now == nil {
		now = time.Now
	}
	sweep := o.SweepReports
	if sweep == nil {
		sweep = func(ctx context.Context, cutoff time.Time) (int64, error) {
			return engagement.SweepReports(ctx, reportRepo, cutoff)
		}
	}
	retention := time.Duration(s.Reports.RetentionDays) * 24 * time.Hour
	sweeper, err := lifecycle.Every(s.Reports.SweepInterval, func(ctx context.Context) error {
		n, err := sweep(ctx, now().Add(-retention))
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "report retention sweep done", "redacted", n)
		return nil
	}, lifecycle.RunAtStart(), lifecycle.WithLogger(logger))
	if err != nil {
		return err
	}

	var fp *kafkax.Fingerprinter
	if s.LogKeyFP.Secret != "" {
		if fp, err = kafkax.NewFingerprinter(s.LogKeyFP.Secret); err != nil {
			return fmt.Errorf("log key fingerprint: %w", err)
		}
	}
	client, err := events.NewListingClient(s.Kafka, s.Events.ConsumerGroup, s.Events.ListingTopic)
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	stack.Push(func(context.Context) error { client.Close(); return nil })
	consumer, err := events.NewListingConsumer(client, repo, dlq, s.Events.ListingTopic, fp, logger)
	if err != nil {
		return fmt.Errorf("consumer %s: %w", s.Events.ConsumerGroup, err)
	}

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace + 2*closeGrace,
		CloseGrace:    closeGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, health, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), relayer, purge, sweeper, consumer))
}
