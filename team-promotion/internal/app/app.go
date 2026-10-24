package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/flags"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-promotion/internal/config"
	"github.com/buidangphuc/team-promotion/internal/events"
	"github.com/buidangphuc/team-promotion/internal/flashsale"
	"github.com/buidangphuc/team-promotion/internal/rpc"
	"github.com/buidangphuc/team-promotion/internal/sponsored"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/subscription"
	"github.com/buidangphuc/team-promotion/internal/upstream"
	"github.com/buidangphuc/team-promotion/internal/voucher"
)

const (
	serviceName        = "team-promotion"
	outboxTable        = "promotion_outbox"
	publishedRetention = 7 * 24 * time.Hour
	purgeInterval      = time.Hour
	holdPurgeInterval  = 24 * time.Hour
	holdRetention      = 30 * 24 * time.Hour
	closeGrace         = 2 * time.Second
	auditScope         = "audit.write"
	auditServicePrefix = "service-"
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

	evaluator, closeFlags := flags.NewOrDisabled(ctx, s.Flags, logger)
	stack.Push(closeFlags)

	listings, err := upstream.NewListingClient(s.Upstream.DomainAddr)
	if err != nil {
		return err
	}
	stack.Push(func(context.Context) error { return listings.Close() })

	auditConn, err := auditx.Dial(s.Audit, auditServicePrefix+serviceName, auditScope)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	if auditConn == nil {
		return errors.New("audit: UPSTREAM_AUDIT_ADDR is required")
	}
	stack.Push(func(context.Context) error { return auditConn.Close() })
	gate := auditx.NewGate(upstream.NewAuditWriter(auditConn), logger)

	pub, err := kafkax.NewProducer(s.Kafka, kgo.AllowAutoTopicCreation())
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	stack.Push(func(context.Context) error { pub.Close(); return nil })

	outStore, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		return err
	}

	topic := s.Events.Topic
	voucherEvent := func(ctx context.Context, v store.Voucher) (outbox.Row, error) {
		return events.Voucher(ctx, topic, voucherProto(v))
	}
	campaignEvent := func(ctx context.Context, c store.FlashSaleCampaign) (outbox.Row, error) {
		return events.FlashSale(ctx, topic, campaignProto(c))
	}

	holds := store.NewPostgresFlashHoldRepository(pool)
	voucherSvc := voucher.New(store.NewPostgresVoucherRepository(pool, outStore), store.NewPostgresReservationRepository(pool), gate, voucherEvent, logger)
	flashSvc := flashsale.New(store.NewPostgresFlashSaleRepository(pool, outStore), holds, evaluator, listings, gate, campaignEvent, logger)
	subSvc := subscription.New(store.NewPostgresSubscriptionRepository(pool), logger)
	adSvc := sponsored.New(store.NewPostgresAdCampaignRepository(pool), listings, gate, logger).WithLimits(s.Ads.MaxBid, s.Ads.MaxBudget)

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv,
		rpc.NewVoucherHandler(voucherSvc, logger),
		rpc.NewFlashSaleHandler(flashSvc, logger),
		rpc.NewSubscriptionHandler(subSvc, logger),
		rpc.NewSponsoredHandler(adSvc, logger),
	)

	relayer, err := outbox.NewRelayer(outStore, pub, outbox.RelayerOptions{
		Topic:  topic,
		Config: s.Outbox,
		Logger: logger,
	})
	if err != nil {
		return err
	}

	outboxPurge, err := lifecycle.Every(purgeInterval, func(ctx context.Context) error {
		_, err := outStore.Purge(ctx, publishedRetention)
		return err
	}, lifecycle.RunAtStart(), lifecycle.WithLogger(logger))
	if err != nil {
		return err
	}

	holdPurge, err := lifecycle.Every(holdPurgeInterval, func(ctx context.Context) error {
		n, err := holds.PurgeReleasedAndExpiredHolds(ctx, time.Now().Add(-holdRetention))
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "purged flash sale holds", "deleted", n)
		return nil
	}, lifecycle.RunAtStart(), lifecycle.WithLogger(logger))
	if err != nil {
		return err
	}

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace + 2*closeGrace,
		CloseGrace:    closeGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, health, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), relayer, outboxPurge, holdPurge))
}
