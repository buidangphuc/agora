package events_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

func newDisputeLoop(t *testing.T, e *loopEnv, d *dEnv, dlq *testkit.Recorder) (*running, *counting) {
	t.Helper()
	h := count(d.h.Handle)
	client := e.client(t, e.s.Events.DisputeGroup, e.s.Events.EngagementTopic)
	c, err := events.NewDisputeConsumer(client, h.handle, dlq, e.fp, e.s)
	if err != nil {
		t.Fatalf("NewDisputeConsumer: %v", err)
	}
	return start(t, c, client), h
}

// Scenario: An audit outage is retried without dead-lettering
func TestDisputeRunRetriesTransientWithoutDLQ(t *testing.T) {
	e := newLoopEnv(t)
	d := newDEnv(t)
	d.order("o-1", true)
	d.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)
	d.audit.failN = 12
	testkit.Produce(t, e.brokers, e.s.Events.EngagementTopic, "d-1", buyerFavour("evt-1", "r-1", "o-1").raw(t))
	dlq := testkit.NewRecorder()

	run, _ := newDisputeLoop(t, e, d, dlq)
	testkit.Eventually(t, loopWait, func() bool { return d.status("r-1") == store.ReturnStatusRefunded })

	if got := len(dlq.Records()); got != 0 {
		t.Fatalf("DLQ received %d records, want 0", got)
	}
	if got := d.audit.n(); got != 13 {
		t.Fatalf("audit attempts = %d, want 13 (12 failures, then success)", got)
	}
	for i, gap := range d.audit.gaps() {
		if gap > 5*time.Second+time.Second {
			t.Fatalf("retry delay %d = %v exceeds the 5s cap", i+1, gap)
		}
	}
	if rows := d.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
	if err := run.stop(t); err != nil {
		t.Fatalf("consumer stopped with %v", err)
	}
	e.requireCommittedPast(t, e.s.Events.DisputeGroup, e.s.Events.EngagementTopic, "d-1", []byte("sentinel"))
}

// Scenario: A return that no longer fits the order is parked at once
func TestDisputeRefundNoRemainderParkedByConsumer(t *testing.T) {
	e := newLoopEnv(t)
	d := newDEnv(t)
	d.order("o-1", true)
	d.ret("r-other", "o-1", 700000, store.ReturnStatusApproved)
	d.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)
	value := buyerFavour("evt-1", "r-1", "o-1").raw(t)
	testkit.Produce(t, e.brokers, e.s.Events.EngagementTopic, "d-1", value)
	dlq := testkit.NewRecorder()

	run, h := newDisputeLoop(t, e, d, dlq)
	testkit.Eventually(t, loopWait, func() bool { return len(dlq.Records()) == 1 })

	if got := h.calls(); got != 1 {
		t.Fatalf("handler ran %d times, want 1", got)
	}
	got := dlq.Records()[0]
	if got.Topic != e.s.Events.EngagementDLQ || !bytes.Equal(got.Value, value) {
		t.Fatalf("DLQ record = topic %q, want the original record on %q", got.Topic, e.s.Events.EngagementDLQ)
	}
	if st := d.status("r-1"); st != store.ReturnStatusRejected {
		t.Fatalf("return status = %d, want REJECTED", st)
	}
	if err := run.stop(t); err != nil {
		t.Fatalf("consumer stopped with %v", err)
	}
}

// Scenario: The offset is not committed when the DLQ write fails (dispute)
func TestDisputeNoCommitWhenDLQFails(t *testing.T) {
	e := newLoopEnv(t)
	d := newDEnv(t)
	poison := []byte{0xff, 0xff, 0xff}
	testkit.Produce(t, e.brokers, e.s.Events.EngagementTopic, "d-1", poison)

	failing := testkit.NewRecorder()
	failing.FailOn("d-1")
	first, _ := newDisputeLoop(t, e, d, failing)
	testkit.Eventually(t, loopWait, func() bool {
		exited, _ := first.exited()
		return exited
	})
	if _, err := first.exited(); err == nil {
		t.Fatal("the consumer must stop with an error when the DLQ write fails")
	}
	_ = first.stop(t)

	working := testkit.NewRecorder()
	second, h := newDisputeLoop(t, e, d, working)
	testkit.Eventually(t, loopWait, func() bool { return len(working.Records()) == 1 })
	if got := working.Records()[0]; !bytes.Equal(got.Value, poison) {
		t.Fatalf("redelivered record = %q, want the original", got.Value)
	}
	if got := h.calls(); got != 1 {
		t.Fatalf("redelivery ran the handler %d times, want 1", got)
	}
	_ = second.stop(t)
}
