// Command ga4-relay forwards paid orders to GA4 (Measurement Protocol). It
// consumes analytics.events (buyer -> GA client id mapping) and order.events
// (OrderPaidEvent -> purchase). It serves only /healthz and /readyz. Without
// GA4 credentials it runs disabled: probes ready, no consumer, no outbound calls.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/consumer"
	"github.com/buidangphuc/team-analytics/internal/ga4relay"
	"github.com/buidangphuc/team-analytics/internal/observability"
	_ "github.com/marcboeker/go-duckdb"
)

// deps are the seams run uses so it can be tested without Kafka or GA4.
type deps struct {
	probeListener net.Listener                    // nil => listen on GA4.RelayProbePort
	newConsumer   func(ctx context.Context) error // enabled mode: starts the consumer, returns once running
	httpClient    *http.Client                    // outbound client for GA4 (unused when disabled)
}

func newProbeMux(ready func() bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if ready() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	return mux
}

func run(ctx context.Context, s *config.Settings, logger *slog.Logger, d deps) error {
	ln := d.probeListener
	if ln == nil {
		var err error
		ln, err = net.Listen("tcp", fmt.Sprintf(":%d", s.GA4.RelayProbePort))
		if err != nil {
			return fmt.Errorf("listen probes: %w", err)
		}
	}
	var mu sync.Mutex
	ready := false
	srv := &http.Server{
		Handler:           newProbeMux(func() bool { mu.Lock(); defer mu.Unlock(); return ready }),
		ReadHeaderTimeout: 5 * time.Second,
	}
	srvErr := make(chan error, 1)
	go func() { srvErr <- srv.Serve(ln) }()

	if s.GA4RelayEnabled() {
		if err := d.newConsumer(ctx); err != nil {
			_ = srv.Close()
			return fmt.Errorf("start consumer: %w", err)
		}
	} else {
		logger.Info("ga4 relay disabled")
	}
	mu.Lock()
	ready = true
	mu.Unlock()

	var err error
	select {
	case <-ctx.Done():
	case err = <-srvErr:
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func main() {
	if err := realMain(); err != nil {
		slog.Error("ga4 relay exited with error", slog.Any("err", err))
		os.Exit(1)
	}
}

func realMain() error {
	s, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	logger := observability.NewLogger(s)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	var cleanup []func()
	d := deps{httpClient: &http.Client{}}
	d.newConsumer = func(ctx context.Context) error {
		c, closers, err := buildConsumer(ctx, s, logger)
		if err != nil {
			return err
		}
		cleanup = closers
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.loop(ctx, logger)
		}()
		return nil
	}
	runErr := run(ctx, s, logger, d)
	stop()
	wg.Wait()
	for _, f := range cleanup {
		f()
	}
	return runErr
}

type relayConsumer struct {
	client       *kgo.Client
	mapping      *ga4relay.MappingHandler
	relay        *ga4relay.Relay
	analyticsTop string
	orderTop     string
	logger       *slog.Logger
}

func buildConsumer(ctx context.Context, s *config.Settings, logger *slog.Logger) (*relayConsumer, []func(), error) {
	var closers []func()
	fail := func(err error) (*relayConsumer, []func(), error) {
		for _, f := range closers {
			f()
		}
		return nil, nil, err
	}
	db, err := sql.Open("duckdb", s.Warehouse.DuckDBPath)
	if err != nil {
		return fail(fmt.Errorf("open duckdb: %w", err))
	}
	closers = append(closers, func() { _ = db.Close() })
	ledger, err := ga4relay.NewSQLLedger(ctx, db, time.Now)
	if err != nil {
		return fail(fmt.Errorf("ledger: %w", err))
	}
	mappings, err := ga4relay.NewSQLMappingStore(ctx, db, time.Now)
	if err != nil {
		return fail(fmt.Errorf("mapping store: %w", err))
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(s.KafkaBrokers()...),
		kgo.ConsumerGroup(s.GA4.RelayConsumerGroup),
		kgo.ConsumeTopics(s.Kafka.AnalyticsTopic, s.Kafka.OrderTopic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return fail(fmt.Errorf("kafka client: %w", err))
	}
	closers = append(closers, client.Close)
	sender := ga4relay.NewClient(ga4relay.ClientConfig{
		Endpoint: s.GA4.Endpoint, MeasurementID: s.GA4.MeasurementID, APISecret: s.GA4.APISecret, Logger: logger,
	})
	relay := ga4relay.NewRelay(ga4relay.RelayConfig{
		Mappings: mappings, Ledger: ledger, Sender: sender,
		DLQ:       dlqProducer{client: client, topic: s.GA4.RelayDLQTopic},
		Committer: committer{client: client, logger: logger},
		Now:       time.Now,
	})
	logger.Info("ga4 relay starting",
		slog.String("group", s.GA4.RelayConsumerGroup),
		slog.String("dlq_topic", s.GA4.RelayDLQTopic))
	return &relayConsumer{
		client: client, mapping: ga4relay.NewMappingHandler(mappings), relay: relay,
		analyticsTop: s.Kafka.AnalyticsTopic, orderTop: s.Kafka.OrderTopic, logger: logger,
	}, closers, nil
}

func (c *relayConsumer) loop(ctx context.Context, logger *slog.Logger) {
	for ctx.Err() == nil {
		fetches := c.client.PollFetches(ctx)
		if ctx.Err() != nil {
			return
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			// Good records of the same fetch are already advanced by the client: still process them.
			logger.Warn("kafka fetch error", slog.Any("errs", errs))
		}
		iter := fetches.RecordIter()
		for !iter.Done() {
			rec := iter.Next()
			// Transient errors are retried in place: the offset is committed only on success.
			for ctx.Err() == nil {
				err := c.handle(ctx, rec)
				if err == nil {
					break
				}
				logger.Warn("relay handle failed; retrying", slog.String("topic", rec.Topic), slog.Any("err", err))
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
				}
			}
			if ctx.Err() != nil {
				return
			}
		}
	}
}

func (c *relayConsumer) handle(ctx context.Context, rec *kgo.Record) error {
	msg := toMessage(rec)
	switch rec.Topic {
	case c.analyticsTop:
		tr, ok, err := consumer.RecordFromEnvelope(rec.Value)
		if err == nil && ok {
			if err := c.mapping.Handle(ctx, *tr); err != nil {
				return err
			}
		}
		// Non-tracking or undecodable records carry no mapping: skip.
		return committer{client: c.client, logger: c.logger}.Commit(ctx, msg)
	case c.orderTop:
		return c.relay.Handle(ctx, msg)
	}
	return committer{client: c.client, logger: c.logger}.Commit(ctx, msg)
}

func toMessage(r *kgo.Record) ga4relay.Message {
	return ga4relay.Message{Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Key: r.Key, Value: r.Value}
}

// committer commits one record's offset. A commit failure (e.g. after a
// rebalance) is logged, not returned: the ledger makes redelivery idempotent,
// so the handler must not be re-run for it.
type committer struct {
	client *kgo.Client
	logger *slog.Logger
}

func (c committer) Commit(ctx context.Context, m ga4relay.Message) error {
	// Epoch -1 = unknown: a hand-built epoch of 0 would look like data loss to kgo on restart.
	var err error
	c.client.CommitOffsetsSync(ctx,
		map[string]map[int32]kgo.EpochOffset{m.Topic: {m.Partition: {Epoch: -1, Offset: m.Offset + 1}}},
		func(_ *kgo.Client, _ *kmsg.OffsetCommitRequest, _ *kmsg.OffsetCommitResponse, e error) { err = e })
	if err != nil && ctx.Err() == nil {
		c.logger.Warn("offset commit failed; record may be redelivered",
			slog.String("topic", m.Topic), slog.Int("partition", int(m.Partition)), slog.Any("err", err))
	}
	return nil
}

type dlqProducer struct {
	client *kgo.Client
	topic  string
}

func (p dlqProducer) Publish(ctx context.Context, m ga4relay.Message, reason string) error {
	rec := &kgo.Record{
		Topic: p.topic, Key: m.Key, Value: m.Value,
		Headers: []kgo.RecordHeader{
			{Key: "error", Value: []byte(reason)},
			{Key: "source_topic", Value: []byte(m.Topic)},
		},
	}
	return p.client.ProduceSync(ctx, rec).FirstErr()
}
