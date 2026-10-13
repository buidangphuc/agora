package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/events"
	"github.com/buidangphuc/team-analytics/internal/ga4relay"

	_ "github.com/marcboeker/go-duckdb" // registers the "duckdb" database/sql driver
)

// RunGA4Relay runs the GA4 purchase relay until ctx ends: it forwards paid
// orders to GA4 (Measurement Protocol). It consumes analytics.events (buyer to
// GA client id mapping) and order.events (OrderPaidEvent to purchase). It serves
// only /healthz and /readyz. Without GA4 credentials it runs disabled: probes
// ready, no consumer, no outbound calls.
func RunGA4Relay(ctx context.Context, s config.Settings) error {
	logger := logx.New(s.Runtime, service)
	slog.SetDefault(logger)

	var stack lifecycle.Stack
	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	stack.Push(shutdownOTel)

	// The consumer goroutines end with the component context. The component
	// waits for them before it returns, so the closers (Kafka client, DuckDB)
	// never run under a loop that still uses them.
	var wg sync.WaitGroup
	d := relayDeps{httpClient: &http.Client{}}
	d.newConsumer = func(ctx context.Context) error {
		c, err := buildConsumer(ctx, &s, logger, &stack)
		if err != nil {
			return err
		}
		wg.Go(func() { c.loop(ctx, logger) })
		wg.Go(func() { runPurger(ctx, purgeInterval, logger, c.purgers...) })
		return nil
	}
	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, lifecycle.Func(func(ctx context.Context) error {
		runErr := runGA4Relay(ctx, &s, logger, d)
		wg.Wait()
		return runErr
	}))
}

// deps are the seams run uses so it can be tested without Kafka or GA4.
type relayDeps struct {
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

func runGA4Relay(ctx context.Context, s *config.Settings, logger *slog.Logger, d relayDeps) error {
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
		logger.InfoContext(ctx, "ga4 relay disabled")
	}
	mu.Lock()
	ready = true
	mu.Unlock()

	var err error
	select {
	case <-ctx.Done():
	case err = <-srvErr:
	}
	shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type relayConsumer struct {
	client       *kgo.Client
	mapping      *ga4relay.MappingHandler
	relay        *ga4relay.Relay
	analyticsTop string
	orderTop     string
	logger       *slog.Logger
	purgers      []ga4relay.Purger
}

// purgeInterval is how often expired ledger and mapping rows are deleted.
const purgeInterval = time.Hour

// runPurger purges every purger immediately, then per interval, until ctx is
// done (lifecycle.Every). A failed purge is logged and does not stop the others.
func runPurger(ctx context.Context, interval time.Duration, logger *slog.Logger, purgers ...ga4relay.Purger) {
	purge := func(ctx context.Context) error {
		for _, p := range purgers {
			n, err := p.Purge(ctx)
			if err != nil {
				if ctx.Err() == nil {
					logger.WarnContext(ctx, "ga4 relay retention purge failed", "error", err.Error())
				}
				continue
			}
			logger.InfoContext(ctx, "ga4 relay retention purge", "deleted", n)
		}
		return nil
	}
	c, err := lifecycle.Every(interval, purge, lifecycle.RunAtStart(), lifecycle.WithLogger(logger))
	if err != nil {
		logger.ErrorContext(ctx, "ga4 relay retention purge disabled", "err", err)
		return
	}
	_ = c.Run(ctx)
}

// buildConsumer opens the relay's ledger and mapping store on its own DuckDB
// file and joins the relay's consumer group. Everything it opens goes on stack.
func buildConsumer(ctx context.Context, s *config.Settings, logger *slog.Logger, stack *lifecycle.Stack) (*relayConsumer, error) {
	db, err := sql.Open("duckdb", s.Warehouse.DuckDBPath)
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}
	stack.Push(func(context.Context) error { return db.Close() })
	ledger, err := ga4relay.NewSQLLedger(ctx, db, time.Now)
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", err)
	}
	mappings, err := ga4relay.NewSQLMappingStore(ctx, db, time.Now)
	if err != nil {
		return nil, fmt.Errorf("mapping store: %w", err)
	}
	client, err := kafkax.NewConsumerClient(s.Kafka.Kafka, s.GA4.RelayConsumerGroup,
		[]string{s.Kafka.AnalyticsTopic, s.Kafka.OrderTopic})
	if err != nil {
		return nil, fmt.Errorf("kafka client: %w", err)
	}
	stack.Push(func(context.Context) error { client.AllowRebalance(); client.Close(); return nil })
	sender := ga4relay.NewClient(ga4relay.ClientConfig{
		Endpoint: s.GA4.Endpoint, MeasurementID: s.GA4.MeasurementID, APISecret: s.GA4.APISecret, Logger: logger,
	})
	relay := ga4relay.NewRelay(ga4relay.RelayConfig{
		Mappings: mappings, Ledger: ledger, Sender: sender,
		DLQ:       dlqProducer{client: client, topic: s.GA4.RelayDLQTopic, group: s.GA4.RelayConsumerGroup},
		Committer: committer{client: client, logger: logger},
		Now:       time.Now,
	})
	logger.InfoContext(ctx, "ga4 relay starting",
		"group", s.GA4.RelayConsumerGroup,
		"dlq_topic", s.GA4.RelayDLQTopic)
	return &relayConsumer{
		client: client, mapping: ga4relay.NewMappingHandler(mappings), relay: relay,
		analyticsTop: s.Kafka.AnalyticsTopic, orderTop: s.Kafka.OrderTopic, logger: logger,
		purgers: []ga4relay.Purger{ledger.(ga4relay.Purger), mappings.(ga4relay.Purger)},
	}, nil
}

