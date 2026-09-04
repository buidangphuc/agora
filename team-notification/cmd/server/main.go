package main

import (
	"context"
	"errors"
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
		lk, err := bootstrap.NewListingKafka(kcfg)
		if err != nil {
			logger.Warn("listing consumer disabled: kafka client init failed", "err", err)
		} else {
			defer lk.Close()
			listingConsumer := consumer.NewListingConsumer(repo, alertSubs, logger)
			go func() {
				logger.Info("listing consumer starting",
					"topic", kcfg.Topic, "group", kcfg.ConsumerGroup)
				if err := listingConsumer.Run(ctx, lk.Reader(), lk.DLQ(), consumer.RunConfig{DLQTopic: kcfg.DLQTopic}); err != nil &&
					!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					logger.Error("listing consumer stopped with error", "err", err)
				}
			}()
		}
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
