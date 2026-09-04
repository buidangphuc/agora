package consumer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// fakeDLQ records the records parked to the dead-letter topic and can be made to
// fail its produce to simulate an unreachable DLQ.
type fakeDLQ struct {
	mu         sync.Mutex
	records    [][2][]byte
	produceErr error
}

func (f *fakeDLQ) Produce(_ context.Context, key, value []byte) error {
	if f.produceErr != nil {
		return f.produceErr
	}
	f.mu.Lock()
	f.records = append(f.records, [2][]byte{key, value})
	f.mu.Unlock()
	return nil
}
func (f *fakeDLQ) Close() {}

func testConsumer(dlq DLQProducer) *Consumer {
	return &Consumer{
		dlq: dlq,
		cfg: RetryConfig{MaxAttempts: 3, BaseBackoff: time.Microsecond, MaxBackoff: time.Millisecond},
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// AD1 / SA-C1: a permanently failing record is retried MaxAttempts times, then
// parked to the DLQ, and only then may the offset advance (processRecord == nil).
func TestProcessRecord_ParksToDLQAfterMaxAttempts(t *testing.T) {
	dlq := &fakeDLQ{}
	c := testConsumer(dlq)

	var calls int
	h := func(_ context.Context, _, _ []byte) error {
		calls++
		return errors.New("sink down")
	}

	err := c.processRecord(context.Background(), []byte("k"), []byte("v"), h, discardLogger())
	if err != nil {
		t.Fatalf("expected nil (offset may advance after park), got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 handler attempts, got %d", calls)
	}
	if len(dlq.records) != 1 {
		t.Fatalf("expected 1 record parked to dlq, got %d", len(dlq.records))
	}
	if got := string(dlq.records[0][0]); got != "k" {
		t.Errorf("dlq key = %q, want %q", got, "k")
	}
	if got := string(dlq.records[0][1]); got != "v" {
		t.Errorf("dlq value = %q, want %q", got, "v")
	}
}

// AD1 / SA-C1: when the record can neither be handled nor parked (DLQ down), the
// offset must NOT advance — processRecord returns an error and no record is lost.
func TestProcessRecord_DLQFailureBlocksAdvance(t *testing.T) {
	dlq := &fakeDLQ{produceErr: errors.New("dlq unreachable")}
	c := testConsumer(dlq)

	h := func(_ context.Context, _, _ []byte) error { return errors.New("sink down") }

	err := c.processRecord(context.Background(), []byte("k"), []byte("v"), h, discardLogger())
	if err == nil {
		t.Fatal("expected error so the offset does not advance past the failed record")
	}
	if len(dlq.records) != 0 {
		t.Fatalf("expected no record recorded on dlq failure, got %d", len(dlq.records))
	}
}

// A record the handler accepts is committed immediately with no retry and no DLQ.
func TestProcessRecord_SuccessNoDLQ(t *testing.T) {
	dlq := &fakeDLQ{}
	c := testConsumer(dlq)

	var calls int
	h := func(_ context.Context, _, _ []byte) error {
		calls++
		return nil
	}

	if err := c.processRecord(context.Background(), []byte("k"), []byte("v"), h, discardLogger()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 handler attempt, got %d", calls)
	}
	if len(dlq.records) != 0 {
		t.Fatalf("expected no dlq records on success, got %d", len(dlq.records))
	}
}

// A retry that eventually succeeds does not reach the DLQ.
func TestProcessRecord_RecoversBeforeDLQ(t *testing.T) {
	dlq := &fakeDLQ{}
	c := testConsumer(dlq)

	var calls int
	h := func(_ context.Context, _, _ []byte) error {
		calls++
		if calls < 2 {
			return errors.New("transient")
		}
		return nil
	}

	if err := c.processRecord(context.Background(), []byte("k"), []byte("v"), h, discardLogger()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 handler attempts, got %d", calls)
	}
	if len(dlq.records) != 0 {
		t.Fatalf("expected no dlq records when handler recovers, got %d", len(dlq.records))
	}
}
