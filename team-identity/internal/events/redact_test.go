package events_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"

	"github.com/buidangphuc/team-identity/internal/events"
)

const fpSecret = "s3cret"

func wantFP(secret, key string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(key))
	return hex.EncodeToString(m.Sum(nil))[:12]
}

func newRedactingLogger(t *testing.T, buf *bytes.Buffer) *slog.Logger {
	t.Helper()
	fp, err := kafkax.NewFingerprinter(fpSecret)
	if err != nil {
		t.Fatalf("fingerprinter: %v", err)
	}
	inner := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(events.RedactingHandler(inner, fp))
}

func decodeLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("log line is not one JSON object: %v: %q", err, buf.String())
	}
	return m
}

// Scenario: Relay failure logs carry a fingerprint, not the user id.
func TestRelayLogsFingerprintNotUserID(t *testing.T) {
	var buf bytes.Buffer
	log := newRedactingLogger(t, &buf)

	log.Error("relay publish failed", "aggregate_id", "u-123", "event_id", "evt-1", "error", errors.New("broker unavailable"))

	line := decodeLine(t, &buf)
	if got, want := line["key_fp"], wantFP(fpSecret, "u-123"); got != want {
		t.Fatalf("key_fp = %v, want %v", got, want)
	}
	if _, ok := line["aggregate_id"]; ok {
		t.Fatalf("aggregate_id attribute must be replaced, line=%s", buf.String())
	}
	if strings.Contains(buf.String(), "u-123") {
		t.Fatalf("captured output leaks the user id: %s", buf.String())
	}
}

// Scenario: Relay failure logs carry a fingerprint, not the user id (attributes added with With).
func TestRelayLogsFingerprintNotUserID_With(t *testing.T) {
	var buf bytes.Buffer
	log := newRedactingLogger(t, &buf).With("aggregate_id", "u-123")

	log.Error("relay publish failed", "event_id", "evt-1")

	line := decodeLine(t, &buf)
	if got, want := line["key_fp"], wantFP(fpSecret, "u-123"); got != want {
		t.Fatalf("key_fp = %v, want %v", got, want)
	}
	if strings.Contains(buf.String(), "u-123") {
		t.Fatalf("captured output leaks the user id: %s", buf.String())
	}
}

// Scenario: Relay failure logs carry a fingerprint, not the user id (attributes inside groups).
func TestRelayLogsFingerprintNotUserID_Group(t *testing.T) {
	var buf bytes.Buffer
	log := newRedactingLogger(t, &buf)

	log.Error("relay publish failed", slog.Group("row", "aggregate_id", "u-123", "event_id", "evt-1"))
	log.WithGroup("relay").Error("relay publish failed", "aggregate_id", "u-123")

	out := buf.String()
	if strings.Contains(out, "u-123") {
		t.Fatalf("captured output leaks the user id: %s", out)
	}
	if n := strings.Count(out, wantFP(fpSecret, "u-123")); n != 2 {
		t.Fatalf("want fingerprint in both lines, found %d times: %s", n, out)
	}
	if strings.Contains(out, `"aggregate_id"`) {
		t.Fatalf("aggregate_id key must not survive: %s", out)
	}
}

// Scenario: Relay failure logs carry a fingerprint, not the user id (other attributes untouched).
func TestRedactingHandlerPassesOtherAttributes(t *testing.T) {
	var buf bytes.Buffer
	log := newRedactingLogger(t, &buf).With("component", "relay")

	log.Warn("retrying", "event_id", "evt-1", "attempt", 3, slog.Group("g", "topic", "identity.events"))

	line := decodeLine(t, &buf)
	if line["msg"] != "retrying" || line["level"] != "WARN" {
		t.Fatalf("msg/level changed: %s", buf.String())
	}
	if line["component"] != "relay" || line["event_id"] != "evt-1" || line["attempt"] != float64(3) {
		t.Fatalf("attributes changed: %s", buf.String())
	}
	g, _ := line["g"].(map[string]any)
	if g["topic"] != "identity.events" {
		t.Fatalf("group attribute changed: %s", buf.String())
	}
	if _, ok := line["key_fp"]; ok {
		t.Fatalf("key_fp must not appear without aggregate_id: %s", buf.String())
	}
}

// Scenario: Relay failure logs carry a fingerprint, not the user id (level gating preserved).
func TestRedactingHandlerRespectsInnerLevel(t *testing.T) {
	var buf bytes.Buffer
	fp, _ := kafkax.NewFingerprinter(fpSecret)
	h := events.RedactingHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}), fp)
	if h.Enabled(t.Context(), slog.LevelInfo) {
		t.Fatalf("handler must defer Enabled to the inner handler")
	}
	if !h.Enabled(t.Context(), slog.LevelError) {
		t.Fatalf("error level must be enabled")
	}
}
