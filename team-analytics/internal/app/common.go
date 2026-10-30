package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/bigquery"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

const (
	consumerService = "team-analytics"
	relayService    = "team-analytics-ga4-relay"
	closeGrace      = 2 * time.Second
	purgeInterval   = time.Hour
)

func openWarehouse(ctx context.Context, s config.Settings) (warehouse.WarehouseWriter, error) {
	if hooks.OpenWarehouse != nil {
		return hooks.OpenWarehouse(ctx, s)
	}
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

func newGroupClient(s config.Settings, stack *lifecycle.Stack, group string, topics []string, opts ...kgo.Opt) (*kgo.Client, error) {
	client, err := kafkax.NewConsumerClient(s.Kafka.Kafka, group, topics, opts...)
	if err != nil {
		return nil, fmt.Errorf("kafka client for group %s: %w", group, err)
	}
	stack.Push(func(context.Context) error {
		client.AllowRebalance()
		client.Close()
		return nil
	})
	return client, nil
}

func untilDone(name string, logger *slog.Logger, fn func(context.Context) error) lifecycle.Component {
	return lifecycle.Func(func(ctx context.Context) error {
		if err := fn(ctx); err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "worker stopped; waiting for shutdown", "worker", name, "err", err)
		} else if ctx.Err() == nil {
			logger.WarnContext(ctx, "worker finished; waiting for shutdown", "worker", name)
		}
		<-ctx.Done()
		return nil
	})
}
