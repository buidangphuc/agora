package events_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	identityv1 "github.com/buidangphuc/team-domain/generated/platform/identity/v1"
	"github.com/buidangphuc/team-domain/internal/events"
)

var hex12 = regexp.MustCompile(`^[0-9a-f]{12}$`)

func failingIdentity(t *testing.T) (consume.Handler, *fakeSellerStore) {
	t.Helper()
	st := newFakeStore()
	st.seed("l1", "user-123", "published")
	st.failErr = fmt.Errorf("update seller user-123: %w", errors.New("db down"))
	return events.NewIdentityConsumer(st, discardLogger()).Handle, st
}

func suspendedValue(t *testing.T) []byte {
	return statusEvent(t, "e1", "user-123", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1)
}

// Scenario: Domain key redaction: Handler failure logs the fingerprint
func TestIdentityConsumerLogsKeyFingerprint(t *testing.T) {
	brokers := redpanda(t)
	h, _ := failingIdentity(t)
	logger, buf := captureLogger()
	dlq := newFakeDLQ(nil)
	produce(t, brokers, "identity.events.fp", []byte("user-123"), suspendedValue(t))
	r := startConsumer(t, brokers, "identity.events.fp", h, dlq, logger)
	r.waitDLQ(dlq)
	if err := r.stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := wantFP(fpSecret, []byte("user-123"))
	if !hex12.MatchString(want) {
		t.Fatalf("test fingerprint %q is not 12 hex", want)
	}
	retries := 0
	for _, m := range parseLines(t, buf.String()) {
		if _, has := m["key"]; has {
			t.Errorf("line has a raw key attribute: %v", m)
		}
		if m["msg"] == "consume: handler failed" {
			retries++
			if m["key_fp"] != want {
				t.Errorf("key_fp = %v, want %s in %v", m["key_fp"], want, m)
			}
		}
	}
	if retries == 0 {
		t.Fatal("no handler-failure warning was logged")
	}
}

// Scenario: Domain key redaction: Raw key never appears in any log line
func TestIdentityConsumerParkLogsNoRawKey(t *testing.T) {
	brokers := redpanda(t)
	h, st := failingIdentity(t)
	logger, buf := captureLogger()
	dlq := newFakeDLQ(nil)
	produce(t, brokers, "identity.events.park", []byte("user-123"), suspendedValue(t))
	r := startConsumer(t, brokers, "identity.events.park", h, dlq, logger)
	r.waitDLQ(dlq)
	if err := r.stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if st.callCount() != 5 {
		t.Errorf("store attempts = %d, want 5 before parking", st.callCount())
	}
	if strings.Contains(buf.String(), "user-123") {
		t.Errorf("raw key leaked in logs: %s", buf.String())
	}
	lines := parseLines(t, buf.String())
	if len(lines) < 2 {
		t.Fatalf("want retry warnings plus park warning, got %d lines", len(lines))
	}
	want := wantFP(fpSecret, []byte("user-123"))
	parked := false
	for _, m := range lines {
		if m["key_fp"] != want {
			t.Errorf("key_fp = %v, want %s in %v", m["key_fp"], want, m)
		}
		if m["msg"] == "consume: record dead-lettered" {
			parked = true
		}
	}
	if !parked {
		t.Error("no park warning was logged")
	}
}

// Scenario: Domain key redaction: DLQ produce failure logs the fingerprint
func TestIdentityConsumerDLQFailureLogsFingerprint(t *testing.T) {
	brokers := redpanda(t)
	h, _ := failingIdentity(t)
	logger, buf := captureLogger()
	dlq := newFakeDLQ(errors.New("dlq down for user-123"))
	produce(t, brokers, "identity.events.dlqfail", []byte("user-123"), suspendedValue(t))
	r := startConsumer(t, brokers, "identity.events.dlqfail", h, dlq, logger)
	r.waitDLQ(dlq)
	if err := r.stop(); err == nil {
		t.Fatal("Run returned nil, want the dead-letter failure")
	}

	if strings.Contains(buf.String(), "user-123") {
		t.Errorf("raw key leaked: %s", buf.String())
	}
	var errLine map[string]any
	for _, m := range parseLines(t, buf.String()) {
		if m["level"] == "ERROR" {
			errLine = m
		}
	}
	if errLine == nil {
		t.Fatal("no ERROR line")
	}
	if errLine["key_fp"] != wantFP(fpSecret, []byte("user-123")) {
		t.Errorf("key_fp = %v in %v", errLine["key_fp"], errLine)
	}
}

