// Package grpcserver builds the wired-but-unstarted gRPC server for the search
// query API, shared by the entrypoint and the in-process tests.
package grpcserver

import (
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"

	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/handler"
	"github.com/buidangphuc/team-search/internal/interceptor"
)

// Build constructs a ready-but-unstarted gRPC server wired with the platform
// stack: OTel stats handler, chained tracing -> auth interceptors, the
// SearchService handler, health, and optional reflection.
func Build(s *config.Settings, h *handler.SearchHandler, healthSrv *health.Server, logger *slog.Logger) *grpc.Server {
	srv := grpc.NewServer(
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

	searchv1.RegisterSearchServiceServer(srv, h)

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
