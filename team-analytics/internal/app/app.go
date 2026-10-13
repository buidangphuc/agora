// Package app is the composition root of team-analytics: it builds every
// dependency in order, wires them together and runs them. Each binary has one
// entry point: RunConsumer, RunGA4Relay and RunGA4Import. Read top to bottom to
// see what runs.
package app

import (
	"context"
	"fmt"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/store"
	"github.com/buidangphuc/team-analytics/internal/store/bigquery"
	"github.com/buidangphuc/team-analytics/internal/store/duckdb"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
)

const service = "team-analytics"

// openWarehouse is the WAREHOUSE_DRIVER switch: the ONE place the concrete
// adapter is chosen. Adding a backend (e.g. Snowflake) is a new case here plus a
// new adapter package: the consume/map/batch path never changes. An adapter that
// cannot be opened fails boot in every environment.
func openWarehouse(ctx context.Context, s config.Settings) (store.WarehouseWriter, error) {
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

// untilDone adapts a background job that returns when ctx ends, or earlier when
// it gives up (for example a disabled scheduler), to a lifecycle component. A
// component that returns ends the whole process, so after f returns the
// component waits for shutdown instead: a job that gave up was only ever logged.
func untilDone(f func(ctx context.Context)) lifecycle.Component {
	return lifecycle.Func(func(ctx context.Context) error {
		f(ctx)
		<-ctx.Done()
		return nil
	})
}
