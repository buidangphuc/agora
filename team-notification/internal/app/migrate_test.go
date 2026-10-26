package app_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/app"
)

func schemaVersion(t *testing.T, url string) (int, bool) {
	t.Helper()
	var v int
	var dirty bool
	if err := dial(t, url).QueryRow(context.Background(), `SELECT version, dirty FROM schema_migrations`).Scan(&v, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return v, dirty
}

func serviceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func buildServer(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "server")
	build := exec.Command("go", "build", "-o", bin, "./cmd/server")
	build.Dir = serviceRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/server: %v\n%s", err, out)
	}
	return bin
}

func TestMigrateUpAppliesAndIsIdempotent(t *testing.T) {
	// scenario: Migrate then boot serves every RPC
	url := freshURL(t)
	migrateEnv(t, url)

	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	for _, table := range []string{"notifications", "alert_subscriptions", "processed_events"} {
		if !tableExists(t, url, table) {
			t.Errorf("table %s missing after migrate", table)
		}
	}
	v, dirty := schemaVersion(t, url)
	if dirty || v == 0 {
		t.Fatalf("schema version = %d dirty=%v after migrate", v, dirty)
	}
	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if v2, d2 := schemaVersion(t, url); v2 != v || d2 {
		t.Errorf("version after rerun = %d dirty=%v, want %d clean", v2, d2, v)
	}
}

func TestMigrateUpNeedsOnlyEnvAndDatabaseURL(t *testing.T) {
	// scenario: Migrate then boot serves every RPC
	url := freshURL(t)
	migrateEnv(t, url)
	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate with only ENV and DATABASE_URL: %v", err)
	}
}

func TestMigrateUpFailsOnDirtyMigration(t *testing.T) {
	// scenario: A failed migration fails the migrate subcommand
	url := migratedURL(t)
	migrateEnv(t, url)
	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate on a clean migrated schema: %v", err)
	}
	if _, err := dial(t, url).Exec(context.Background(), `UPDATE schema_migrations SET dirty = true`); err != nil {
		t.Fatal(err)
	}
	if err := app.Migrate(context.Background()); err == nil {
		t.Fatal("Migrate on a dirty schema returned nil, want an error")
	}
}

func TestMigrateUpFailsOnUnreachableDatabase(t *testing.T) {
	// scenario: Postgres open is bounded and fails fast
	migrateEnv(t, closedDatabaseURL)
	done := make(chan error, 1)
	go func() { done <- app.Migrate(context.Background()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Migrate on an unreachable database returned nil")
		}
		if !strings.Contains(err.Error(), "127.0.0.1:1") {
			t.Errorf("error %q does not name the database host", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Migrate did not fail fast on an unreachable database")
	}
}

func TestMigrateSubcommandRunsMigrationsAndFailsNonZero(t *testing.T) {
	// scenario: A failed migration fails the migrate subcommand
	bin := buildServer(t)

	url := freshURL(t)
	good := exec.Command(bin, "migrate", "up")
	good.Env = append(os.Environ(), "ENV=local", "DATABASE_URL="+url)
	if out, err := good.CombinedOutput(); err != nil {
		t.Fatalf("migrate up: %v\n%s", err, out)
	}
	if !tableExists(t, url, "notifications") {
		t.Error("notifications table missing after `server migrate up`")
	}

	if _, err := dial(t, url).Exec(context.Background(), `UPDATE schema_migrations SET dirty = true`); err != nil {
		t.Fatal(err)
	}
	bad := exec.Command(bin, "migrate", "up")
	bad.Env = append(os.Environ(), "ENV=local", "DATABASE_URL="+url)
	err := bad.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() == 0 {
		t.Fatalf("migrate up on a dirty schema: %v, want a non-zero exit", err)
	}
}

func TestRunDoesNotMigrate(t *testing.T) {
	// scenario: Boot never migrates
	url := freshURL(t)
	s := notifSettings(testkit.FreePort(t), url, deadBroker)
	r := start(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "liveness"}, grpc.WaitForReady(true)); err != nil {
		select {
		case <-r.done:
			t.Fatalf("app exited before serving: %v", r.err)
		default:
		}
		t.Fatalf("server not reachable: %v", err)
	}
	for _, table := range []string{"schema_migrations", "notifications", "alert_subscriptions", "processed_events"} {
		if tableExists(t, url, table) {
			t.Errorf("table %s exists after boot; the service must not migrate", table)
		}
	}
	callCtx, callCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer callCancel()
	_, err := notificationv1.NewNotificationServiceClient(r.conn).GetUnreadCount(asUser(callCtx, "buyer-1"), &notificationv1.GetUnreadCountRequest{})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("GetUnreadCount on unmigrated db = %v, want Internal / internal error", err)
	}
	if tableExists(t, url, "notifications") {
		t.Error("an RPC created the notifications table")
	}
}

func TestBootBinaryDoesNotMigrate(t *testing.T) {
	// scenario: Boot never migrates
	bin := buildServer(t)
	url := freshURL(t)
	port := testkit.FreePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = append(os.Environ(),
		"ENV=local", "DATABASE_URL="+url, "KAFKA_BROKERS="+deadBroker,
		"GRPC_HOST=127.0.0.1", "GRPC_PORT="+strconv.Itoa(port), "LOG_LEVEL=error", "OTEL_ENABLED=false",
		"UPSTREAM_DOMAIN_ADDR=127.0.0.1:1", "UPSTREAM_IDENTITY_ADDR=127.0.0.1:1",
	)
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start server: %v", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(15 * time.Second):
		}
	})
	conn, err := grpc.NewClient("127.0.0.1:"+strconv.Itoa(port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	waitCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	if _, err := grpc_health_v1.NewHealthClient(conn).Check(waitCtx, &grpc_health_v1.HealthCheckRequest{Service: "liveness"}, grpc.WaitForReady(true)); err != nil {
		select {
		case e := <-exited:
			t.Fatalf("server exited before serving: %v", e)
		default:
		}
		t.Fatalf("server not reachable: %v", err)
	}
	for _, table := range []string{"schema_migrations", "notifications"} {
		if tableExists(t, url, table) {
			t.Errorf("table %s exists after boot; the service must not migrate", table)
		}
	}
}
