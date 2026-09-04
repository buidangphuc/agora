package grpcserver

import (
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/config"
	"github.com/buidangphuc/team-promotion/internal/handler"
	"github.com/buidangphuc/team-promotion/internal/interceptor"
)

func Build(
	s *config.Settings,
	voucherHandler *handler.VoucherHandler,
	flashSaleHandler *handler.FlashSaleHandler,
	subscriptionHandler *handler.SubscriptionHandler,
	sponsoredHandler *handler.SponsoredHandler,
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

	if voucherHandler != nil {
		promotionv1.RegisterVoucherServiceServer(srv, voucherHandler)
	}
	if flashSaleHandler != nil {
		promotionv1.RegisterFlashSaleServiceServer(srv, flashSaleHandler)
	}
	if subscriptionHandler != nil {
		promotionv1.RegisterSubscriptionServiceServer(srv, subscriptionHandler)
	}
	if sponsoredHandler != nil {
		promotionv1.RegisterSponsoredServiceServer(srv, sponsoredHandler)
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
