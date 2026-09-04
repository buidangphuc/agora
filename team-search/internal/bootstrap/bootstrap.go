// Package bootstrap opens the resources team-search needs — its OpenSearch
// read-model index and a health server — shared by both entrypoints (the gRPC
// query API and the Kafka indexer). Mirrors team-domain's lifecycle, minus
// Postgres (OpenSearch is this service's store).
package bootstrap

import (
	"context"
	"fmt"

	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
)

// Resources is the central bag of opened handles.
type Resources struct {
	Index  index.Index
	Health *health.Server
}

// OpenResources builds the OpenSearch index client, ensures the index exists,
// and installs a SERVING health server.
func OpenResources(ctx context.Context, s *config.Settings) (*Resources, error) {
	idx, err := index.New(s.OpenSearch.URL, s.OpenSearch.Index)
	if err != nil {
		return nil, err
	}
	if err := idx.EnsureIndex(ctx); err != nil {
		return nil, fmt.Errorf("ensure index: %w", err)
	}
	h := health.NewServer()
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	return &Resources{Index: idx, Health: h}, nil
}

// CloseResources releases resources. The OpenSearch client is stateless HTTP, so
// there is nothing to close today; kept for lifecycle symmetry.
func CloseResources(_ context.Context, _ *Resources) error { return nil }
