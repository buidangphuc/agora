// Package grpcserver builds the wired-but-unstarted gRPC server so both the
// entrypoint and the in-process tests share one construction path (mirrors
// team-ai's build_grpc_server).
package grpcserver

import (
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"

	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/handler"
	"github.com/buidangphuc/team-domain/internal/interceptor"
)

// Build constructs a ready-but-unstarted gRPC server wired with the platform
// stack: the OTel stats handler, chained tracing -> auth interceptors, the
// ListingService handler, the health service, and optional reflection. If
// healthSrv is nil a always-SERVING one is created (convenient for tests).
func Build(s *config.Settings, h *handler.ListingHandler, healthSrv *health.Server, logger *slog.Logger) *grpc.Server {
	srv := grpc.NewServer(
		// otelgrpc stats handler owns spans + W3C traceparent propagation.
		grpc.StatsHandler(interceptor.OTelStatsHandler()),
		// tracing -> principal resolution (from gateway-forwarded metadata, ADR-0003).
		grpc.ChainUnaryInterceptor(
			interceptor.RequestIDUnaryInterceptor(logger),
			interceptor.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			interceptor.RequestIDStreamInterceptor(logger),
			interceptor.StreamServerInterceptor(),
		),
	)

	listingv1.RegisterListingServiceServer(srv, h)

	if healthSrv == nil {
		healthSrv = health.NewServer()
		healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	}
	healthpb.RegisterHealthServer(srv, healthSrv)

	if s.Server.ReflectionEnabled {
		reflection.Register(srv)
	}
	return srv
}
