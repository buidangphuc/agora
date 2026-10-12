package app_test

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/buidangphuc/team-notification/internal/app"
	"github.com/buidangphuc/team-notification/internal/config"
)

// closedDatabaseURL points at a port nothing listens on.
const closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

// freePort asks the kernel for an unused port, then releases it for the service
// to bind. GRPC_PORT has no default and 0 is not usable.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func load(t *testing.T, env string, port int, databaseURL string) config.Settings {
	t.Helper()
	t.Setenv("ENV", env)
	t.Setenv("GRPC_HOST", "127.0.0.1")
	t.Setenv("GRPC_PORT", strconv.Itoa(port))
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("LOG_LEVEL", "error")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return s
}

func TestBootFailsWhenDatabaseUnreachableDeployed(t *testing.T) {
	port := freePort(t)
	s := load(t, "staging", port, closedDatabaseURL)

	done := make(chan error, 1)
	go func() { done <- app.Run(context.Background(), s) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil with an unreachable database in ENV=staging")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not fail; it is serving without its database")
	}

	// No listener was opened: the port is still free.
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("port %d was bound by the failed boot: %v", port, err)
	}
	_ = l.Close()
}
