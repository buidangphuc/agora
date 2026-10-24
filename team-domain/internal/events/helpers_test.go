package events_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
)

const fpSecret = "s3cret"

func wantFP(secret string, key []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(key)
	return hex.EncodeToString(m.Sum(nil))[:12]
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func captureLogger() (*slog.Logger, *syncBuf) {
	b := &syncBuf{}
	return slog.New(slog.NewJSONHandler(b, &slog.HandlerOptions{Level: slog.LevelDebug})), b
}

func parseLines(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(raw), "\n") {
		if l == "" {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("bad log line %q: %v", l, err)
		}
		out = append(out, m)
	}
	return out
}

type fakeDLQ struct {
	mu   sync.Mutex
	recs []*kgo.Record
	err  error
	got  chan struct{}
}

func newFakeDLQ(err error) *fakeDLQ { return &fakeDLQ{err: err, got: make(chan struct{}, 64)} }

func (f *fakeDLQ) PublishRecord(_ context.Context, r *kgo.Record) error {
	f.mu.Lock()
	f.recs = append(f.recs, r)
	f.mu.Unlock()
	select {
	case f.got <- struct{}{}:
	default:
	}
	return f.err
}

func (f *fakeDLQ) records() []*kgo.Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*kgo.Record(nil), f.recs...)
}

type run struct {
	t      *testing.T
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
	res    error
}

func (r *run) stop() error {
	r.cancel()
	r.once.Do(func() { r.res = <-r.done })
	return r.res
}

func produce(t *testing.T, brokers, topic string, key, value []byte) {
	t.Helper()
	c, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("producer client: %v", err)
	}
	defer c.Close()
	if err := c.ProduceSync(t.Context(), &kgo.Record{Topic: topic, Key: key, Value: value}).FirstErr(); err != nil {
		t.Fatalf("produce: %v", err)
	}
}

func startConsumer(t *testing.T, brokers, topic string, h consume.Handler, dlq consume.DLQProducer, logger *slog.Logger) *run {
	t.Helper()
	fp, err := kafkax.NewFingerprinter(fpSecret)
	if err != nil {
		t.Fatal(err)
	}
	client, err := kafkax.NewConsumerClient(config.Kafka{Brokers: brokers}, "g-"+topic, []string{topic}, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("consumer client: %v", err)
	}
	c, err := consume.NewConsumer(client, h, dlq, consume.Options{
		DLQTopic:    topic + ".dlq",
		MaxAttempts: 5,
		BaseBackoff: time.Millisecond,
		MaxBackoff:  2 * time.Millisecond,
		Logger:      logger,
		KeyFP:       fp,
	})
	if err != nil {
		client.Close()
		t.Fatalf("NewConsumer: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	r := &run{t: t, cancel: cancel, done: make(chan error, 1)}
	go func() { r.done <- c.Run(ctx) }()
	t.Cleanup(func() {
		_ = r.stop()
		client.Close()
	})
	return r
}

func (r *run) waitDLQ(d *fakeDLQ) {
	r.t.Helper()
	select {
	case <-d.got:
	case err := <-r.done:
		r.done <- err
		r.t.Fatalf("consumer stopped before the record was parked: %v", err)
	case <-r.t.Context().Done():
		r.t.Fatal("test ended before the record was parked")
	}
}

func (r *run) waitSignal(ch <-chan struct{}) {
	r.t.Helper()
	select {
	case <-ch:
	case err := <-r.done:
		r.done <- err
		r.t.Fatalf("consumer stopped before the handler ran: %v", err)
	case <-r.t.Context().Done():
		r.t.Fatal("test ended before the handler ran")
	}
}

func redpanda(t *testing.T) string {
	t.Helper()
	return testkit.Redpanda(t)
}
