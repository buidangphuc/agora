package app

import (
	"context"
	"database/sql"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/ga4relay"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

type testHooks struct {
	OpenWarehouse  func(ctx context.Context, s config.Settings) (warehouse.WarehouseWriter, error)
	PingDuckDB     func(ctx context.Context, db *sql.DB) error
	HealthInterval time.Duration
	DLQProducer    func(role string, p consume.DLQProducer)
	WrapLedger     func(l ga4relay.Ledger) ga4relay.Ledger
	Relay          func(r *ga4relay.Relay)
}

var hooks testHooks

func healthInterval() time.Duration {
	if hooks.HealthInterval > 0 {
		return hooks.HealthInterval
	}
	return healthx.DefaultInterval
}

func pingDuckDB(ctx context.Context, db *sql.DB) error {
	if hooks.PingDuckDB != nil {
		return hooks.PingDuckDB(ctx, db)
	}
	return db.PingContext(ctx)
}
