package otelx

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

// Init installs the W3C trace-context propagator and, when enabled, OTLP/gRPC
// tracer and meter providers as the OTel globals. The returned shutdown flushes
// both providers; it is a no-op when telemetry is disabled.
func Init(ctx context.Context, c config.OTel, service string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.TraceContext{})

	if !c.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	name := c.ServiceName
	if name == "" {
		name = service
	}
	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(name)))
	if err != nil {
		return nil, err
	}

	var traceOpts []otlptracegrpc.Option
	var metricOpts []otlpmetricgrpc.Option
	if c.Endpoint != "" {
		traceOpts = append(traceOpts, otlptracegrpc.WithEndpointURL(c.Endpoint))
		metricOpts = append(metricOpts, otlpmetricgrpc.WithEndpointURL(c.Endpoint))
	}
	traceExp, err := otlptracegrpc.New(ctx, traceOpts...)
	if err != nil {
		return nil, err
	}
	metricExp, err := otlpmetricgrpc.New(ctx, metricOpts...)
	if err != nil {
		return nil, errors.Join(err, traceExp.Shutdown(ctx))
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExp), sdktrace.WithResource(res))
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)

	return func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx))
	}, nil
}
