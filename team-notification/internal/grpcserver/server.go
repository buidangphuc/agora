package grpcserver

import (
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/handler"
	"github.com/buidangphuc/team-notification/internal/interceptor"
)

type Server struct {
	grpcServer *grpc.Server
	port       int
}

func New(port int, notiHandler *handler.NotificationHandler) *Server {
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptor.UnaryServerInterceptor()))
	notificationv1.RegisterNotificationServiceServer(srv, notiHandler)
	reflection.Register(srv)
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(srv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	return &Server{grpcServer: srv, port: port}
}

func (s *Server) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return err
	}
	return s.grpcServer.Serve(lis)
}

func (s *Server) Stop() {
	s.grpcServer.GracefulStop()
}
