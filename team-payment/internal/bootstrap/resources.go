package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/buidangphuc/team-payment/internal/config"
	"github.com/buidangphuc/team-payment/internal/events"
	"github.com/buidangphuc/team-payment/internal/repository"
)

type Resources struct {
	Pool   *pgxpool.Pool
	Health *health.Server

	// Outbox is the transactional-outbox store over the pool (nil if DB disabled).
	Outbox *repository.OutboxStore
	// TxWriter settles a payment and enqueues its PaymentSettled outbox row in one
	// transaction (AD4). main.go wires it into the service via WithTxWriter. nil
	// when DB is disabled, in which case settle degrades to a status-only update.
	TxWriter repository.PaymentTxWriter

	// producer is the relayer's Kafka produce path (nil when Kafka disabled).
	producer interface {
		events.RawProducer
		Close()
	}
	// stopRelayer cancels the background outbox relayer; relayerDone signals it has
	// drained and returned.
	stopRelayer context.CancelFunc
	relayerDone chan struct{}
}

func InitResources(ctx context.Context, cfg *config.Settings, logger *slog.Logger) (*Resources, error) {
	res := &Resources{
		Health: health.NewServer(),
	}
	res.Health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	if cfg.Database.Enabled {
		poolCfg, err := pgxpool.ParseConfig(cfg.Database.URL)
		if err != nil {
			return nil, fmt.Errorf("parse db url: %w", err)
		}
		poolCfg.MaxConns = cfg.Database.MaxConns

		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		pool, err := pgxpool.NewWithConfig(pingCtx, poolCfg)
		if err != nil {
			return nil, fmt.Errorf("connect db pool: %w", err)
		}
		if err := pool.Ping(pingCtx); err != nil {
			pool.Close()
			return nil, fmt.Errorf("ping db: %w", err)
		}
		res.Pool = pool
		// The transactional outbox store + writer live over the same pool: settle
		// writes payment=PAID and the PaymentSettled row in one tx (AD4).
		res.Outbox = repository.NewOutboxStore(pool)
		res.TxWriter = repository.NewPgTxWriter(pool, res.Outbox)
		logger.Info("connected to postgres", slog.String("db", poolCfg.ConnConfig.Database))
	}

	// Outbox relayer (AD4/ADR-0002): drains pending PaymentSettled rows to Kafka
	// "payment.events". Started only when Kafka is enabled AND the outbox store is
	// present; with Kafka disabled, rows are recorded but never relayed.
	kcfg := kafkaConfigFromEnv()
	if kcfg.Enabled && res.Outbox != nil {
		producer, err := newKafkaProducer(kcfg)
		if err != nil {
			_ = CloseResources(context.Background(), res)
			return nil, fmt.Errorf("init kafka producer: %w", err)
		}
		res.producer = producer
		res.startRelayer(producer, logger)
		logger.Info("payment outbox relayer started", slog.String("topic", kcfg.Topic))
	}

	return res, nil
}

// startRelayer launches the transactional-outbox relayer in its own goroutine,
// mirroring team-domain's lifecycle. Cancelled by CloseResources, which waits on
// relayerDone so the current pass drains.
func (res *Resources) startRelayer(producer events.RawProducer, logger *slog.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	res.stopRelayer = cancel
	res.relayerDone = make(chan struct{})

	relayer := events.NewRelayer(res.Outbox, producer, logger, events.RelayerConfig{})
	go func() {
		defer close(res.relayerDone)
		relayer.Run(ctx)
	}()
}

func CloseResources(ctx context.Context, res *Resources) error {
	if res == nil {
		return nil
	}
	// Stop the relayer first — it produces via the producer and reads the pool,
	// both torn down below. Wait for it to drain its current pass.
	if res.stopRelayer != nil {
		res.stopRelayer()
		if res.relayerDone != nil {
			<-res.relayerDone
			res.relayerDone = nil
		}
		res.stopRelayer = nil
	}
	if res.producer != nil {
		res.producer.Close()
		res.producer = nil
	}
	if res.Pool != nil {
		res.Pool.Close()
	}
	return nil
}
