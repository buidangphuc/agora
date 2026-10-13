package app_test

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/buidangphuc/team-analytics/internal/app"
	"github.com/buidangphuc/team-analytics/internal/config"
)

// freePort returns a port nothing listens on right now.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// Scenario: team-analytics fails boot on an unreachable database when deployed.
// team-analytics has no Postgres: its database is the warehouse behind the
// WarehouseWriter seam, here a DuckDB file under a directory that does not exist.
func TestBootFailsWhenDatabaseUnreachableDeployed(t *testing.T) {
	port := freePort(t)
	t.Setenv("ENV", "staging")
	t.Setenv("KAFKA_BROKERS", "localhost:9092") // required when deployed
	t.Setenv("GRPC_PORT", strconv.Itoa(port))
	t.Setenv("METRICS_ADDR", "")
	t.Setenv("WAREHOUSE_DRIVER", "duckdb")
	t.Setenv("DUCKDB_PATH", filepath.Join(t.TempDir(), "missing-dir", "analytics.duckdb"))
	s, err := config.Load()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = app.RunConsumer(ctx, s)
	require.Error(t, err, "boot must fail instead of serving without a warehouse")

	// Boot failed before any port opened: the gRPC port is still free.
	l, lerr := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	require.NoError(t, lerr, "gRPC port was opened before boot failed")
	assert.NoError(t, l.Close())
}
