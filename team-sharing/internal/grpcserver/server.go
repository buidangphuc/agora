package grpcserver

import (
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	sharingv1 "github.com/buidangphuc/team-sharing/generated/platform/sharing/v1"
	"github.com/buidangphuc/team-sharing/internal/handler"
	"github.com/buidangphuc/team-sharing/internal/interceptor"
)

type Server struct {
	grpcServer *grpc.Server
	port       int
}

func New(port int, sharingHandler *handler.SharingHandler) *Server {
	srv := grpc.NewServer(grpc.UnaryInterceptor(interceptor.Unary()))
	sharingv1.RegisterSharingServiceServer(srv, sharingHandler)
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
