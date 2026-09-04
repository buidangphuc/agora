// Package grpcserver builds the wired-but-unstarted gRPC server for
// EngagementService (shared by main + tests).
package grpcserver

import (
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"

	"github.com/buidangphuc/team-engagement/internal/config"
	"github.com/buidangphuc/team-engagement/internal/handler"
	"github.com/buidangphuc/team-engagement/internal/interceptor"
)

func Build(s *config.Settings, h *handler.EngagementHandler, healthSrv *health.Server, logger *slog.Logger) *grpc.Server {
	srv := grpc.NewServer(
		grpc.StatsHandler(interceptor.OTelStatsHandler()),
		grpc.ChainUnaryInterceptor(
			interceptor.RequestIDUnaryInterceptor(logger),
			interceptor.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			interceptor.RequestIDStreamInterceptor(logger),
			interceptor.StreamServerInterceptor(),
		),
	)

	engagementv1.RegisterEngagementServiceServer(srv, h)

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
