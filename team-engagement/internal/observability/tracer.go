// Package observability wires OpenTelemetry tracing (ADR-0004). The exporter is
// swappable and tracing is a no-op unless OTEL_ENABLED=true, so local dev pays
// nothing and shipping to Datadog/Tempo/Jaeger is an exporter config change.
package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/buidangphuc/team-engagement/internal/config"
)

// InitTracer sets the global W3C propagator (always, so traceparent/x-request-id
// bridge across hops) and, when OTEL is enabled, wires an OTLP/gRPC exporter +
// tracer provider. It returns a shutdown func that flushes spans.
func InitTracer(ctx context.Context, s *config.Settings) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.TraceContext{})

	noop := func(context.Context) error { return nil }
	if !s.Observability.Enabled {
		return noop, nil
	}

	opts := []otlptracegrpc.Option{}
	if s.Observability.OTLPEndpoint != "" {
		opts = append(opts, otlptracegrpc.WithEndpointURL(s.Observability.OTLPEndpoint))
	}
	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(s.Observability.ServiceName)),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}
