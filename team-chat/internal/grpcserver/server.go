package grpcserver

import (
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	"github.com/buidangphuc/team-chat/internal/config"
	"github.com/buidangphuc/team-chat/internal/handler"
	"github.com/buidangphuc/team-chat/internal/interceptor"
)

func Build(s *config.Settings, h *handler.ChatHandler, healthSrv *health.Server, logger *slog.Logger) *grpc.Server {
	srv := grpc.NewServer(
		grpc.StatsHandler(interceptor.OTelStatsHandler()),
		grpc.UnaryInterceptor(interceptor.AuthUnaryInterceptor()),
	)

	chatv1.RegisterChatServiceServer(srv, h)

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
