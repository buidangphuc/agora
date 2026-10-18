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
)

var hex12 = regexp.MustCompile(`^[0-9a-f]{12}$`)

func fp(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:])[:12] }

func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func parseLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
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

var errBoom = errors.New("boom")

func failing(context.Context, []byte, []byte) error { return errBoom }

// Scenario: Domain key redaction: Handler failure logs the fingerprint
func TestProcessRecordLogsKeyFingerprint(t *testing.T) {
	logger, buf := captureLogger()
	c := testConsumer(&fakeDLQ{})
	if err := c.processRecord(context.Background(), []byte("user-123"), []byte("v"), failing, logger); err != nil {
		t.Fatalf("err = %v", err)
	}
	lines := parseLines(t, buf)
	if len(lines) == 0 {
		t.Fatal("no log lines")
	}
	want := fp([]byte("user-123"))
	for _, m := range lines {
		if _, has := m["key"]; has {
			t.Errorf("line has raw key attribute: %v", m)
		}
		got, _ := m["key_fp"].(string)
		if got != want || !hex12.MatchString(got) {
			t.Errorf("key_fp = %q, want %q in %v", got, want, m)
		}
	}
}

// Scenario: Domain key redaction: Raw key never appears in any log line
func TestProcessRecordParkLogsNoRawKey(t *testing.T) {
	logger, buf := captureLogger()
	dlq := &fakeDLQ{}
	c := testConsumer(dlq)
	if err := c.processRecord(context.Background(), []byte("user-123"), []byte("v"), failing, logger); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(dlq.records) != 1 {
		t.Fatalf("dlq = %d, want 1", len(dlq.records))
	}
	if strings.Contains(buf.String(), "user-123") {
		t.Errorf("raw key leaked in logs: %s", buf.String())
	}
	lines := parseLines(t, buf)
	if len(lines) < 2 {
		t.Fatalf("want retry warnings plus park warning, got %d lines", len(lines))
	}
	want := fp([]byte("user-123"))
	for _, m := range lines {
		if m["key_fp"] != want {
			t.Errorf("key_fp = %v, want %s in %v", m["key_fp"], want, m)
		}
	}
}

type failDLQ struct{}

func (failDLQ) Produce(context.Context, []byte, []byte) error { return errors.New("dlq down") }
func (failDLQ) Close()                                         {}

// Scenario: Domain key redaction: DLQ produce failure logs the fingerprint
func TestProcessRecordDLQFailureLogsFingerprint(t *testing.T) {
	logger, buf := captureLogger()
	c := testConsumer(failDLQ{})
	if err := c.processRecord(context.Background(), []byte("user-123"), []byte("v"), failing, logger); err == nil {
		t.Fatal("want non-nil error when DLQ produce fails")
	}
	if strings.Contains(buf.String(), "user-123") {
		t.Errorf("raw key leaked: %s", buf.String())
	}
	var errLine map[string]any
	for _, m := range parseLines(t, buf) {
		if m["level"] == "ERROR" {
			errLine = m
		}
	}
	if errLine == nil {
		t.Fatal("no ERROR line")
	}
	if errLine["key_fp"] != fp([]byte("user-123")) {
		t.Errorf("key_fp = %v in %v", errLine["key_fp"], errLine)
	}
}

// Scenario: Domain key redaction: Empty key is still fingerprinted
func TestKeyFingerprintEmptyKey(t *testing.T) {
	if got := keyFingerprint(nil); got != "e3b0c44298fc" {
		t.Errorf("keyFingerprint(nil) = %q, want e3b0c44298fc", got)
	}
	logger, buf := captureLogger()
	c := testConsumer(&fakeDLQ{})
	_ = c.processRecord(context.Background(), nil, []byte("v"), failing, logger)
	lines := parseLines(t, buf)
	if len(lines) == 0 {
		t.Fatal("no log lines")
	}
	for _, m := range lines {
		if m["key_fp"] != "e3b0c44298fc" {
			t.Errorf("key_fp = %v in %v", m["key_fp"], m)
		}
	}
}

// Scenario: Domain key redaction: Fingerprint is stable and fixed length
func TestKeyFingerprintStableAndTwelveHex(t *testing.T) {
	k := bytes.Repeat([]byte("a"), 100)
	a, b := keyFingerprint(k), keyFingerprint(k)
	other := keyFingerprint(bytes.Repeat([]byte("b"), 100))
	if a != b || !hex12.MatchString(a) {
		t.Errorf("a=%q b=%q, want equal 12 lowercase hex", a, b)
	}
	if other == a || !hex12.MatchString(other) {
		t.Errorf("other=%q must differ from %q and be 12 hex", other, a)
	}
}

// Scenario: Domain key redaction: Kafka key and retry behaviour are unchanged
func TestProcessRecordDLQKeepsOriginalKey(t *testing.T) {
	dlq := &fakeDLQ{}
	c := testConsumer(dlq)
	var calls int
	h := func(context.Context, []byte, []byte) error { calls++; return ErrPermanent }
	logger, _ := captureLogger()
	if err := c.processRecord(context.Background(), []byte("user-123"), []byte("val"), h, logger); err != nil {
		t.Fatalf("err = %v", err)
	}
	if calls != 1 || len(dlq.records) != 1 {
		t.Fatalf("calls=%d dlq=%d, want 1/1", calls, len(dlq.records))
	}
	if string(dlq.records[0][0]) != "user-123" || string(dlq.records[0][1]) != "val" {
		t.Errorf("dlq record = %q/%q", dlq.records[0][0], dlq.records[0][1])
	}
}

// Scenario: Domain key redaction: Successful records log no key
func TestProcessRecordSuccessLogsNothing(t *testing.T) {
	logger, buf := captureLogger()
	c := testConsumer(&fakeDLQ{})
	ok := func(context.Context, []byte, []byte) error { return nil }
	if err := c.processRecord(context.Background(), []byte("user-123"), []byte("v"), ok, logger); err != nil {
		t.Fatalf("err = %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("want empty log, got %s", buf.String())
	}
}
