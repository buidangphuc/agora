// Package bootstrap opens the resources team-search needs — its OpenSearch
// read-model index, retrieval engine, and a health server — shared by both entrypoints (the gRPC
// query API and the Kafka indexer). Mirrors team-domain's lifecycle, minus
// Postgres (OpenSearch is this service's store).
package bootstrap

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
)

// Resources is the central bag of opened handles.
type Resources struct {
	Index        index.Index
	EmbedClient  retrieval.EmbedClient
	RerankClient retrieval.RerankClient
	Engine       *retrieval.Engine
	Health       *health.Server
}

// OpenResources builds the OpenSearch index client, ensures the index exists,
// constructs retrieval clients and engine, and installs a SERVING health server.
func OpenResources(ctx context.Context, s *config.Settings) (*Resources, error) {
	idx, err := index.New(s.OpenSearch.URL, s.OpenSearch.Index)
	if err != nil {
		return nil, err
	}
	if err := idx.EnsureIndex(ctx); err != nil {
		return nil, fmt.Errorf("ensure index: %w", err)
	}

	embedClient := retrieval.NewHTTPEmbedClient(s.Retrieval.ModelServerURL, 2*time.Second)
	var rerankClient retrieval.RerankClient
	if s.Retrieval.EnableReranker {
		rerankClient = retrieval.NewHTTPRerankClient(s.Retrieval.ModelServerURL, 2*time.Second)
	}

	engine := retrieval.NewEngine(idx, embedClient, rerankClient, s.Retrieval)

	h := health.NewServer()
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	return &Resources{
		Index:        idx,
		EmbedClient:  embedClient,
		RerankClient: rerankClient,
		Engine:       engine,
		Health:       h,
	}, nil
}

// CloseResources releases resources. The OpenSearch client is stateless HTTP, so
// there is nothing to close today; kept for lifecycle symmetry.
func CloseResources(_ context.Context, _ *Resources) error { return nil }
