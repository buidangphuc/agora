// Package bootstrap opens the resources the analytics worker needs — the
// selected WarehouseWriter adapter and a health server — and owns the boot-time
// adapter selection so the warehouse package stays free of any adapter import
// (adapters depend on warehouse, never the reverse). Mirrors team-search's
// lifecycle, with the warehouse in place of OpenSearch.
package bootstrap

import (
	"context"
	"fmt"

	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/bigquery"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

// Resources is the central bag of opened handles.
type Resources struct {
	Writer warehouse.WarehouseWriter
	Health *health.Server
}

// OpenResources selects + opens the warehouse adapter and installs a SERVING
// health server.
func OpenResources(ctx context.Context, s *config.Settings) (*Resources, error) {
	w, err := OpenWarehouse(ctx, s)
	if err != nil {
		return nil, err
	}
	h := health.NewServer()
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	return &Resources{Writer: w, Health: h}, nil
}

// OpenWarehouse is the WAREHOUSE_DRIVER switch: the ONE place the concrete
// adapter is chosen. Adding a backend (e.g. Snowflake) is a new case here plus a
// new adapter package — the consume/map/batch path never changes (spec: driver
// switch requires no change to event handling).
func OpenWarehouse(ctx context.Context, s *config.Settings) (warehouse.WarehouseWriter, error) {
	switch s.Warehouse.Driver {
	case config.DriverDuckDB:
		return duckdb.Open(ctx, s.Warehouse.DuckDBPath)
	case config.DriverBigQuery:
		return bigquery.Open(ctx, bigquery.Config{
			Project: s.Warehouse.BigQueryProject,
			Dataset: s.Warehouse.BigQueryDataset,
			Table:   s.Warehouse.BigQueryTable,
		})
	default:
		return nil, fmt.Errorf("unknown WAREHOUSE_DRIVER %q", s.Warehouse.Driver)
	}
}

// CloseResources releases resources (flushes/closes the warehouse handle).
func CloseResources(_ context.Context, r *Resources) error {
	if r == nil || r.Writer == nil {
		return nil
	}
	return r.Writer.Close()
}
