package app_test

import (
	"context"
	"io/fs"
	"net"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-order/internal/app"
	"github.com/buidangphuc/team-order/internal/config"
	"github.com/buidangphuc/team-order/migrations"
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

func settings(t *testing.T, env, databaseURL string) config.Settings {
	t.Helper()
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: env, LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: freePort(t), ShutdownGraceSeconds: 10},
		Postgres: sdkconfig.Postgres{URL: databaseURL, MaxConns: 4},
		Kafka:    sdkconfig.Kafka{Brokers: "127.0.0.1:1"},
		Outbox:   sdkconfig.Outbox{PollInterval: 20 * time.Millisecond, BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 10},
		Events: config.Events{
			ConsumerGroup: "team-order.payment",
			PaymentTopic:  "payment.events",
			PaymentDLQ:    "payment.events.dlq",
			OrderTopic:    "order.events",
		},
		Reservation: config.Reservation{TTL: "15m", SweepInterval: "1m"},
	}
}

const unreachableDB = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

func TestBootRequiresDatabaseURL(t *testing.T) {
	s := settings(t, "local", "")
	err := app.Run(context.Background(), s)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Run = %v, want an error naming DATABASE_URL", err)
	}
	l, lerr := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Port)))
	if lerr != nil {
		t.Fatalf("port %d was opened before the boot check failed: %v", s.Port, lerr)
	}
	_ = l.Close()
}

// Staging and production never fall back to the in-memory repositories: an
// unreachable database refuses the boot before any port opens.
func TestBootFailsWhenDatabaseUnreachableOutsideLocal(t *testing.T) {
	for _, env := range []string{"staging", "stage", "prod", "production"} {
		t.Run(env, func(t *testing.T) {
			s := settings(t, env, unreachableDB)
			if err := app.Run(context.Background(), s); err == nil {
				t.Fatalf("Run succeeded in ENV=%s with an unreachable database", env)
			}
			l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Port)))
			if err != nil {
				t.Fatalf("port opened before the boot check failed: %v", err)
			}
			_ = l.Close()
		})
	}
}

// watchHealth dials the service and returns the open Watch stream plus a closer.
func watchHealth(t *testing.T, port int) (grpc_health_v1.Health_WatchClient, func()) {
	t.Helper()
	conn, err := grpc.NewClient("127.0.0.1:"+strconv.Itoa(port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	stream, err := grpc_health_v1.NewHealthClient(conn).Watch(ctx, &grpc_health_v1.HealthCheckRequest{}, grpc.WaitForReady(true))
	if err != nil {
		cancel()
		t.Fatalf("watch: %v", err)
	}
	return stream, func() { cancel(); _ = conn.Close() }
}

func awaitStop(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("app did not stop")
	}
}

// In ENV=local an unreachable database keeps the in-memory repositories: the
// service boots, reports SERVING and stops cleanly.
func TestBootFallsBackToMemoryInLocal(t *testing.T) {
	s := settings(t, "local", unreachableDB)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, s) }()

	stream, closeWatch := watchHealth(t, s.Port)
	defer closeWatch()
	first, err := stream.Recv()
	if err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("first status = %v, %v, want SERVING", first.GetStatus(), err)
	}

	cancel()
	next, err := stream.Recv()
	if err != nil || next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("status after cancel = %v, %v, want NOT_SERVING (health drains first)", next.GetStatus(), err)
	}
	closeWatch()
	awaitStop(t, done)
}

// With a database the service runs its relayer, consumer and sweeper, reports
// SERVING, and stops cleanly with the pool closed after the workers.
func TestBootWithDatabaseServesAndStopsCleanly(t *testing.T) {
	ctx := context.Background()
	url := testkit.Postgres(t)

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		b, err := fs.ReadFile(migrations.Files, n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, string(b)); err != nil {
			t.Fatalf("apply %s: %v", n, err)
		}
	}

	const appName = "team-order-shutdown-test"
	s := settings(t, "local", url+"&application_name="+appName)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(runCtx, s) }()

	stream, closeWatch := watchHealth(t, s.Port)
	defer closeWatch()
	first, err := stream.Recv()
	if err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("first status = %v, %v, want SERVING", first.GetStatus(), err)
	}

	cancel()
	next, err := stream.Recv()
	if err != nil || next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("status after cancel = %v, %v, want NOT_SERVING", next.GetStatus(), err)
	}
	closeWatch()
	awaitStop(t, done)

	var open int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE application_name = $1", appName).Scan(&open); err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("%d connections of the service pool still open after Run returned", open)
	}
}
