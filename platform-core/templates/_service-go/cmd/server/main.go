// Command server is the gRPC entrypoint for this service.
//
// It wires the standard platform stack: auth + tracing interceptors, an OTLP
// tracer, the gRPC health service, server reflection, and one seed RPC
// (ListingService). Most of it is real; the single block that registers the
// generated handler needs `make proto` first (clearly marked below).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/your-org/team-service/internal/handler"
	"github.com/your-org/team-service/internal/interceptor"
	"github.com/your-org/team-service/internal/repository"
	"github.com/your-org/team-service/internal/service"

	// Generated — resolves only after `make proto` (see proto-vendor/README.md).
	listingv1 "github.com/your-org/team-service/generated/platform/listing/v1"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("server exited with error", slog.Any("err", err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Tracing (ADR-0004) ──────────────────────────────────────────────
	// OTLP/gRPC exporter by default; swap for stdout/none in local dev without
	// touching handler code. Endpoint from OTEL_EXPORTER_OTLP_ENDPOINT; the SDK
	// also honors OTEL_* env vars directly.
	shutdownTracing, err := initTracer(ctx, envOr("OTEL_SERVICE_NAME", "team-service"))
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			logger.Warn("tracer shutdown", slog.Any("err", err))
		}
	}()

	// ── Interceptors ────────────────────────────────────────────────────
	auth := interceptor.NewAuthConfig(os.Getenv("AUTH_BEARER_TOKEN"))

	srv := grpc.NewServer(
		// otelgrpc stats handler owns spans + W3C traceparent propagation.
		grpc.StatsHandler(interceptor.OTelStatsHandler()),
		grpc.ChainUnaryInterceptor(
			interceptor.RequestIDUnaryInterceptor(logger),
			auth.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			interceptor.RequestIDStreamInterceptor(logger),
			auth.StreamServerInterceptor(),
		),
	)

	// ── Health (serving) ────────────────────────────────────────────────
	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(srv, healthSrv)

	// ── Seed RPC registration ───────────────────────────────────────────
	// requires `make proto` — the listingv1 import and this registration only
	// compile once generated code exists under ./generated.
	repo := repository.NewInMemoryListingRepository()
	listingSvc := service.NewListingService(repo)
	listingv1.RegisterListingServiceServer(srv, handler.NewListingHandler(listingSvc))

	// ── Reflection (grpcurl / debugging) ────────────────────────────────
	reflection.Register(srv)

	// ── Listen + graceful shutdown ──────────────────────────────────────
	addr := ":" + envOr("GRPC_PORT", "50051")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("gRPC server listening", slog.String("addr", addr))
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining")
		healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		srv.GracefulStop()
		return <-serveErr
	}
}

// initTracer configures a global tracer provider with an OTLP/gRPC exporter and
// W3C trace-context propagation. It returns a shutdown func that flushes spans.
func initTracer(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	exporter, err := otlptracegrpc.New(ctx) // reads OTEL_EXPORTER_OTLP_ENDPOINT
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return tp.Shutdown, nil
}

// envOr returns the env var value or a default when unset/empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
