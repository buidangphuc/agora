package events_test

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

type unavailableOrders struct {
	events.OrderStore
	calls int
}

func (u *unavailableOrders) GetOrder(_ context.Context, id string) (store.Order, error) {
	u.calls++
	return store.Order{}, errors.New("get order " + id + ": rpc error: code = Unavailable desc = transport is closing")
}

func newPaymentLoop(t *testing.T, e *loopEnv, h *counting, dlq *testkit.Recorder) *running {
	t.Helper()
	client := e.client(t, e.s.Events.PaymentGroup, e.s.Events.PaymentTopic)
	c, err := events.NewPaymentConsumer(client, h.handle, dlq, e.fp, e.s)
	if err != nil {
		t.Fatalf("NewPaymentConsumer: %v", err)
	}
	return start(t, c, client)
}

// Scenario: A transient failure is retried and then parked
func TestPaymentTransientFailureParksAfterFiveAttempts(t *testing.T) {
	e := newLoopEnv(t)
	r := newRig(t)
	orders := &unavailableOrders{OrderStore: r.orders}
	h := count(events.NewPaymentHandler(orders, r.pool, quietLogger()).Handle)
	dlq := testkit.NewRecorder()
	value := settledEnvelope(t, "evt-77", paidSettlement("order-77"))
	testkit.Produce(t, e.brokers, e.s.Events.PaymentTopic, "order-77", value)

	run := newPaymentLoop(t, e, h, dlq)
	testkit.Eventually(t, loopWait, func() bool { return len(dlq.Records()) == 1 })

	if got := h.calls(); got != 5 {
		t.Fatalf("handler ran %d times, want 5", got)
	}
	got := dlq.Records()[0]
	if got.Topic != e.s.Events.PaymentDLQ {
		t.Fatalf("DLQ topic = %q, want %q", got.Topic, e.s.Events.PaymentDLQ)
	}
	if string(got.Key) != "order-77" {
		t.Fatalf("DLQ key = %q, want order-77", got.Key)
	}
	if err := run.stop(t); err != nil {
		t.Fatalf("consumer stopped with %v", err)
	}
	e.requireCommittedPast(t, e.s.Events.PaymentGroup, e.s.Events.PaymentTopic, "order-77", []byte("sentinel"))
}

// Scenario: A poison record is parked at once
func TestPaymentPoisonRecordParkedAtOnce(t *testing.T) {
	e := newLoopEnv(t)
	r := newRig(t)
	h := count(events.NewPaymentHandler(r.orders, r.pool, quietLogger()).Handle)
	dlq := testkit.NewRecorder()
	poison := []byte("not a valid envelope \xff\xff")
	testkit.Produce(t, e.brokers, e.s.Events.PaymentTopic, "order-9", poison)

	run := newPaymentLoop(t, e, h, dlq)
	testkit.Eventually(t, loopWait, func() bool { return len(dlq.Records()) == 1 })

	if got := h.calls(); got != 1 {
		t.Fatalf("handler ran %d times for a poison record, want 1", got)
	}
	got := dlq.Records()[0]
	if got.Topic != e.s.Events.PaymentDLQ || !bytes.Equal(got.Value, poison) || string(got.Key) != "order-9" {
		t.Fatalf("DLQ record = topic %q key %q value %q, want the record unchanged on %q", got.Topic, got.Key, got.Value, e.s.Events.PaymentDLQ)
	}
	if err := run.stop(t); err != nil {
		t.Fatalf("consumer stopped with %v", err)
	}
}

// Scenario: The offset is not committed when the DLQ write fails (payment)
func TestPaymentNoCommitWhenDLQFails(t *testing.T) {
	e := newLoopEnv(t)
	r := newRig(t)
	poison := []byte("not a valid envelope \xff\xff")
	testkit.Produce(t, e.brokers, e.s.Events.PaymentTopic, "order-5", poison)

	failing := testkit.NewRecorder()
	failing.FailOn("order-5")
	first := newPaymentLoop(t, e, count(events.NewPaymentHandler(r.orders, r.pool, quietLogger()).Handle), failing)
	testkit.Eventually(t, loopWait, func() bool {
		exited, _ := first.exited()
		return exited
	})
	if _, err := first.exited(); err == nil {
		t.Fatal("the consumer must stop with an error when the DLQ write fails")
	}
	_ = first.stop(t)

	h := count(events.NewPaymentHandler(r.orders, r.pool, quietLogger()).Handle)
	working := testkit.NewRecorder()
	second := newPaymentLoop(t, e, h, working)
	testkit.Eventually(t, loopWait, func() bool { return len(working.Records()) == 1 })
	if got := working.Records()[0]; !bytes.Equal(got.Value, poison) {
		t.Fatalf("redelivered record = %q, want the original", got.Value)
	}
	if got := h.calls(); got != 1 {
		t.Fatalf("redelivery ran the handler %d times, want 1", got)
	}
	_ = second.stop(t)
}

// Scenario: Consumer logs carry a key fingerprint and not the key
func TestPaymentConsumerLogsKeyFingerprintOnly(t *testing.T) {
	logger, logs := captureLogs(t)
	e := newLoopEnv(t)
	r := newRig(t)
	orders := &unavailableOrders{OrderStore: r.orders}
	h := count(events.NewPaymentHandler(orders, r.pool, logger).Handle)
	dlq := testkit.NewRecorder()
	testkit.Produce(t, e.brokers, e.s.Events.PaymentTopic, "order-77", settledEnvelope(t, "evt-fp", paidSettlement("order-77")))

	run := newPaymentLoop(t, e, h, dlq)
	testkit.Eventually(t, loopWait, func() bool { return len(dlq.Records()) == 1 })
	if err := run.stop(t); err != nil {
		t.Fatalf("consumer stopped with %v", err)
	}

	wantFP := "key_fp=" + e.fp.Of([]byte("order-77"))
	if !regexp.MustCompile(`key_fp=[0-9a-f]{12}\b`).MatchString(wantFP) {
		t.Fatalf("fingerprint %q is not 12 hex characters", wantFP)
	}
	var about int
	for _, line := range strings.Split(logs.String(), "\n") {
		if line == "" {
			continue
		}
		if strings.Contains(line, "order-77") {
			t.Errorf("a log line contains the record key: %s", line)
		}
		if strings.Contains(line, "consume:") {
			about++
			if !strings.Contains(line, wantFP) {
				t.Errorf("a consumer log line lacks %s: %s", wantFP, line)
			}
		}
	}
	if about == 0 {
		t.Fatal("no consumer log line was captured")
	}
}
