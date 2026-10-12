package app_test

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-search/internal/app"
	"github.com/buidangphuc/team-search/internal/config"
)

// freePort asks the kernel for an unused port and releases it for the service.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func baseSettings(env string, port int) config.Settings {
	return config.Settings{
		Runtime:    sdkconfig.Runtime{Env: env, LogLevel: "error", LogJSON: true},
		GRPC:       sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGraceSeconds: 10},
		Postgres:   sdkconfig.Postgres{URL: "postgres://u:p@127.0.0.1:1/db?sslmode=disable"},
		OpenSearch: config.OpenSearch{URL: "http://127.0.0.1:1", Index: "listings"},
	}
}

// Spec: team-search fails boot on an unreachable database when deployed. The
// database opens before OpenSearch, so the error names it, and no port opens.
func TestBootFailsWhenDatabaseUnreachableDeployed(t *testing.T) {
	port := freePort(t)
	s := baseSettings("staging", port)

	err := app.RunServer(context.Background(), s)
	if err == nil || !strings.Contains(err.Error(), "open database") {
		t.Fatalf("RunServer = %v, want an error from opening the database", err)
	}
	l, lerr := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if lerr != nil {
		t.Fatalf("port %d was opened before the boot check failed: %v", port, lerr)
	}
	_ = l.Close()
}
