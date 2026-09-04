package grpcserver

import (
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/config"
	"github.com/buidangphuc/team-payment/internal/handler"
	"github.com/buidangphuc/team-payment/internal/interceptor"
)

func Build(
	s *config.Settings,
	paymentHandler *handler.PaymentHandler,
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

	if paymentHandler != nil {
		paymentv1.RegisterPaymentServiceServer(srv, paymentHandler)
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
