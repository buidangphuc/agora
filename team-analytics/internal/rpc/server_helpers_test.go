package rpc_test

import (
	"log/slog"

	"google.golang.org/grpc"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/rpc"
)

// newTestServer builds the production server shape (SDK interceptors, health)
// and registers the query service when one is supplied.
func newTestServer(q analyticsv1.AnalyticsQueryServiceServer) *grpc.Server {
	logger := slog.New(slog.DiscardHandler)
	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		panic(err)
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{
		Config: sdkconfig.GRPC{},
		Logger: logger,
		Health: health,
	})
	if q != nil {
		rpc.Register(srv, q)
	}
	return srv
}
