package app

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

const healthCheckProcedure = "/grpc.health.v1.Health/Check"

func healthHandler(srv *health.Server) (string, http.Handler) {
	return healthCheckProcedure, connect.NewUnaryHandler(
		healthCheckProcedure,
		func(ctx context.Context, req *connect.Request[grpc_health_v1.HealthCheckRequest]) (*connect.Response[grpc_health_v1.HealthCheckResponse], error) {
			resp, err := srv.Check(ctx, req.Msg)
			if err != nil {
				return nil, connect.NewError(connect.Code(status.Code(err)), err)
			}
			return connect.NewResponse(resp), nil
		},
	)
}