// Scenario: Domain key redaction: Empty key is still fingerprinted
func TestEmptyKeyIsFingerprinted(t *testing.T) {
	brokers := redpanda(t)
	h, _ := failingIdentity(t)
	logger, buf := captureLogger()
	dlq := newFakeDLQ(nil)
	produce(t, brokers, "identity.events.empty", nil, suspendedValue(t))
	r := startConsumer(t, brokers, "identity.events.empty", h, dlq, logger)
	r.waitDLQ(dlq)
	if err := r.stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := wantFP(fpSecret, nil)
	seen := 0
	for _, m := range parseLines(t, buf.String()) {
		seen++
		if m["key_fp"] != want || m["key_fp"] == "" {
			t.Errorf("key_fp = %v, want %s in %v", m["key_fp"], want, m)
		}
	}
	if seen == 0 {
		t.Fatal("no log lines")
	}
}

// Scenario: Domain key redaction: Fingerprint is stable and fixed length
func TestKeyFingerprintStableTwelveHex(t *testing.T) {
	f, err := kafkax.NewFingerprinter(fpSecret)
	if err != nil {
		t.Fatal(err)
	}
	k := bytes.Repeat([]byte("a"), 100)
	a, b := f.Of(k), f.Of(k)
	other := f.Of(bytes.Repeat([]byte("b"), 100))
	if a != b || !hex12.MatchString(a) || a != wantFP(fpSecret, k) {
		t.Errorf("a=%q b=%q, want equal 12 lowercase hex = %q", a, b, wantFP(fpSecret, k))
	}
	if other == a || !hex12.MatchString(other) {
		t.Errorf("other=%q must differ from %q and be 12 hex", other, a)
	}
}

// Scenario: Domain key redaction: The fingerprint depends on the secret
func TestKeyFingerprintDependsOnSecret(t *testing.T) {
	f1, err := kafkax.NewFingerprinter("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	f2, err := kafkax.NewFingerprinter("other")
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("user-123")
	sum := sha256.Sum256(key)
	unkeyed := hex.EncodeToString(sum[:])[:12]
	a, b := f1.Of(key), f2.Of(key)
	if a == b {
		t.Errorf("fingerprints equal under different secrets: %q", a)
	}
	if a == unkeyed || b == unkeyed {
		t.Errorf("fingerprint equals the unkeyed SHA-256 prefix %q", unkeyed)
	}
}

// Scenario: Domain key redaction: Kafka key and retry behaviour are unchanged
func TestParkedRecordKeepsOriginalKey(t *testing.T) {
	brokers := redpanda(t)
	st := newFakeStore()
	id := events.NewIdentityConsumer(st, discardLogger())
	var calls atomic.Int32
	h := func(ctx context.Context, r consume.Record) error { calls.Add(1); return id.Handle(ctx, r) }
	logger, _ := captureLogger()
	dlq := newFakeDLQ(nil)
	value := []byte{0xff, 0xff, 0xff, 0xff}
	produce(t, brokers, "identity.events.key", []byte("user-123"), value)
	r := startConsumer(t, brokers, "identity.events.key", h, dlq, logger)
	r.waitDLQ(dlq)
	if err := r.stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	recs := dlq.records()
	if calls.Load() != 1 || len(recs) != 1 {
		t.Fatalf("handler calls=%d dlq records=%d, want 1/1", calls.Load(), len(recs))
	}
	if string(recs[0].Key) != "user-123" || !bytes.Equal(recs[0].Value, value) {
		t.Errorf("dlq record = %q/%x", recs[0].Key, recs[0].Value)
	}
}

// Scenario: Domain key redaction: Successful records log no key
func TestSuccessfulRecordLogsNoKey(t *testing.T) {
	brokers := redpanda(t)
	st := newFakeStore()
	id := events.NewIdentityConsumer(st, discardLogger())
	handled := make(chan struct{}, 1)
	h := func(ctx context.Context, r consume.Record) error {
		err := id.Handle(ctx, r)
		handled <- struct{}{}
		return err
	}
	logger, buf := captureLogger()
	dlq := newFakeDLQ(nil)
	produce(t, brokers, "identity.events.ok", []byte("user-123"),
		statusEvent(t, "e1", "ghost", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1))
	r := startConsumer(t, brokers, "identity.events.ok", h, dlq, logger)
	r.waitSignal(handled)
	if err := r.stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(dlq.records()) != 0 {
		t.Fatalf("record was parked: %d", len(dlq.records()))
	}
	if strings.Contains(buf.String(), "user-123") {
		t.Errorf("raw key in logs: %s", buf.String())
	}
	for _, m := range parseLines(t, buf.String()) {
		if _, has := m["key"]; has {
			t.Errorf("line has raw key attribute: %v", m)
		}
	}
}
