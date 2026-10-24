package app_test

import (
	"bytes"
	"context"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-domain/internal/app"
	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/migrations"
)

const (
	closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"
	deadBroker        = "127.0.0.1:1"
	healthWindow      = healthx.DefaultInterval + 1*time.Second
	listingTopic      = "listing.events"
	identityTopic     = "identity.events"
)

var strayKeys = []string{
	"KAFKA_BROKERS", "GRPC_PORT", "GRPC_HOST", "LOG_LEVEL", "LOG_JSON", "OTEL_ENABLED",
	"GRPC_REFLECTION_ENABLED", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY", "DB_MAX_CONNS",
	"UPSTREAM_AUDIT_ADDR", "UPSTREAM_AI_ADDR", "LOG_KEY_FP_SECRET",
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

func freshURL(t *testing.T) string {
	t.Helper()
	return testkit.Schema(t, testkit.SharedPostgres(t))
}

func migratedURL(t *testing.T) string {
	t.Helper()
	url := freshURL(t)
	if err := migrate.Up(t.Context(), url, fs.FS(migrations.FS)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return url
}

func migratedUpTo(t *testing.T, maxVersion int) string {
	t.Helper()
	url := freshURL(t)
	m := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	for _, e := range entries {
		n, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0])
		if err != nil || n > maxVersion || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := fs.ReadFile(migrations.FS, e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		m[e.Name()] = &fstest.MapFile{Data: b}
	}
	if err := migrate.Up(t.Context(), url, m); err != nil {
		t.Fatalf("migrate to %d: %v", maxVersion, err)
	}
	return url
}

func dial(t *testing.T, url string) *pgx.Conn {
	t.Helper()
	c, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.WithoutCancel(t.Context())) })
	return c
}

func openPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func tableExists(t *testing.T, url, table string) bool {
	t.Helper()
	var ok bool
	err := dial(t, url).QueryRow(t.Context(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1)`, table).Scan(&ok)
	if err != nil {
		t.Fatalf("query table %s: %v", table, err)
	}
	return ok
}

func domainSettings(port int, url, brokers string) config.Settings {
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGrace: 5 * time.Second},
		Postgres: sdkconfig.Postgres{URL: url, MaxConns: 5},
		Kafka:    sdkconfig.Kafka{Brokers: brokers},
		Outbox:   sdkconfig.Outbox{PollInterval: 200 * time.Millisecond, BatchSize: 100, ClaimLock: 60 * time.Second},
		LogKeyFP: sdkconfig.LogKeyFP{Secret: "test-log-key-fp-secret"},
		Storage: config.Storage{
			Endpoint: "localhost:9000", Bucket: "listing-images", AccessKey: "testkey", SecretKey: "testsecret",
			PublicBaseURL: "http://localhost:9000/listing-images",
		},
		Events: config.Events{
			ListingTopic:  listingTopic,
			IdentityTopic: identityTopic,
			IdentityGroup: "team-domain-identity",
		},
		Reservation: config.Reservation{TTL: "15m", SweepInterval: "200ms"},
		Upstream:    config.Upstream{AuditAddr: "127.0.0.1:1", AIAddr: "127.0.0.1:1", AITimeoutSeconds: 2},
	}
}

type running struct {
	conn   *grpc.ClientConn
	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

func start(t *testing.T, s config.Settings) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
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
		case <-time.After(30 * time.Second):
			t.Error("app did not stop")
		}
	})
	return r
}

func waitServing(t *testing.T, r *running) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	type result struct {
		resp *grpc_health_v1.HealthCheckResponse
		err  error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{}, grpc.WaitForReady(true))
		got <- result{resp, err}
	}()
	select {
	case <-r.done:
		t.Fatalf("app exited before serving: %v", r.err)
	case res := <-got:
		if res.err != nil || res.resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Fatalf("health = %v, %v, want SERVING", res.resp.GetStatus(), res.err)
		}
	}
}

func healthStatus(t *testing.T, r *running, service string) grpc_health_v1.HealthCheckResponse_ServingStatus {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: service})
	if err != nil {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN
	}
	return resp.GetStatus()
}

func principal(ctx context.Context, id, typ string, scopes ...string) context.Context {
	return metadata.AppendToOutgoingContext(ctx,
		"x-principal-id", id, "x-principal-type", typ, "x-principal-scopes", strings.Join(scopes, ","))
}

func asSeller(ctx context.Context, id string) context.Context {
	return principal(ctx, id, "user", "listing.read", "listing.write")
}

func assertPortFree(t *testing.T, port int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("port %d was bound by the failed boot: %v", port, err)
	}
	_ = l.Close()
}

func portAccepts(port int) bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func setDatabaseAvailable(t *testing.T, url string, available bool) {
	t.Helper()
	ctx := context.WithoutCancel(t.Context())
	admin, err := pgx.Connect(ctx, strings.Replace(url, "/test?", "/postgres?", 1))
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer func() { _ = admin.Close(ctx) }()
	qs := []string{"ALTER DATABASE test ALLOW_CONNECTIONS " + strconv.FormatBool(available)}
	if !available {
		qs = append(qs, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'test'")
	}
	for _, q := range qs {
		if _, err := admin.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

func seedListing(t *testing.T, pool *pgxpool.Pool, l store.Listing) {
	t.Helper()
	if _, err := store.NewPostgresListingRepository(pool).Create(t.Context(), l); err != nil {
		t.Fatalf("seed listing %s: %v", l.ID, err)
	}
}

func listingStatus(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var s string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM listings WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatalf("read status of %s: %v", id, err)
	}
	return s
}

func outboxStatus(t *testing.T, pool *pgxpool.Pool, eventID string) (string, int) {
	t.Helper()
	var s string
	var attempts int
	err := pool.QueryRow(t.Context(), `SELECT status, attempts FROM outbox_events WHERE event_id = $1`, eventID).Scan(&s, &attempts)
	if err != nil {
		t.Fatalf("read outbox row %s: %v", eventID, err)
	}
	return s, attempts
}

func waitLockWaiter(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	testkit.Eventually(t, 20*time.Second, func() bool {
		var n int
		err := pool.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND pid <> pg_backend_pid()`).Scan(&n)
		return err == nil && n > 0
	})
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func buildServer(t *testing.T) string {
	t.Helper()
	bin := t.TempDir() + "/server"
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, "./cmd/server")
	build.Dir = serviceRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/server: %v\n%s", err, out)
	}
	return bin
}

func bootBinary(t *testing.T, bin string, env ...string) (*syncBuffer, int) {
	t.Helper()
	port := testkit.FreePort(t)
	ctx, cancel := context.WithCancel(t.Context())
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = append(os.Environ(),
		"ENV=local", "KAFKA_BROKERS="+deadBroker, "GRPC_HOST=127.0.0.1", "GRPC_PORT="+strconv.Itoa(port),
		"LOG_LEVEL=info", "LOG_JSON=true", "OTEL_ENABLED=false", "LOG_KEY_FP_SECRET=test-log-key-fp-secret",
		"UPSTREAM_AI_ADDR=127.0.0.1:1", "UPSTREAM_AUDIT_ADDR=127.0.0.1:1",
	)
	cmd.Env = append(cmd.Env, env...)
	out := &syncBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out
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
	waitCtx, stop := context.WithTimeout(t.Context(), 30*time.Second)
	defer stop()
	if _, err := grpc_health_v1.NewHealthClient(conn).Check(waitCtx, &grpc_health_v1.HealthCheckRequest{Service: "liveness"}, grpc.WaitForReady(true)); err != nil {
		select {
		case e := <-exited:
			t.Fatalf("server exited before serving: %v\n%s", e, out.String())
		default:
		}
		t.Fatalf("server not reachable: %v\n%s", err, out.String())
	}
	return out, port
}
