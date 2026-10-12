package app_test

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/app"
	"github.com/buidangphuc/team-promotion/internal/config"
)

// closedDatabaseURL points at a port nothing listens on.
const closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

// freePort asks the kernel for an unused port, then releases it for the
// service to bind. GRPC_PORT has no default and 0 is not usable.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func settings(port int, env string) config.Settings {
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: env, LogLevel: "error", LogJSON: true},
		Server:   sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGraceSeconds: 5},
		Database: sdkconfig.Postgres{URL: closedDatabaseURL},
		Kafka:    sdkconfig.Kafka{Brokers: "127.0.0.1:1"},
		Events:   config.Events{Topic: "promotion.events"},
		Upstream: config.Upstream{CallTimeoutSeconds: 2},
		Ads:      config.Ads{MaxBid: 1000, MaxBudget: 5000},
	}
}

func TestBootFailsWhenDatabaseUnreachableDeployed(t *testing.T) {
	port := freePort(t)
	s := settings(port, "staging")

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

// Locally the in-memory repositories keep the service usable without a database.
func TestLocalBootServesOnInMemoryRepositories(t *testing.T) {
	s := settings(freePort(t), "local")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, s) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("app did not stop")
		}
	})

	conn, err := grpc.NewClient("127.0.0.1:"+strconv.Itoa(s.Server.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ccancel()
	_, err = promotionv1.NewVoucherServiceClient(conn).GetVoucher(cctx, &promotionv1.GetVoucherRequest{Code: "NOPE"}, grpc.WaitForReady(true))
	if status.Code(err) != codes.NotFound {
		t.Fatalf("GetVoucher on a fresh in-memory store = %v, want NotFound", err)
	}
}
