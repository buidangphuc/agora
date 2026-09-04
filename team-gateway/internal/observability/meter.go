package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/buidangphuc/team-gateway/internal/config"
)

// InitMeter mirrors InitTracer (ADR-0004): the exporter is swappable and the
// meter is a no-op unless OTEL_ENABLED=true. When enabled it wires an OTLP/gRPC
// metric exporter behind a periodic reader and installs the MeterProvider
// globally, so the otelgrpc client instrumentation on the upstream dials
// (internal/upstream) emits per-service RED metrics that flow gateway →
// otel-collector (OTLP) → Prometheus exporter :8889 → Prometheus. When disabled
// the global MeterProvider stays the SDK no-op, so local dev pays nothing and
// shipping to a different backend is an exporter config change, not code.
func InitMeter(ctx context.Context, s *config.Settings) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }
	if !s.Observability.Enabled {
		return noop, nil
	}

	opts := []otlpmetricgrpc.Option{}
	if s.Observability.OTLPEndpoint != "" {
		opts = append(opts, otlpmetricgrpc.WithEndpointURL(s.Observability.OTLPEndpoint))
	}
	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(s.Observability.ServiceName)),
	)
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)
	return mp.Shutdown, nil
}
