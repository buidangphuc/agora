package logx

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("decode %q: %v", buf.String(), err)
	}
	return rec
}

func TestHandlerAddsContextIDs(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(NewHandler(slog.NewJSONHandler(&buf, nil)))

	traceID, _ := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	spanID, _ := trace.SpanIDFromHex("0102030405060708")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
	}))
	ctx = reqctx.WithRequestID(ctx, "r1")
	ctx = reqctx.WithPrincipalID(ctx, "u1")

	log.InfoContext(ctx, "hello")

	rec := decode(t, &buf)
	if rec["request_id"] != "r1" || rec["principal_id"] != "u1" || rec["trace_id"] != traceID.String() {
		t.Fatalf("record = %v", rec)
	}
}

func TestHandlerOmitsMissingIDs(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(NewHandler(slog.NewJSONHandler(&buf, nil)))

	log.InfoContext(context.Background(), "hello")

	rec := decode(t, &buf)
	for _, k := range []string{"request_id", "principal_id", "trace_id"} {
		if _, ok := rec[k]; ok {
			t.Fatalf("key %q present in %v", k, rec)
		}
	}
}

func TestNewHonoursLevelAndFormat(t *testing.T) {
	var buf bytes.Buffer
	log := newWithWriter(&buf, Options{Level: "warn", JSON: true}, "svc")
	log.Info("dropped")
	log.Warn("kept")
	rec := decode(t, &buf)
	if rec["msg"] != "kept" || rec["service"] != "svc" {
		t.Fatalf("json record = %v", rec)
	}

	buf.Reset()
	log = newWithWriter(&buf, Options{Level: "debug", JSON: false}, "svc")
	log.Debug("shown")
	out := buf.String()
	if !strings.Contains(out, "level=DEBUG") || !strings.Contains(out, "service=svc") {
		t.Fatalf("text output = %q", out)
	}
}
