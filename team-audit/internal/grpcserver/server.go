package grpcserver

import (
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	"github.com/buidangphuc/team-audit/internal/handler"
	"github.com/buidangphuc/team-audit/internal/interceptor"
)

type Server struct {
	grpcServer *grpc.Server
	port       int
}

// Build assembles the gRPC server: the principal interceptor (never rejects; each
// handler authorizes as its first statement), the audit service, reflection and health.
func Build(auditHandler *handler.AuditHandler) *grpc.Server {
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptor.Unary()))
	auditv1.RegisterAuditServiceServer(srv, auditHandler)
	reflection.Register(srv)
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(srv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	return srv
}

func New(port int, auditHandler *handler.AuditHandler) *Server {
	return &Server{grpcServer: Build(auditHandler), port: port}
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
