// Package rpc is the transport layer of team-analytics: it validates requests,
// reads the caller, maps proto to the analytics read model and errors to gRPC
// status. The aggregations live in internal/analytics.
package rpc

import (
	"google.golang.org/grpc"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
)

// Register adds the read-only AnalyticsQueryService to s.
func Register(s grpc.ServiceRegistrar, h analyticsv1.AnalyticsQueryServiceServer) {
	analyticsv1.RegisterAnalyticsQueryServiceServer(s, h)
}
