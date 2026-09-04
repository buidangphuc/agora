// Package grpcserver builds the gRPC server for the analytics worker. Its core
// input is the Kafka topic, so it always exposes the standard gRPC Health
// service for Kubernetes liveness/readiness probes (grpc.health.v1.Health). It
// also optionally serves the read-only AnalyticsQueryService (seller-dashboard
// funnel/revenue aggregations over the warehouse) when a servicer is supplied.
package grpcserver

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/config"
)

// Build constructs a ready-but-unstarted gRPC server exposing the standard gRPC
// Health service (+ optional reflection) and, when queryServer is non-nil, the
// read-only AnalyticsQueryService. No unary/stream interceptors are wired: the
// query RPCs are read-only aggregations scoped by an explicit seller_id
// argument (no principal-bearing mutating surface to protect here).
func Build(s *config.Settings, healthSrv *health.Server, queryServer analyticsv1.AnalyticsQueryServiceServer) *grpc.Server {
	srv := grpc.NewServer()

	if healthSrv == nil {
		healthSrv = health.NewServer()
		healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	}
	healthpb.RegisterHealthServer(srv, healthSrv)

	if queryServer != nil {
		analyticsv1.RegisterAnalyticsQueryServiceServer(srv, queryServer)
	}

	if s.Server.ReflectionEnabled {
		reflection.Register(srv)
	}
	return srv
}
