package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/buidangphuc/team-domain/internal/events"
	"github.com/buidangphuc/team-domain/internal/repository"
)

// fakeProducer is a RawProducer whose produce result is configurable.
type fakeProducer struct {
	err   error
	calls int
}

func (p *fakeProducer) ProduceRaw(_ context.Context, _ string, _ []byte) error {
	p.calls++
	return p.err
}

func newRelayer(store events.OutboxClaimer, prod events.RawProducer) *events.Relayer {
	return events.NewRelayer(store, prod, nil, events.RelayerConfig{
		BatchSize:   10,
		LockSeconds: 60,
		MaxAttempts: 10,
		BaseBackoff: time.Second,
		MaxBackoff:  time.Minute,
	})
}

func TestRelayer_MarksPublishedOnSuccess(t *testing.T) {
	store := repository.NewInMemoryOutbox(
		repository.PendingEvent{EventID: "e1", AggregateID: "l1", Payload: []byte("env")},
	)
	prod := &fakeProducer{}
	now := time.Now()

	rep, err := newRelayer(store, prod).RelayOnce(context.Background(), now)
	if err != nil {
		t.Fatalf("RelayOnce: %v", err)
	}
	if rep.Claimed != 1 || rep.Published != 1 || rep.Retried != 0 || rep.Failed != 0 {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if prod.calls != 1 {
		t.Fatalf("want 1 produce call, got %d", prod.calls)
	}
	if _, ok := store.Published["e1"]; !ok {
		t.Fatalf("e1 should be marked published, published=%v", store.Published)
	}
}

func TestRelayer_RetriesWithBackoffOnFailure(t *testing.T) {
	store := repository.NewInMemoryOutbox(
		repository.PendingEvent{EventID: "e1", AggregateID: "l1", Attempts: 0},
	)
	prod := &fakeProducer{err: errors.New("broker unavailable")}
	now := time.Now()

	rep, err := newRelayer(store, prod).RelayOnce(context.Background(), now)
	if err != nil {
		t.Fatalf("RelayOnce: %v", err)
	}
	if rep.Retried != 1 || rep.Published != 0 || rep.Failed != 0 {
		t.Fatalf("want a single retry, got %+v", rep)
	}
	// Retry schedules a backoff (attempt 1 => BaseBackoff of 1s) and does NOT park.
	next, ok := store.Retried["e1"]
	if !ok {
		t.Fatalf("e1 should be scheduled for retry, retried=%v", store.Retried)
	}
	if got := next.Sub(now); got != time.Second {
		t.Fatalf("want 1s backoff on first retry, got %v", got)
	}
	if _, parked := store.Failed["e1"]; parked {
		t.Fatalf("e1 must not be parked failed on a retryable attempt")
	}
	if store.Attempts["e1"] != 1 {
		t.Fatalf("want attempts=1, got %d", store.Attempts["e1"])
	}
}

func TestRelayer_ParksPoisonRowPastMaxAttempts(t *testing.T) {
	// Two rows: a poison row already at the attempt ceiling, and a healthy row.
	// The poison row is parked without blocking the healthy one.
	store := repository.NewInMemoryOutbox(
		repository.PendingEvent{EventID: "poison", AggregateID: "l1", Attempts: 9}, // attempt 10 == MaxAttempts
		repository.PendingEvent{EventID: "ok", AggregateID: "l2"},
	)
	// Producer fails only for the poison payload... simplest: fail all, so both
	// are exercised; the healthy row (attempts 0) retries, the poison one parks.
	prod := &fakeProducer{err: errors.New("still down")}
	now := time.Now()

	rep, err := newRelayer(store, prod).RelayOnce(context.Background(), now)
	if err != nil {
		t.Fatalf("RelayOnce: %v", err)
	}
	if rep.Claimed != 2 {
		t.Fatalf("want 2 claimed, got %d", rep.Claimed)
	}
	if _, parked := store.Failed["poison"]; !parked {
		t.Fatalf("poison row should be parked failed, failed=%v", store.Failed)
	}
	if rep.Failed != 1 {
		t.Fatalf("want 1 parked failure, got %+v", rep)
	}
	// The healthy row is not parked — it is retried, proving the poison row did
	// not block the queue.
	if _, parked := store.Failed["ok"]; parked {
		t.Fatalf("healthy row must not be parked")
	}
	if _, retried := store.Retried["ok"]; !retried {
		t.Fatalf("healthy row should be scheduled for retry, retried=%v", store.Retried)
	}
}
