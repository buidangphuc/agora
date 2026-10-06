package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-notification/internal/bootstrap"
	"github.com/buidangphuc/team-notification/internal/config"
	"github.com/buidangphuc/team-notification/internal/consumer"
	"github.com/buidangphuc/team-notification/internal/grpcserver"
	"github.com/buidangphuc/team-notification/internal/handler"
	"github.com/buidangphuc/team-notification/internal/repository"
	"github.com/buidangphuc/team-notification/internal/service"
	"github.com/buidangphuc/team-notification/internal/upstream"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Warn("cannot connect to postgres-notification, running in mock mode", "err", err)
	}

	repo := repository.NewPostgresNotificationRepo(pool)

	// Alert subscriptions (F3) live in Postgres; wire the use case into the gRPC
	// handler so Subscribe/Unsubscribe/List work, and reuse the same repo for the
	// listing.events consumer below.
	var handlerOpts []handler.Option
	var alertSubs repository.AlertSubscriptionRepository
	if pool != nil {
		alertSubs = repository.NewPostgresAlertSubscriptionRepo(pool)
		handlerOpts = append(handlerOpts, handler.WithAlertService(service.NewAlertService(alertSubs)))

		// Notification preferences (F3) also live in Postgres; wire the use case
		// so Get/UpdateNotificationPrefs work and the digest scheduler can query
		// recipients by cadence.
		prefsRepo := repository.NewPostgresNotificationPrefsRepo(pool)
		handlerOpts = append(handlerOpts, handler.WithPrefsService(service.NewPrefsService(prefsRepo)))
	}
	notiHandler := handler.NewNotificationHandler(repo, handlerOpts...)

	srv := grpcserver.New(cfg.GRPCPort, notiHandler)

	// listing.events consumer (F3): self-diff ListingChanged snapshots into
	// price-drop and back-in-stock (0→positive) in-app alerts for subscribed users,
	// idempotently, committing the offset only after success and dead-lettering
	// poison records (AD1/AD4). Only when Postgres is present (subscriptions live
	// there) and Kafka is enabled. Cancelled by ctx on shutdown.
	kcfg := bootstrap.KafkaConfigFromEnv()
	if pool != nil && kcfg.Enabled {
		// Consumer state is durable (processed_events, listing_last_seen): a restart
		// or redeploy neither re-notifies a redelivered event nor loses the baseline
		// needed to detect a price drop or a restock.
		dedupe := repository.NewPostgresProcessedEventRepo(pool)
		startConsumer(ctx, logger, "listing", kcfg, func() consumerRun {
			return consumer.NewListingConsumer(repo, alertSubs, logger,
				consumer.WithDeduper(dedupe),
				consumer.WithPriceStateStore(repository.NewPostgresListingPriceStore(pool)),
				consumer.WithStockStateStore(repository.NewPostgresListingStockStore(pool)),
			).Run
		})

		// chat.events + order.events (notify-chat-and-shipment): a CHAT notification
		// for the other thread participant and an ORDER notification when a shipment
		// is created, each deduped by event_id and gated by the recipient's
		// notification preferences. Same offset/DLQ discipline as the listing consumer.
		prefsSvc := service.NewPrefsService(repository.NewPostgresNotificationPrefsRepo(pool))
		// The chat consumer names the sender via team-domain (shop) and team-identity
		// (user), as a service principal. Connections are lazy and every failure
		// falls back to a neutral label, so an upstream outage never blocks delivery.
		chatConsumer := consumer.NewChatConsumer(repo, prefsSvc, logger, consumer.WithUserEventDeduper(dedupe))
		if upstreams, err := upstream.Dial(cfg.DomainAddr, cfg.IdentityAddr, logger); err != nil {
			logger.Warn("sender name lookup disabled", "err", err)
		} else {
			defer upstreams.Close()
			chatConsumer.WithOptions(consumer.WithSenderNameResolver(upstreams.Resolver))
		}
		startConsumer(ctx, logger, "chat", bootstrap.ChatKafkaConfigFromEnv(), func() consumerRun {
			return chatConsumer.Run
		})
		startConsumer(ctx, logger, "order", bootstrap.OrderKafkaConfigFromEnv(), func() consumerRun {
			return consumer.NewOrderConsumer(repo, prefsSvc, logger, consumer.WithUserEventDeduper(dedupe)).Run
		})
	}

	go func() {
		logger.Info("starting team-notification gRPC server", "port", cfg.GRPCPort)
		if err := srv.Start(); err != nil {
			logger.Error("server failed", "err", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down team-notification...")
	srv.Stop()
}

// consumerRun is a consumer's Run method.
type consumerRun func(ctx context.Context, reader consumer.RecordReader, dlq consumer.DeadLetterProducer, cfg consumer.RunConfig) error

// startConsumer joins kcfg's consumer group and runs the consumer built by mk in
// the background until ctx is cancelled. A Kafka init failure disables just that
// consumer. The Kafka handles are closed when the process exits.
func startConsumer(ctx context.Context, logger *slog.Logger, name string, kcfg bootstrap.KafkaConfig, mk func() consumerRun) {
	tk, err := bootstrap.NewTopicKafka(kcfg)
	if err != nil {
		logger.Warn(name+" consumer disabled: kafka client init failed", "err", err)
		return
	}
	run := mk()
	go func() {
		defer tk.Close()
		logger.Info(name+" consumer starting", "topic", kcfg.Topic, "group", kcfg.ConsumerGroup)
		err := run(ctx, tk.Reader(), tk.DLQ(), consumer.RunConfig{DLQTopic: kcfg.DLQTopic})
		// The loop only returns on shutdown. Any other exit leaves the service up
		// without this consumer, so make it loud.
		if ctx.Err() == nil {
			logger.Error(name+" consumer stopped while the service is running", "err", err)
		}
	}()
}