// loop polls and handles record by record; each handled record commits its own
// offset. The group client blocks rebalances after every poll (kafkax), so they
// are allowed again once the fetch is handled and when the loop ends.
func (c *relayConsumer) loop(ctx context.Context, logger *slog.Logger) {
	defer c.client.AllowRebalance()
	for ctx.Err() == nil {
		fetches := c.client.PollFetches(ctx)
		if ctx.Err() != nil {
			return
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			// Good records of the same fetch are already advanced by the client: still process them.
			logger.WarnContext(ctx, "kafka fetch error", "errs", errs)
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
				logger.WarnContext(ctx, "relay handle failed; retrying", "topic", rec.Topic, "err", err)
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
				}
			}
			if ctx.Err() != nil {
				return
			}
		}
		c.client.AllowRebalance()
	}
}

func (c *relayConsumer) handle(ctx context.Context, rec *kgo.Record) error {
	msg := toMessage(rec)
	switch rec.Topic {
	case c.analyticsTop:
		tr, ok, err := events.RecordFromEnvelope(rec.Value)
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
		c.logger.WarnContext(ctx, "offset commit failed; record may be redelivered",
			"topic", m.Topic, "partition", int(m.Partition), "err", err)
	}
	return nil
}

type dlqProducer struct {
	client *kgo.Client
	topic  string
	group  string
}

func (p dlqProducer) Publish(ctx context.Context, m ga4relay.Message, reason string) error {
	return p.client.ProduceSync(ctx, relayDLQRecord(p.topic, p.group, m, reason)).FirstErr()
}

// relayDLQRecord is the dead-letter record of m: its key and value with the
// kafkax dlq-* headers, the reason (invalid, 4xx, exhausted) as dlq-error.
func relayDLQRecord(topic, group string, m ga4relay.Message, reason string) *kgo.Record {
	return events.DeadLetterRecord(topic, group, consume.Record{
		Topic: m.Topic, Partition: m.Partition, Offset: m.Offset, Key: m.Key, Value: m.Value,
	}, errors.New(reason))
}
