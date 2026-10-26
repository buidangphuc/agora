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
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/team-domain/internal/audit"
	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/events"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/objects"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/screen"
	"github.com/buidangphuc/team-domain/internal/store"

	auditv1 "github.com/buidangphuc/team-domain/generated/platform/audit/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
)

const (
	serviceName        = "team-domain"
	servicePrincipal   = "service-team-domain"
	auditScope         = "audit.write"
	outboxTable        = "outbox_events"
	publishedRetention = 7 * 24 * time.Hour
	closeGrace         = 2 * time.Second
)

var (
	purgeInterval = time.Hour
	wrapDLQ       = func(p consume.DLQProducer) consume.DLQProducer { return p }
)

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
	stack.Push(func(ctx context.Context) error {
		if err := shutdownOTel(ctx); err != nil {
			logger.WarnContext(ctx, "otel shutdown", "err", err)
		}
		return nil
	})

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

	pub, err := kafkax.NewProducer(s.Kafka, kgo.AllowAutoTopicCreation())
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	stack.Push(func(context.Context) error { pub.Close(); return nil })

	outStore, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		return err
	}

	auditConn, err := auditx.Dial(sdkconfig.Audit{Addr: s.Upstream.AuditAddr}, servicePrincipal, auditScope)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	if auditConn == nil {
		return fmt.Errorf("audit: UPSTREAM_AUDIT_ADDR is required")
	}
	stack.Push(func(context.Context) error { return auditConn.Close() })
	gate := auditx.NewGate(audit.NewWriter(auditv1.NewAuditServiceClient(auditConn)), logger)

	kw, err := screen.New(s.Screen.TermsFile)
	if err != nil {
		return fmt.Errorf("screen terms: %w", err)
	}
	aiClient, err := screen.NewAIClient(s.Upstream.AIAddr, time.Duration(s.Upstream.AITimeoutSeconds*float64(time.Second)))
	if err != nil {
		return err
	}
	stack.Push(func(context.Context) error { return aiClient.Close() })

	repo := store.NewPostgresListingRepository(pool).WithStockEvents(outStore, rpc.NewStockEventBuilder())
	svc := listing.NewListingServiceWithOutbox(repo, store.NewPgTxWriter(pool, outStore)).
		WithAudit(gate).
		WithScreener(kw, aiClient).
		WithReservationTTL(s.ReservationTTL(logger))
	sweeper := listing.NewReservationSweeper(svc, s.ReservationSweepInterval(logger), logger)

	h := rpc.NewListingHandler(svc, store.NewPostgresCategoryRepository(pool), objects.NewS3Storage(s.Storage)).
		WithStorefront(listing.NewStorefrontService(store.NewPostgresStorefrontRepository(pool))).
		WithBundles(listing.NewBundleService(store.NewPostgresBundleRepository(pool)))

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	listingv1.RegisterListingServiceServer(srv, h)

	relayer, err := outbox.NewRelayer(outStore, pub, outbox.RelayerOptions{
		Topic:  s.Events.ListingTopic,
		Config: s.Outbox,
		Logger: logger,
	})
	if err != nil {
		return err
	}

	purge, err := lifecycle.Every(purgeInterval, func(ctx context.Context) error {
		_, err := outStore.Purge(ctx, publishedRetention)
		return err
	}, lifecycle.RunAtStart(), lifecycle.WithLogger(logger))
	if err != nil {
		return err
	}

	fp, err := kafkax.NewFingerprinter(s.LogKeyFP.Secret)
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	client, err := kafkax.NewConsumerClient(s.Kafka, s.Events.IdentityGroup, []string{s.Events.IdentityTopic}, kgo.AllowAutoTopicCreation())
	if err != nil {
		return err
	}
	stack.Push(func(context.Context) error { client.Close(); return nil })
	identity := events.NewIdentityConsumer(store.NewPgTxWriter(pool, outStore), logger)
	consumer, err := consume.NewConsumer(client, identity.Handle, wrapDLQ(pub), consume.Options{
		DLQTopic: s.Events.IdentityTopic + ".dlq",
		KeyFP:    fp,
		Logger:   logger,
	})
	if err != nil {
		return err
	}

	relay := lifecycle.Func(func(ctx context.Context) error {
		err := relayer.Run(ctx)
		drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), closeGrace)
		defer cancel()
		for drainCtx.Err() == nil {
			rep, derr := relayer.RelayOnce(drainCtx)
			if derr != nil || rep.Published == 0 {
				break
			}
		}
		return err
	})

	sweep := lifecycle.Func(func(ctx context.Context) error {
		sweeper.Run(ctx)
		return nil
	})

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace + 2*closeGrace,
		CloseGrace:    closeGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, health, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), relay, sweep, purge, consumer))
}
