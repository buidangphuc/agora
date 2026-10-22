package consumer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"
)

type fakeDLQ struct {
	keys [][]byte
	err  error
}

func (f *fakeDLQ) Produce(_ context.Context, key, _ []byte) error {
	f.keys = append(f.keys, append([]byte(nil), key...))
	return f.err
}
func (f *fakeDLQ) Close() {}

func newTestConsumer(dlq DLQProducer, attempts int) *Consumer {
	return &Consumer{dlq: dlq, cfg: RetryConfig{MaxAttempts: attempts, BaseBackoff: time.Nanosecond, MaxBackoff: time.Nanosecond}}
}

func jsonLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func wantFP(key string) string {
	s := sha256.Sum256([]byte(key))
	return hex.EncodeToString(s[:])[:12]
}

func logRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("bad log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func findMsg(recs []map[string]any, msg string) map[string]any {
	for _, r := range recs {
		if r["msg"] == msg {
			return r
		}
	}
	return nil
}

var failing = func(context.Context, []byte, []byte) error { return errors.New("boom") }

// Scenario: Engagement key redaction: handler failure logs fingerprint
func TestHandlerFailureLogsKeyFingerprint(t *testing.T) {
	var buf bytes.Buffer
	c := newTestConsumer(&fakeDLQ{}, 3)
	_ = c.processRecord(context.Background(), []byte("seller-123"), []byte("v"), failing, jsonLogger(&buf))
	rec := findMsg(logRecords(t, &buf), "event handler failed")
	if rec == nil {
		t.Fatalf("no 'event handler failed' record: %s", buf.String())
	}
	if rec["key_fp"] != wantFP("seller-123") {
		t.Fatalf("key_fp = %v, want %s", rec["key_fp"], wantFP("seller-123"))
	}
	if _, ok := rec["key"]; ok {
		t.Fatalf("raw key attribute present")
	}
	if strings.Contains(buf.String(), "seller-123") {
		t.Fatalf("raw key leaked in log output: %s", buf.String())
	}
}

// Scenario: Engagement key redaction: DLQ parking logs fingerprint
func TestDLQParkLogsFingerprintKeepsDLQKey(t *testing.T) {
	var buf bytes.Buffer
	dlq := &fakeDLQ{}
	c := newTestConsumer(dlq, 3)
	if err := c.processRecord(context.Background(), []byte("seller-123"), []byte("v"), failing, jsonLogger(&buf)); err != nil {
		t.Fatalf("park should succeed: %v", err)
	}
	rec := findMsg(logRecords(t, &buf), "event parked to dlq after max attempts")
	if rec == nil {
		t.Fatalf("no parking record: %s", buf.String())
	}
	if rec["key_fp"] != wantFP("seller-123") {
		t.Fatalf("key_fp = %v", rec["key_fp"])
	}
	if rec["attempts"] != float64(3) {
		t.Fatalf("attempts = %v", rec["attempts"])
	}
	if _, ok := rec["key"]; ok {
		t.Fatalf("raw key attribute present")
	}
	if strings.Contains(buf.String(), "seller-123") {
		t.Fatalf("raw key leaked: %s", buf.String())
	}
	if len(dlq.keys) != 1 || string(dlq.keys[0]) != "seller-123" {
		t.Fatalf("dlq keys = %q, want raw key", dlq.keys)
	}
}

// Scenario: Engagement key redaction: DLQ produce failure logs fingerprint
func TestDLQProduceFailureLogsFingerprintOffsetHeld(t *testing.T) {
	var buf bytes.Buffer
	c := newTestConsumer(&fakeDLQ{err: errors.New("dlq down")}, 3)
	err := c.processRecord(context.Background(), []byte("seller-123"), []byte("v"), failing, jsonLogger(&buf))
	if err == nil {
		t.Fatalf("expected error so offset does not advance")
	}
	rec := findMsg(logRecords(t, &buf), "dlq produce failed; offset will not advance")
	if rec == nil {
		t.Fatalf("no dlq failure record: %s", buf.String())
	}
	if rec["key_fp"] != wantFP("seller-123") {
		t.Fatalf("key_fp = %v", rec["key_fp"])
	}
	if strings.Contains(buf.String(), "seller-123") {
		t.Fatalf("raw key leaked: %s", buf.String())
	}
}

// Scenario: Engagement key redaction: empty key
func TestEmptyKeyFingerprint(t *testing.T) {
	var buf bytes.Buffer
	c := newTestConsumer(&fakeDLQ{}, 2)
	_ = c.processRecord(context.Background(), nil, []byte("v"), failing, jsonLogger(&buf))
	rec := findMsg(logRecords(t, &buf), "event handler failed")
	if rec == nil {
		t.Fatalf("no record: %s", buf.String())
	}
	if rec["key_fp"] != "e3b0c44298fc" {
		t.Fatalf("key_fp = %v, want e3b0c44298fc", rec["key_fp"])
	}
}

// Scenario: Engagement key redaction: fingerprint is exactly 12 hex and stable
func TestKeyFingerprintFormatStable(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{12}$`)
	fp := func(key string) any {
		var buf bytes.Buffer
		c := newTestConsumer(&fakeDLQ{}, 1)
		_ = c.processRecord(context.Background(), []byte(key), []byte("v"), failing, jsonLogger(&buf))
		rec := findMsg(logRecords(t, &buf), "event handler failed")
		if rec == nil {
			t.Fatalf("no record for key %q", key)
		}
		return rec["key_fp"]
	}
	a1, a2, b := fp("seller-123"), fp("seller-123"), fp("seller-456")
	long := fp(strings.Repeat("k", 200))
	if a1 != a2 {
		t.Fatalf("not stable: %v vs %v", a1, a2)
	}
	if a1 == b {
		t.Fatalf("different keys share fingerprint %v", a1)
	}
	for _, v := range []any{a1, b, long} {
		s, _ := v.(string)
		if !re.MatchString(s) {
			t.Fatalf("key_fp %v not 12 lowercase hex", v)
		}
	}
}

// Scenario: Engagement key redaction: successful records and key unchanged
func TestSuccessPassesRawKeyToHandler(t *testing.T) {
	var buf bytes.Buffer
	var got []byte
	h := func(_ context.Context, key, _ []byte) error { got = append([]byte(nil), key...); return nil }
	c := newTestConsumer(&fakeDLQ{}, 3)
	if err := c.processRecord(context.Background(), []byte("seller-123"), []byte("v"), h, jsonLogger(&buf)); err != nil {
		t.Fatal(err)
	}
	if string(got) != "seller-123" {
		t.Fatalf("handler key = %q", got)
	}
	if strings.TrimSpace(buf.String()) != "" {
		t.Fatalf("unexpected logs on success: %s", buf.String())
	}
}
