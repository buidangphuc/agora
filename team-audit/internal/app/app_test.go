package app_test

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	"github.com/buidangphuc/team-audit/internal/app"
	"github.com/buidangphuc/team-audit/internal/config"
	"github.com/buidangphuc/team-audit/internal/store"
)

const closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

type running struct {
	conn   *grpc.ClientConn
	cancel context.CancelFunc
	done   chan struct{}
	err    error
	port   int
}

func settings(port int, url string) config.Settings {
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGrace: 5 * time.Second},
		Postgres: sdkconfig.Postgres{URL: url},
	}
}

func startApp(t *testing.T, s config.Settings) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan struct{}), port: s.Port}
	go func() { r.err = app.Run(ctx, s); close(r.done) }()
	conn, err := grpc.NewClient("127.0.0.1:"+strconv.Itoa(s.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	r.conn = conn
	t.Cleanup(func() {
		_ = conn.Close()
		cancel()
		select {
		case <-r.done:
		case <-time.After(15 * time.Second):
			t.Error("app did not stop")
		}
	})
	return r
}

func waitServing(t *testing.T, r *running) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{}, grpc.WaitForReady(true))
	if err != nil || resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("health = %v, %v, want SERVING", resp.GetStatus(), err)
	}
}

func serviceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func conn(t *testing.T, url string) *pgx.Conn {
	t.Helper()
	c, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

func exists(t *testing.T, url, qualified string) bool {
	t.Helper()
	var reg *string
	if err := conn(t, url).QueryRow(context.Background(), "SELECT to_regclass($1)::text", qualified).Scan(&reg); err != nil {
		t.Fatal(err)
	}
	return reg != nil
}

func count(t *testing.T, url, table string) int {
	t.Helper()
	var n int
	if err := conn(t, url).QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func schemaVersion(t *testing.T, url string) (int, bool) {
	t.Helper()
	var v int
	var dirty bool
	if err := conn(t, url).QueryRow(context.Background(), "SELECT version, dirty FROM schema_migrations").Scan(&v, &dirty); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	return v, dirty
}

func TestRunFailsWhenDatabaseUnreachable(t *testing.T) {
	// scenario: An unreachable database fails boot locally
	port := testkit.FreePort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, settings(port, closedDatabaseURL)) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil with an unreachable database in ENV=local")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not fail within 10s; it is serving or hanging without its database")
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("port %d was bound by the failed boot: %v", port, err)
	}
	_ = l.Close()
}

func TestServerBinaryExcludesInMemoryStore(t *testing.T) {
	// scenario: The server binary does not link the in-memory store
	cmd := exec.Command("go", "list", "-deps", "./cmd/server")
	cmd.Dir = serviceRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps ./cmd/server: %v\n%s", err, out)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.HasSuffix(pkg, "storetest") {
			t.Errorf("cmd/server links %s", pkg)
		}
	}
}

func TestRunWriteIsCommittedToPostgres(t *testing.T) {
	// scenario: A written event is durable
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := app.Migrate(context.Background(), settings(testkit.FreePort(t), url)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := startApp(t, settings(testkit.FreePort(t), url))
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-principal-id", "service-x", "x-principal-type", "service", "x-principal-scopes", "audit.write")
	_, err := auditv1.NewAuditServiceClient(r.conn).WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{
		ActorId: "actor-durable", Action: "listing.publish", TargetType: "listing", TargetId: "l-1",
	}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("WriteAuditEvent: %v", err)
	}

	var n int
	if err := conn(t, url).QueryRow(context.Background(),
		"SELECT count(*) FROM audit_events WHERE action = 'listing.publish' AND actor_id = 'actor-durable'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("audit_events rows = %d, want 1", n)
	}
	if total := count(t, url, "audit_events"); total != 1 {
		t.Fatalf("audit_events total rows = %d, want exactly 1", total)
	}
}

func TestMigrateUpMigratesEmptyDatabase(t *testing.T) {
	// scenario: An empty database is migrated by migrate up
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := app.Migrate(context.Background(), settings(testkit.FreePort(t), url)); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !exists(t, url, "audit_events") {
		t.Error("audit_events missing after migrate up")
	}
	if !exists(t, url, "idx_audit_events_action_created") {
		t.Error("idx_audit_events_action_created missing after migrate up")
	}
	if v, dirty := schemaVersion(t, url); v != 2 || dirty {
		t.Errorf("schema_migrations = (%d, dirty=%v), want (2, false)", v, dirty)
	}
}

