package grpcserver

import (
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/config"
	"github.com/buidangphuc/team-order/internal/handler"
	"github.com/buidangphuc/team-order/internal/interceptor"
)

func Build(
	s *config.Settings,
	cartHandler *handler.CartHandler,
	orderHandler *handler.OrderHandler,
	healthSrv *health.Server,
	logger *slog.Logger,
) *grpc.Server {
	srv := grpc.NewServer(
		grpc.StatsHandler(interceptor.OTelStatsHandler()),
		grpc.ChainUnaryInterceptor(
			interceptor.RequestIDUnaryInterceptor(logger),
			interceptor.AuthUnaryInterceptor(),
		),
		grpc.ChainStreamInterceptor(interceptor.RequestIDStreamInterceptor(logger)),
	)

	if cartHandler != nil {
		orderv1.RegisterCartServiceServer(srv, cartHandler)
	}
	if orderHandler != nil {
		orderv1.RegisterOrderServiceServer(srv, orderHandler)
	}

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
