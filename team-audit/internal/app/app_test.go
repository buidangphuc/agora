package app_test

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	"github.com/buidangphuc/team-audit/internal/app"
	"github.com/buidangphuc/team-audit/internal/config"
)

// closedDatabaseURL points at a port nothing listens on.
const closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

type running struct {
	conn   *grpc.ClientConn
	cancel context.CancelFunc
	done   chan struct{} // closed when Run returned; err is then set
	err    error
}

func settings(port int, env string, db sdkconfig.Postgres) config.Settings {
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: env, LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGrace: 5 * time.Second},
		Postgres: db,
	}
}

func startApp(t *testing.T, s config.Settings) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan struct{})}
	go func() { r.err = app.Run(ctx, s); close(r.done) }()

	var err error
	r.conn, err = grpc.NewClient("127.0.0.1:"+strconv.Itoa(s.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.conn.Close()
		cancel()
		select {
		case <-r.done:
		case <-time.After(15 * time.Second):
			t.Error("app did not stop")
		}
	})
	return r
}

func TestBootFailsWhenDatabaseUnreachableOutsideLocal(t *testing.T) {
	port := testkit.FreePort(t)
	s := settings(port, "staging", sdkconfig.Postgres{URL: closedDatabaseURL})

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

func TestLocalFallsBackToMemoryStore(t *testing.T) {
	s := settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: closedDatabaseURL})
	r := startApp(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := auditv1.NewAuditServiceClient(r.conn)

	svc := metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-principal-id", "svc-order", "x-principal-type", "service", "x-principal-scopes", "audit.write"))
	if _, err := client.WriteAuditEvent(svc, &auditv1.WriteAuditEventRequest{ActorId: "u1", Action: "login", TargetType: "user"}, grpc.WaitForReady(true)); err != nil {
		t.Fatalf("WriteAuditEvent: %v", err)
	}
	adm := metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-principal-id", "admin-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	res, err := client.QueryAuditLog(adm, &auditv1.QueryAuditLogRequest{})
	if err != nil {
		t.Fatalf("QueryAuditLog: %v", err)
	}
	if len(res.GetEvents()) != 1 || res.GetEvents()[0].GetAction() != "login" {
		t.Fatalf("events = %+v, want the one written event", res.GetEvents())
	}

	// Health is SERVING: there is no database to check.
	h, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil || h.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("health = %v, %v, want SERVING", h.GetStatus(), err)
	}
}

func TestHealthReportsDatabaseDown(t *testing.T) {
	// A dedicated container, not SharedPostgres: the test disables connections to
	// the whole database "test".
	url := testkit.Postgres(t)
	s := settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url})
	r := startApp(t, s)

	watchCtx, stopWatch := context.WithTimeout(context.Background(), 60*time.Second)
	defer stopWatch()
	stream, err := grpc_health_v1.NewHealthClient(r.conn).Watch(watchCtx, &grpc_health_v1.HealthCheckRequest{}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	first, err := stream.Recv()
	if err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("first status = %v, %v, want SERVING", first.GetStatus(), err)
	}

	// Take the database away: refuse new connections and kill the open ones.
	adminURL := strings.Replace(url, "/test?", "/postgres?", 1)
	admin, err := pgx.Connect(context.Background(), adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	for _, q := range []string{
		"ALTER DATABASE test ALLOW_CONNECTIONS false",
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'test'",
	} {
		if _, err := admin.Exec(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	// Within a probe interval (5s) plus slack the status must flip.
	got := make(chan grpc_health_v1.HealthCheckResponse_ServingStatus, 1)
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				return
			}
			if resp.GetStatus() == grpc_health_v1.HealthCheckResponse_NOT_SERVING {
				got <- resp.GetStatus()
				return
			}
		}
	}()
	select {
	case <-got:
	case <-time.After(20 * time.Second):
		t.Fatal("health stayed SERVING after the database went away")
	}
	stopWatch()
}

func TestLocalMemoryStoreHealthIsServing(t *testing.T) {
	s := settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: closedDatabaseURL})
	r := startApp(t, s)

	// Outlast one probe interval (5s): a registered postgres check would have
	// flipped the status by now.
	time.Sleep(7 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("status = %v, want SERVING", resp.GetStatus())
	}
}

func TestBootDoesNotApplyMigrations(t *testing.T) {
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	r := startApp(t, settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// A served health check proves boot completed.
	if _, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{}, grpc.WaitForReady(true)); err != nil {
		t.Fatalf("health check: %v", err)
	}

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	for _, table := range []string{"audit_events", "schema_migrations"} {
		var reg *string
		if err := conn.QueryRow(ctx, "SELECT to_regclass($1)::text", table).Scan(&reg); err != nil {
			t.Fatal(err)
		}
		if reg != nil {
			t.Errorf("table %s exists after boot; migrations must not run at boot", table)
		}
	}
}
