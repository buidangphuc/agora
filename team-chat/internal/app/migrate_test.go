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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	"github.com/buidangphuc/team-chat/internal/app"
)

const closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

var strayKeys = []string{
	"KAFKA_BROKERS", "KAFKA_CHAT_TOPIC", "GRPC_PORT", "GRPC_HOST", "LOG_LEVEL", "LOG_JSON",
	"OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK", "OTEL_ENABLED",
	"GRPC_REFLECTION_ENABLED", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY", "DB_MAX_CONNS",
}

func migrateEnv(t *testing.T, url string) {
	t.Helper()
	for _, k := range strayKeys {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	t.Setenv("ENV", "local")
	t.Setenv("DATABASE_URL", url)
}

func dial(t *testing.T, url string) *pgx.Conn {
	t.Helper()
	c, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

func tableExists(t *testing.T, url, table string) bool {
	t.Helper()
	var ok bool
	err := dial(t, url).QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1)`, table).Scan(&ok)
	if err != nil {
		t.Fatalf("query table %s: %v", table, err)
	}
	return ok
}

func schemaVersion(t *testing.T, url string) (int, bool) {
	t.Helper()
	var v int
	var dirty bool
	if err := dial(t, url).QueryRow(context.Background(), `SELECT version, dirty FROM schema_migrations`).Scan(&v, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return v, dirty
}

func TestMigrateUpAppliesAndIsIdempotent(t *testing.T) {
	// scenario: migrate up applies all migrations and is idempotent
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	migrateEnv(t, url)

	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("first migrate up: %v", err)
	}
	if v, dirty := schemaVersion(t, url); v != 12 || dirty {
		t.Fatalf("schema_migrations = (%d, dirty=%v), want (12, false)", v, dirty)
	}
	c := dial(t, url)
	for _, col := range []string{"seq", "topic", "traceparent", "status", "aggregate_id"} {
		var n int
		if err := c.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'chat_outbox_events' AND column_name = $1`, col).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("chat_outbox_events.%s missing after migrate up", col)
		}
	}
	for _, table := range []string{"chat_threads", "chat_messages"} {
		if !tableExists(t, url, table) {
			t.Errorf("table %s missing after migrate up", table)
		}
	}

	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate up: %v", err)
	}
	if v, dirty := schemaVersion(t, url); v != 12 || dirty {
		t.Fatalf("after second run schema_migrations = (%d, dirty=%v), want (12, false)", v, dirty)
	}
}

func TestMigrateUpNeedsOnlyEnvAndDatabaseURL(t *testing.T) {
	// scenario: migrate up applies all migrations and is idempotent
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	migrateEnv(t, url)
	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate up with only ENV and DATABASE_URL: %v", err)
	}
	if !tableExists(t, url, "chat_outbox_events") {
		t.Error("chat_outbox_events missing")
	}
}

func TestMigrateUpFailsOnUnreachableDatabase(t *testing.T) {
	// scenario: migrate up fails loudly on an unreachable database
	migrateEnv(t, closedDatabaseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	err := app.Migrate(ctx)
	if err == nil {
		t.Fatal("Migrate returned nil against an unreachable database")
	}
	if ctx.Err() != nil || time.Since(start) >= 10*time.Second {
		t.Fatalf("Migrate did not fail within 10s: %v", err)
	}
	if !strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("error %q does not name the database host", err)
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

func TestBootDoesNotMigrate(t *testing.T) {
	// scenario: Service boot never migrates
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	root := serviceRoot(t)
	bin := filepath.Join(t.TempDir(), "server")
	build := exec.Command("go", "build", "-o", bin, "./cmd/server")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/server: %v\n%s", err, out)
	}

	port := testkit.FreePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = append(os.Environ(),
		"ENV=local",
		"DATABASE_URL="+url,
		"KAFKA_BROKERS=127.0.0.1:1",
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

	for _, table := range []string{"schema_migrations", "chat_outbox_events", "chat_threads", "chat_messages"} {
		if tableExists(t, url, table) {
			t.Errorf("table %s exists after boot; the service must not migrate", table)
		}
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer callCancel()
	callCtx = metadata.AppendToOutgoingContext(callCtx, "x-principal-id", "buyer-1", "x-principal-type", "user")
	_, err = chatv1.NewChatServiceClient(conn).SendMessage(callCtx, &chatv1.SendMessageRequest{ThreadId: "t-1", Content: "hello"})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Fatalf("SendMessage = %v, want code Internal", err)
	}
	if st.Message() != "internal error" {
		t.Errorf("SendMessage message = %q, want %q", st.Message(), "internal error")
	}
	if tableExists(t, url, "chat_messages") {
		t.Error("SendMessage created chat_messages")
	}
}
