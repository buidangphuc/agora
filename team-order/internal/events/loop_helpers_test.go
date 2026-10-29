package events_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-order/internal/config"
)

const loopWait = 120 * time.Second

type loopEnv struct {
	brokers string
	s       config.Settings
	fp      *kafkax.Fingerprinter
}

func newLoopEnv(t *testing.T) *loopEnv {
	t.Helper()
	brokers := testkit.Redpanda(t)
	id := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	fp, err := kafkax.NewFingerprinter("order-events-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	var s config.Settings
	s.Kafka = sdkconfig.Kafka{Brokers: brokers}
	s.Events = config.Events{
		PaymentTopic:    "payment.events." + id,
		PaymentGroup:    "team-order.payment." + id,
		PaymentDLQ:      "payment.events.dlq." + id,
		OrderTopic:      "order.events",
		EngagementTopic: "engagement.events." + id,
		DisputeGroup:    "team-order.dispute." + id,
		EngagementDLQ:   "engagement.events.order.dlq." + id,
	}
	return &loopEnv{brokers: brokers, s: s, fp: fp}
}

func (e *loopEnv) client(t *testing.T, group, topic string) *kgo.Client {
	t.Helper()
	c, err := kafkax.NewConsumerClient(e.s.Kafka, group, []string{topic},
		kgo.AllowAutoTopicCreation(), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatalf("consumer client: %v", err)
	}
	return c
}

type running struct {
	cancel context.CancelFunc
	done   chan error
	client *kgo.Client
}

func start(t *testing.T, c *consume.Consumer, client *kgo.Client) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan error, 1), client: client}
	go func() { r.done <- c.Run(ctx) }()
	t.Cleanup(func() { _ = r.stop(t) })
	return r
}

func (r *running) stop(t *testing.T) error {
	t.Helper()
	r.cancel()
	select {
	case err := <-r.done:
		r.done <- err
		r.client.Close()
		return err
	case <-time.After(60 * time.Second):
		t.Error("consumer did not stop")
		return nil
	}
}

func (r *running) exited() (error, bool) {
	select {
	case err := <-r.done:
		r.done <- err
		return err, true
	default:
		return nil, false
	}
}

type counting struct {
	n atomic.Int32
	h consume.Handler
}

func count(h consume.Handler) *counting {
	c := &counting{h: h}
	return c
}

func (c *counting) handle(ctx context.Context, r consume.Record) error {
	c.n.Add(1)
	return c.h(ctx, r)
}

func (c *counting) calls() int { return int(c.n.Load()) }

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLogs(t *testing.T) (*slog.Logger, *syncBuffer) {
	t.Helper()
	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })
	return logger, buf
}

func (e *loopEnv) requireCommittedPast(t *testing.T, group, topic, key string, sentinel []byte) {
	t.Helper()
	testkit.Produce(t, e.brokers, topic, key, sentinel)
	client := e.client(t, group, topic)
	var mu sync.Mutex
	var seen [][]byte
	c, err := consume.NewConsumer(client, func(_ context.Context, r consume.Record) error {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, append([]byte(nil), r.Value...))
		return nil
	}, testkit.NewRecorder(), consume.Options{DLQTopic: "unused.dlq"})
	if err != nil {
		t.Fatalf("verification consumer: %v", err)
	}
	r := start(t, c, client)
	testkit.Eventually(t, loopWait, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, v := range seen {
			if bytes.Equal(v, sentinel) {
				return true
			}
		}
		return false
	})
	_ = r.stop(t)
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("a restarted consumer in the group saw %d records, want only the sentinel: the earlier record was not committed", len(seen))
	}
}
