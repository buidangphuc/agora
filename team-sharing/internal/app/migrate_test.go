package app_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-sharing/internal/app"
)

var migrateTables = []string{"share_links"}

func migrateEnv(t *testing.T, url string) {
	t.Helper()
	for _, k := range []string{"GRPC_PORT", "GRPC_HOST", "LOG_LEVEL", "LOG_JSON", "OTEL_ENABLED", "GRPC_REFLECTION_ENABLED", "SHUTDOWN_GRACE"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	t.Setenv("ENV", "local")
	t.Setenv("DATABASE_URL", url)
}

func migDial(t *testing.T, url string) *pgx.Conn {
	t.Helper()
	c, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

func migTableExists(t *testing.T, url, table string) bool {
	t.Helper()
	var reg *string
	if err := migDial(t, url).QueryRow(context.Background(), "SELECT to_regclass($1)::text", table).Scan(&reg); err != nil {
		t.Fatalf("to_regclass %s: %v", table, err)
	}
	return reg != nil
}

func migVersion(t *testing.T, url string) (int, bool) {
	t.Helper()
	var v int
	var dirty bool
	if err := migDial(t, url).QueryRow(context.Background(), "SELECT version, dirty FROM schema_migrations").Scan(&v, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return v, dirty
}

func TestMigrateUpAppliesEmbeddedMigrations(t *testing.T) {
	// scenario: team-sharing migrate up applies the embedded migrations
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	migrateEnv(t, url)

	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if v, dirty := migVersion(t, url); v != 2 || dirty {
		t.Fatalf("schema_migrations = (%d, dirty=%v), want (2, false)", v, dirty)
	}
	for _, table := range migrateTables {
		if !migTableExists(t, url, table) {
			t.Errorf("table %s missing after migrate up", table)
		}
	}
}

func TestMigrateUpIsIdempotent(t *testing.T) {
	// scenario: team-sharing migrate up applies the embedded migrations
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	migrateEnv(t, url)

	for i := 1; i <= 2; i++ {
		if err := app.Migrate(context.Background()); err != nil {
			t.Fatalf("migrate up run %d: %v", i, err)
		}
	}
	if v, dirty := migVersion(t, url); v != 2 || dirty {
		t.Fatalf("schema_migrations = (%d, dirty=%v), want (2, false)", v, dirty)
	}
}

func TestMigrateUpRefusesEmptyDatabaseURL(t *testing.T) {
	// scenario: migrate up fails on an unreachable database or an empty DATABASE_URL
	migrateEnv(t, "")
	if err := app.Migrate(context.Background()); err == nil {
		t.Fatal("Migrate returned nil with an empty DATABASE_URL")
	}
}

func TestMigrateUpFailsOnUnreachableDatabase(t *testing.T) {
	// scenario: migrate up fails on an unreachable database or an empty DATABASE_URL
	migrateEnv(t, closedDatabaseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := app.Migrate(ctx)
	if err == nil {
		t.Fatal("Migrate returned nil against an unreachable database")
	}
	if ctx.Err() != nil {
		t.Fatalf("Migrate did not fail within 10s: %v", err)
	}
	if !strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("error %q does not name the database host", err)
	}
}

func migBuild(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	bin := filepath.Join(t.TempDir(), "server")
	build := exec.Command("go", "build", "-o", bin, "./cmd/server")
	build.Dir = filepath.Join(filepath.Dir(file), "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/server: %v\n%s", err, out)
	}
	return bin
}

func TestMigrateUpSubcommandAppliesMigrationsAndExits(t *testing.T) {
	// scenario: team-sharing migrate up applies the embedded migrations
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	bin := migBuild(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "migrate", "up")
	cmd.Env = append(os.Environ(), "ENV=local", "DATABASE_URL="+url, "LOG_LEVEL=error", "OTEL_ENABLED=false")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("server migrate up: %v\n%s", err, out)
	}
	for _, table := range migrateTables {
		if !migTableExists(t, url, table) {
			t.Errorf("table %s missing after server migrate up", table)
		}
	}
}

func TestMigrateUpSubcommandFailsOnEmptyDatabaseURL(t *testing.T) {
	// scenario: migrate up fails on an unreachable database or an empty DATABASE_URL
	bin := migBuild(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "migrate", "up")
	cmd.Env = append(os.Environ(), "ENV=local", "DATABASE_URL=", "LOG_LEVEL=error", "OTEL_ENABLED=false")
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatal("server migrate up with empty DATABASE_URL did not exit within 20s")
	}
	if err == nil {
		t.Fatal("server migrate up with empty DATABASE_URL exited 0")
	}
}

func TestBootBinaryNeverMigrates(t *testing.T) {
	// scenario: Server boot never migrates
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	bin := migBuild(t)

	port := testkit.FreePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = append(os.Environ(),
		"ENV=local",
		"DATABASE_URL="+url,
		"GRPC_HOST=127.0.0.1",
		"GRPC_PORT="+strconv.Itoa(port),
		"LOG_LEVEL=error",
		"OTEL_ENABLED=false",
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

	for _, table := range append([]string{"schema_migrations"}, migrateTables...) {
		if migTableExists(t, url, table) {
			t.Errorf("table %s exists after boot; the server must not migrate", table)
		}
	}
}
