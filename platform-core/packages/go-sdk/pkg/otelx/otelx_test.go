package otelx

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

func restoreGlobals(t *testing.T) {
	t.Helper()
	tp, mp, prop := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(tp)
		otel.SetMeterProvider(mp)
		otel.SetTextMapPropagator(prop)
	})
}

func TestInitDisabledIsNoop(t *testing.T) {
	restoreGlobals(t)
	otel.SetTracerProvider(noop.NewTracerProvider())

	shutdown, err := Init(context.Background(), config.OTel{Enabled: false}, "svc")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
		t.Fatal("global tracer provider is an SDK provider while disabled")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if len(otel.GetTextMapPropagator().Fields()) == 0 {
		t.Fatal("propagator not installed")
	}
}

func TestInitEnabledUsesServiceFallback(t *testing.T) {
	restoreGlobals(t)

	shutdown, err := Init(context.Background(),
		config.OTel{Enabled: true, Endpoint: "http://127.0.0.1:1"}, "team-x")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = shutdown(ctx) // no collector exists; the flush error is expected
	})

	_, span := otel.Tracer("test").Start(context.Background(), "op")
	rw, ok := span.(sdktrace.ReadOnlySpan)
	if !ok {
		t.Fatalf("span is %T, want an SDK span", span)
	}
	got, found := rw.Resource().Set().Value(attribute.Key("service.name"))
	if !found || got.AsString() != "team-x" {
		t.Fatalf("service.name = %v (found=%v), want team-x", got, found)
	}
}
