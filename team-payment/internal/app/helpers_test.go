package app_test

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-payment/internal/app"
	"github.com/buidangphuc/team-payment/internal/config"
	"github.com/buidangphuc/team-payment/migrations"
)

const (
	closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"
	deadBroker        = "127.0.0.1:1"
	healthWindow      = healthx.DefaultInterval + 1*time.Second
)

var strayKeys = []string{
	"KAFKA_BROKERS", "GRPC_PORT", "GRPC_HOST", "LOG_LEVEL", "LOG_JSON", "OTEL_ENABLED",
	"GRPC_REFLECTION_ENABLED", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY", "DB_MAX_CONNS",
	"UPSTREAM_ORDER_ADDR", "UPSTREAM_AUDIT_ADDR", "LOG_KEY_FP_SECRET", "MOCK_DATA", "MOCK_PAYMENTS",
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
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return url
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

func paySettings(port int, url, brokers string) config.Settings {
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGrace: 5 * time.Second},
		Postgres: sdkconfig.Postgres{URL: url, MaxConns: 5},
		Kafka:    sdkconfig.Kafka{Brokers: brokers},
		Outbox:   sdkconfig.Outbox{PollInterval: 200 * time.Millisecond, BatchSize: 100, ClaimLock: 60 * time.Second},
		LogKeyFP: sdkconfig.LogKeyFP{Secret: "test-log-key-fp-secret"},
		Events: config.Events{
			PaymentTopic:  "payment.events",
			OrderTopic:    "order.events",
			ConsumerGroup: "team-payment.settlement",
			DLQTopic:      "order.events.payment-settlement.dlq",
		},
		Upstream: config.Upstream{OrderAddr: "127.0.0.1:1", AuditAddr: "127.0.0.1:1"},
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
		case <-time.After(20 * time.Second):
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
		select {
		case <-r.done:
			t.Fatalf("app exited before serving: %v", r.err)
		default:
		}
		t.Fatalf("health = %v, %v, want SERVING", resp.GetStatus(), err)
	}
}

func healthStatus(r *running, service string) grpc_health_v1.HealthCheckResponse_ServingStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: service})
	if err != nil {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN
	}
	return resp.GetStatus()
}

func asUser(ctx context.Context, id string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-principal-id", id, "x-principal-type", "user")
}

func assertPortFree(t *testing.T, port int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("port %d was bound by the failed boot: %v", port, err)
	}
	_ = l.Close()
}

func setDatabaseAvailable(t *testing.T, url string, available bool) {
	t.Helper()
	admin, err := pgx.Connect(context.Background(), strings.Replace(url, "/test?", "/postgres?", 1))
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	qs := []string{"ALTER DATABASE test ALLOW_CONNECTIONS " + strconv.FormatBool(available)}
	if !available {
		qs = append(qs, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'test'")
	}
	for _, q := range qs {
		if _, err := admin.Exec(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
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
	build := exec.Command("go", "build", "-o", bin, "./cmd/server")
	build.Dir = serviceRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/server: %v\n%s", err, out)
	}
	return bin
}

func bootBinary(t *testing.T, bin string, env ...string) (*syncBuffer, int) {
	t.Helper()
	port := testkit.FreePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = append(os.Environ(),
		"ENV=local", "KAFKA_BROKERS="+deadBroker, "GRPC_HOST=127.0.0.1", "GRPC_PORT="+strconv.Itoa(port),
		"LOG_LEVEL=info", "LOG_JSON=false", "OTEL_ENABLED=false",
		"UPSTREAM_ORDER_ADDR=127.0.0.1:1", "UPSTREAM_AUDIT_ADDR=127.0.0.1:1",
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
	waitCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
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
