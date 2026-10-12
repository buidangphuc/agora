// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them. Read top to bottom to see what runs.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/flags"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"

	"github.com/buidangphuc/team-promotion/internal/config"
	"github.com/buidangphuc/team-promotion/internal/events"
	"github.com/buidangphuc/team-promotion/internal/promotion"
	"github.com/buidangphuc/team-promotion/internal/rpc"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/upstream"
)

const service = "team-promotion"

// repos are the five repositories the domain services read and write.
type repos struct {
	vouchers      store.VoucherRepository
	reservations  store.ReservationRepository
	flashSales    store.FlashSaleRepository
	subscriptions store.SubscriptionRepository
	adCampaigns   store.AdCampaignRepository
}

// Run wires the service and runs it until ctx ends (main cancels it on
// SIGINT/SIGTERM) or a component fails. Build order: logger, telemetry, health,
// database, publisher, flags, upstream, domain, server. Everything opened goes
// on the stack; closers run in reverse order on shutdown and on an early failure.
//
// Migrations are not applied here: the deploy migrate job owns them.
func Run(ctx context.Context, s config.Settings) error {
	logger := logx.New(s.Runtime, service)
	slog.SetDefault(logger)

	var stack lifecycle.Stack

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	stack.Push(shutdownOTel)

	health := healthx.New(healthx.DefaultInterval, logger)

	// Outside ENV=local an unreachable database fails boot. Locally the
	// in-memory repositories keep the service usable; their data lives only for
	// the process.
	r := inMemoryRepos()
	pool, err := pgx.Open(ctx, s.Database)
	switch {
	case err == nil:
		r = postgresRepos(pool)
		health.Register("postgres", pgx.Checker(pool))
		stack.Push(func(context.Context) error { pool.Close(); return nil })
	case s.Runtime.IsLocal():
		logger.WarnContext(ctx, "database unreachable, using in-memory repositories", "err", err)
	default:
		return errors.Join(fmt.Errorf("open database: %w", err), stack.Close(ctx))
	}

	publisher, err := events.NewKafkaPublisher(s.Kafka, s.Events.Topic)
	if err != nil {
		return errors.Join(fmt.Errorf("kafka publisher: %w", err), stack.Close(ctx))
	}
	stack.Push(publisher.Close)

	// A flag-system outage must not stop the service: degrade to the callers'
	// defaults.
	var evaluator flags.Evaluator
	flagClient, err := flags.New(ctx, s.Flags, logger)
	if err != nil {
		logger.WarnContext(ctx, "feature flags unavailable; using defaults", "err", err)
		evaluator = flags.Disabled()
	} else {
		evaluator = flagClient
		stack.Push(flagClient.Close)
	}

	var domain *upstream.Domain
	if addr := s.Upstream.DomainAddr; addr != "" {
		domain, err = upstream.Dial(addr, time.Duration(s.Upstream.CallTimeoutSeconds*float64(time.Second)))
		if err != nil {
			return errors.Join(fmt.Errorf("upstream domain: %w", err), stack.Close(ctx))
		}
		stack.Push(domain.Close)
	} else {
		logger.WarnContext(ctx, "UPSTREAM_DOMAIN_ADDR unset: non-admin CreateCampaign/CreateAdCampaign fail closed")
	}
	listings := upstream.Listings(domain)

	voucherSvc := promotion.NewVoucherService(r.vouchers, r.reservations, publisher, evaluator, logger)
	flashSaleSvc := promotion.NewFlashSaleService(r.flashSales, publisher, evaluator, listings, logger)
	subscriptionSvc := promotion.NewSubscriptionService(r.subscriptions, logger)
	sponsoredSvc := promotion.NewSponsoredService(r.adCampaigns, listings, logger).WithLimits(s.Ads.MaxBid, s.Ads.MaxBudget)

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.Server, Logger: logger, Health: health})
	rpc.Register(srv,
		rpc.NewVoucherHandler(voucherSvc, logger),
		rpc.NewFlashSaleHandler(flashSaleSvc, logger),
		rpc.NewSubscriptionHandler(subscriptionSvc, logger),
		rpc.NewSponsoredHandler(sponsoredSvc, logger),
	)

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.Server.ShutdownGrace(),
		DrainDelay:    s.Server.DrainDelay(),
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, grpcx.Serve(srv, s.Server), lifecycle.Func(health.Run))
}
