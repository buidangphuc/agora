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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-notification/internal/events"
)

const (
	wiringTopic = "chat.events"
	wiringDLQ   = "chat.events.dlq"
	wiringGroup = "team-notification.chat"
)

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

type wiring struct {
	pool *pgxpool.Pool
	dlq  *testkit.Recorder
	logs *syncBuffer
	fp   *kafkax.Fingerprinter
	key  string
}

func startChatConsumer(t *testing.T, brokers string, pool *pgxpool.Pool, wrap func(consume.Handler) consume.Handler) *wiring {
	t.Helper()
	w := &wiring{pool: pool, dlq: testkit.NewRecorder(), logs: &syncBuffer{}, key: "buyer-1"}
	fp, err := kafkax.NewFingerprinter("wiring-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	w.fp = fp
	logger := slog.New(slog.NewTextHandler(w.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := config.Kafka{Brokers: brokers}
	client, err := kafkax.NewConsumerClient(cfg, wiringGroup, []string{wiringTopic},
		kgo.AllowAutoTopicCreation(), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatalf("consumer client: %v", err)
	}
	h := events.NewChatHandler(w.pool, nil, logger)
	if wrap != nil {
		h = wrap(h)
	}
	c, err := consume.NewConsumer(client, h, w.dlq, consume.Options{
		DLQTopic:    wiringDLQ,
		MaxAttempts: 1000,
		BaseBackoff: 5 * time.Millisecond,
		MaxBackoff:  10 * time.Millisecond,
		Logger:      logger,
		KeyFP:       fp,
	})
	if err != nil {
		t.Fatalf("consumer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Error("consumer did not stop")
		}
		client.Close()
	})
	return w
}

func TestMalformedRecordGoesToDLQAndAdvances(t *testing.T) {
	brokers := testkit.Redpanda(t)
	poison := []byte("not a proto \xff\xff")
	testkit.Produce(t, brokers, wiringTopic, "buyer-1", poison)
	testkit.Produce(t, brokers, wiringTopic, "buyer-1", chatEnvelope(t, "evt-after-poison", sellerReply()))

	w := startChatConsumer(t, brokers, newDB(t), nil)

	testkit.Eventually(t, 60*time.Second, func() bool { return totalNotifications(t, w.pool) == 1 })
	one(t, w.pool, "buyer-1")
	recs := w.dlq.Records()
	if len(recs) != 1 {
		t.Fatalf("want exactly 1 dead-lettered record, got %d", len(recs))
	}
	if recs[0].Topic != wiringDLQ {
		t.Errorf("dlq topic = %q, want %q", recs[0].Topic, wiringDLQ)
	}
	if !bytes.Equal(recs[0].Value, poison) {
		t.Errorf("dead-lettered value = %q, want the original bytes", recs[0].Value)
	}
}

func TestTransientFailureRetriesBeforeCommit(t *testing.T) {
	brokers := testkit.Redpanda(t)
	testkit.Produce(t, brokers, wiringTopic, "buyer-1", chatEnvelope(t, "evt-transient", sellerReply()))

	var attempts atomic.Int32
	w := failingChatConsumer(t, brokers, &attempts)

	testkit.Eventually(t, 60*time.Second, func() bool { return attempts.Load() >= 3 })
	if got := totalNotifications(t, w.pool); got != 0 {
		t.Fatalf("nothing may be written while the failure persists, got %d", got)
	}
	if got := len(w.dlq.Records()); got != 0 {
		t.Fatalf("a transient failure must be retried, not dead-lettered; dlq has %d", got)
	}
	healInserts(t, w.pool)

	testkit.Eventually(t, 60*time.Second, func() bool { return totalNotifications(t, w.pool) == 1 })
	one(t, w.pool, "buyer-1")
	if got := len(w.dlq.Records()); got != 0 {
		t.Fatalf("dlq has %d records, want 0", got)
	}
	if got := ledgerRows(t, w.pool, "evt-transient"); got != 1 {
		t.Fatalf("ledger rows = %d, want 1", got)
	}
}

func failingChatConsumer(t *testing.T, brokers string, attempts *atomic.Int32) *wiring {
	t.Helper()
	pool := newDB(t)
	failInserts(t, pool)
	return startChatConsumer(t, brokers, pool, func(inner consume.Handler) consume.Handler {
		return func(ctx context.Context, r consume.Record) error {
			attempts.Add(1)
			return inner(ctx, r)
		}
	})
}

func TestConsumerLogsKeyFingerprintNotKey(t *testing.T) {
	brokers := testkit.Redpanda(t)
	testkit.Produce(t, brokers, wiringTopic, "buyer-1", chatEnvelope(t, "evt-logs", sellerReply()))

	var attempts atomic.Int32
	w := failingChatConsumer(t, brokers, &attempts)

	want := w.fp.Of([]byte("buyer-1"))
	testkit.Eventually(t, 60*time.Second, func() bool {
		return attempts.Load() >= 2 && strings.Contains(w.logs.String(), want)
	})
	healInserts(t, w.pool)
	testkit.Eventually(t, 60*time.Second, func() bool { return totalNotifications(t, w.pool) == 1 })

	logs := w.logs.String()
	if !strings.Contains(logs, "key_fp="+want) {
		t.Errorf("logs must carry key_fp=%s", want)
	}
	if strings.Contains(logs, "buyer-1") {
		t.Errorf("logs must never contain the record key or a user id taken from the event:\n%s", logs)
	}
}
