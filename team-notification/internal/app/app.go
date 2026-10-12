// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them. Read top to bottom to see what runs.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"

	"github.com/buidangphuc/team-notification/internal/config"
	"github.com/buidangphuc/team-notification/internal/events"
	"github.com/buidangphuc/team-notification/internal/notification"
	"github.com/buidangphuc/team-notification/internal/rpc"
	"github.com/buidangphuc/team-notification/internal/store"
	"github.com/buidangphuc/team-notification/internal/upstream"
)

const service = "team-notification"

// Run wires the service and runs it until ctx ends (main cancels it on
// SIGINT/SIGTERM) or a component fails. Build order: logger, telemetry, health,
// database, use cases, Kafka, server. Everything opened goes on the stack;
// closers run in reverse order on shutdown and on an early failure.
//
// Migrations are not applied here: the deploy migrate job owns them.
func Run(ctx context.Context, s config.Settings) error {
	logger := logx.New(s.Runtime, service)
	slog.SetDefault(logger)

	var stack lifecycle.Stack
	fail := func(err error) error { return errors.Join(err, stack.Close(ctx)) }

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fail(fmt.Errorf("init telemetry: %w", err))
	}
	stack.Push(shutdownOTel)

	health := healthx.New(healthx.DefaultInterval, logger)

	pool, err := openDatabase(ctx, s, logger)
	if err != nil {
		return fail(err)
	}
	health.Register("postgres", pgx.Checker(pool))
	stack.Push(func(context.Context) error { pool.Close(); return nil })

	repo := store.NewPostgresNotificationRepo(pool)
	alertSubs := store.NewPostgresAlertSubscriptionRepo(pool)
	prefsSvc := notification.NewPrefsService(store.NewPostgresNotificationPrefsRepo(pool))
	handler := rpc.NewNotificationHandler(repo,
		rpc.WithAlertService(notification.NewAlertService(alertSubs)),
		rpc.WithPrefsService(prefsSvc),
	)

	// The chat consumer names the sender via team-domain (shop) and team-identity
	// (user), as a service principal. Connections are lazy and every failure falls
	// back to a neutral label, so an upstream outage never blocks delivery.
	var chatOpts []events.ChatOption
	if upstreams, err := upstream.Dial(s.Upstream.DomainAddr, s.Upstream.IdentityAddr, logger); err != nil {
		logger.WarnContext(ctx, "sender name lookup disabled", "err", err)
	} else {
		stack.Push(func(context.Context) error { upstreams.Close(); return nil })
		chatOpts = append(chatOpts, events.WithSenderNameResolver(upstreams.Resolver))
	}

	// Consumer state is durable (processed_events, listing_last_seen): a restart
	// or redeploy neither re-notifies a redelivered event nor loses the baseline
	// needed to detect a price drop or a restock.
	dedupe := store.NewPostgresProcessedEventRepo(pool)
	listing := events.NewListingConsumer(repo, alertSubs, logger,
		events.WithDeduper(dedupe),
		events.WithPriceStateStore(store.NewPostgresListingPriceStore(pool)),
		events.WithStockStateStore(store.NewPostgresListingStockStore(pool)),
	)
	chat := events.NewChatConsumer(repo, prefsSvc, logger, events.WithUserEventDeduper(dedupe)).WithOptions(chatOpts...)
	order := events.NewOrderConsumer(repo, prefsSvc, logger, events.WithUserEventDeduper(dedupe))

	dlq, err := kafkax.NewProducer(s.Kafka)
	if err != nil {
		return fail(fmt.Errorf("dead-letter producer: %w", err))
	}
	stack.Push(func(context.Context) error { dlq.Close(); return nil })

	var consumers []lifecycle.Component
	for _, c := range []struct {
		name, group, topic, dlqTopic string
		handle                       events.HandlerFunc
	}{
		{"listing", s.Listing.Group, s.Listing.Topic, s.Listing.DLQTopic, listing.HandleRaw},
		{"chat", s.Chat.Group, s.Chat.Topic, s.Chat.DLQTopic, chat.HandleRaw},
		{"order", s.Order.Group, s.Order.Topic, s.Order.DLQTopic, order.HandleRaw},
	} {
		client, err := kafkax.NewConsumerClient(s.Kafka, c.group, []string{c.topic})
		if err != nil {
			return fail(fmt.Errorf("%s consumer: %w", c.name, err))
		}
		stack.Push(func(context.Context) error { closeClient(client); return nil })
		logger.InfoContext(ctx, c.name+" consumer configured", "topic", c.topic, "group", c.group)
		consumers = append(consumers, events.NewRunner(client, c.handle, dlq, c.dlqTopic, logger))
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, handler)

	// The consumers write through the same store the server reads, so they stop
	// only after the server has drained.
	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace(),
		DrainDelay:    s.DrainDelay(),
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), consumers...), lifecycle.Func(health.Run))
}

// openDatabase opens the pool with a ping. Outside ENV=local an unreachable
// database fails boot. Locally the service starts anyway on a pool that connects
// lazily, as it always did: calls fail until the database is up.
func openDatabase(ctx context.Context, s config.Settings, logger *slog.Logger) (*pgxpool.Pool, error) {
	pool, err := pgx.Open(ctx, s.Postgres)
	if err == nil {
		return pool, nil
	}
	if !s.IsLocal() {
		return nil, fmt.Errorf("open database: %w", err)
	}
	logger.WarnContext(ctx, "database unreachable, starting with a lazily connecting pool", "err", err)
	cfg, perr := pgxpool.ParseConfig(s.Postgres.URL)
	if perr != nil {
		return nil, fmt.Errorf("open database: %w", perr)
	}
	if s.Postgres.MaxConns > 0 {
		cfg.MaxConns = s.Postgres.MaxConns
	}
	lazy, perr := pgxpool.NewWithConfig(ctx, cfg)
	if perr != nil {
		return nil, fmt.Errorf("open database: %w", perr)
	}
	return lazy, nil
}

func closeClient(c *kgo.Client) { c.Close() }
