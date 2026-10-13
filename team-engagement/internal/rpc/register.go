package rpc

import (
	"google.golang.org/grpc"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
)

// Register adds the EngagementService handler to srv. The SDK's grpcx.NewServer
// already installs the interceptors (request id, panic recovery, logging, auth,
// otelgrpc), the health service and reflection.
func Register(srv grpc.ServiceRegistrar, h *EngagementHandler) {
	engagementv1.RegisterEngagementServiceServer(srv, h)
}