func TestMigrateUpLeavesMigratedDatabaseUnchanged(t *testing.T) {
	// scenario: A database already at version 2 is unchanged
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(context.Background(), url, store.Migrations()); err != nil {
		t.Fatalf("seed migrate: %v", err)
	}
	c := conn(t, url)
	for _, id := range []string{"a1", "a2", "a3"} {
		if _, err := c.Exec(context.Background(),
			"INSERT INTO audit_events (id, actor_id, action) VALUES ($1::text, 'actor', 'act.'||$1::text)", id); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.Migrate(context.Background(), settings(testkit.FreePort(t), url)); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if n := count(t, url, "audit_events"); n != 3 {
		t.Errorf("audit_events rows = %d, want 3", n)
	}
	if v, dirty := schemaVersion(t, url); v != 2 || dirty {
		t.Errorf("schema_migrations = (%d, dirty=%v), want (2, false)", v, dirty)
	}
}

func TestMigrateUpFailsWhenMigrationStateIsDirty(t *testing.T) {
	// scenario: A dirty migration state fails migrate up
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(context.Background(), url, store.Migrations()); err != nil {
		t.Fatalf("seed migrate: %v", err)
	}
	if _, err := conn(t, url).Exec(context.Background(), "UPDATE schema_migrations SET dirty = true"); err != nil {
		t.Fatal(err)
	}
	if err := app.Migrate(context.Background(), settings(testkit.FreePort(t), url)); err == nil {
		t.Fatal("Migrate returned nil on a dirty schema_migrations row")
	}
	if v, dirty := schemaVersion(t, url); v != 2 || !dirty {
		t.Errorf("schema_migrations = (%d, dirty=%v), want (2, true) unchanged", v, dirty)
	}
}

func TestRunDoesNotMigrate(t *testing.T) {
	// scenario: Run does not migrate
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	r := startApp(t, settings(testkit.FreePort(t), url))
	waitServing(t, r)
	for _, table := range []string{"audit_events", "schema_migrations"} {
		if exists(t, url, table) {
			t.Errorf("table %s exists after boot; Run must not migrate", table)
		}
	}
}

func TestMigrateUpSubcommandExitsNonZeroOnUnreachableDatabase(t *testing.T) {
	// scenario: The migrate up subcommand is dispatched by main
	bin := filepath.Join(t.TempDir(), "server")
	build := exec.Command("go", "build", "-o", bin, "./cmd/server")
	build.Dir = serviceRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/server: %v\n%s", err, out)
	}

	port := testkit.FreePort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "migrate", "up")
	cmd.Env = []string{"ENV=local", "GRPC_PORT=" + strconv.Itoa(port), "DATABASE_URL=" + closedDatabaseURL}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("migrate up did not exit within 10s\n%s", out)
	}
	if err == nil {
		t.Fatalf("migrate up exited 0 against an unreachable database\n%s", out)
	}
	if exitErr := (*exec.ExitError)(nil); !errors.As(err, &exitErr) {
		t.Fatalf("migrate up failed to run: %v", err)
	}
	l, lerr := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if lerr != nil {
		t.Fatalf("port %d is bound after migrate up: %v", port, lerr)
	}
	_ = l.Close()
}

func TestShutdownDrainsHealthFirst(t *testing.T) {
	// scenario: Shutdown drains health first
	url := testkit.Postgres(t)
	s := settings(testkit.FreePort(t), url)
	s.DrainDelay = 2 * time.Second
	r := startApp(t, s)
	waitServing(t, r)

	watchCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	hc := grpc_health_v1.NewHealthClient(r.conn)
	streams := map[string]grpc_health_v1.Health_WatchClient{}
	for _, svc := range []string{"", "liveness"} {
		st, err := hc.Watch(watchCtx, &grpc_health_v1.HealthCheckRequest{Service: svc}, grpc.WaitForReady(true))
		if err != nil {
			t.Fatalf("watch %q: %v", svc, err)
		}
		first, err := st.Recv()
		if err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Fatalf("watch %q first = %v, %v, want SERVING", svc, first.GetStatus(), err)
		}
		streams[svc] = st
	}

	r.cancel()

	for svc, st := range streams {
		for {
			resp, err := st.Recv()
			if err != nil {
				t.Fatalf("watch %q closed before reporting NOT_SERVING: %v", svc, err)
			}
			if resp.GetStatus() == grpc_health_v1.HealthCheckResponse_NOT_SERVING {
				break
			}
		}
	}

	select {
	case <-r.done:
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return")
	}
	if r.err != nil {
		t.Fatalf("Run returned %v, want nil", r.err)
	}
}

func TestUnknownHealthServiceNotFound(t *testing.T) {
	// scenario: An unknown health service name is not found
	url := testkit.Postgres(t)
	r := startApp(t, settings(testkit.FreePort(t), url))
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "nope"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("Check(nope) code = %v, want NotFound (err=%v)", status.Code(err), err)
	}
}
