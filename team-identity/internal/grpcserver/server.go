// Package grpcserver builds the wired-but-unstarted gRPC server for identity services.
package grpcserver

import (
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/handler"
	"github.com/buidangphuc/team-identity/internal/interceptor"
)

func Build(
	s *config.Settings,
	authHandler *handler.AuthHandler,
	addrHandler *handler.AddressHandler,
	sessionHandler *handler.SessionHandler,
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

	identityv1.RegisterAuthServiceServer(srv, authHandler)
	if addrHandler != nil {
		identityv1.RegisterAddressServiceServer(srv, addrHandler)
	}
	if sessionHandler != nil {
		identityv1.RegisterSessionServiceServer(srv, sessionHandler)
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
