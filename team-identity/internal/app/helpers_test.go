package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	auditv1 "github.com/buidangphuc/team-identity/generated/platform/audit/v1"
	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/store"
)

const (
	closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"
	deadBroker        = "127.0.0.1:1"
	latestVersion     = 19
	healthWindow      = healthx.DefaultInterval + 2*time.Second
	testPassword      = "Sup3r-secret-pass!"
)

var (
	keyOnce sync.Once
	keyPEM  string
)

func testSigningKey(t testing.TB) string {
	t.Helper()
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		der, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			t.Fatalf("marshal key: %v", err)
		}
		keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	})
	return keyPEM
}

var composeKey = regexp.MustCompile(`JWT_PRIVATE_KEY='(-----BEGIN [^']+)'`)

func committedDevPEM(t testing.TB) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docker-compose.services.yaml"))
	if err != nil {
		t.Fatalf("compose file with the committed development key: %v", err)
	}
	m := composeKey.FindSubmatch(raw)
	if m == nil {
		t.Fatal("no JWT_PRIVATE_KEY in docker-compose.services.yaml")
	}
	return strings.ReplaceAll(string(m[1]), `\n`, "\n")
}

func freshURL(t testing.TB) string {
	t.Helper()
	return testkit.Schema(t, testkit.SharedPostgres(t))
}

func migrateTo(t testing.TB, url string, version int) {
	t.Helper()
	src := store.Migrations()
	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	sub := tempFS{}
	for _, e := range entries {
		v, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0])
		if err != nil || v > version {
			continue
		}
		b, err := fs.ReadFile(src, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		sub[e.Name()] = b
	}
	if err := migrate.Up(context.Background(), url, sub.fs()); err != nil {
		t.Fatalf("migrate to %d: %v", version, err)
	}
}

func migratedURL(t testing.TB) string {
	t.Helper()
	url := freshURL(t)
	migrateTo(t, url, latestVersion)
	return url
}

func dial(t testing.TB, url string) *pgx.Conn {
	t.Helper()
	c, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

func execSQL(t testing.TB, url, sql string, args ...any) {
	t.Helper()
	if _, err := dial(t, url).Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func queryInt(t testing.TB, url, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := dial(t, url).QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	return n
}

func queryString(t testing.TB, url, sql string, args ...any) string {
	t.Helper()
	var s string
	if err := dial(t, url).QueryRow(context.Background(), sql, args...).Scan(&s); err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	return s
}

func tableExists(t testing.TB, url, table string) bool {
	t.Helper()
	return queryInt(t, url, `SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1`, table) == 1
}

func schemaVersion(t testing.TB, url string) (int, bool) {
	t.Helper()
	var v int
	var dirty bool
	if err := dial(t, url).QueryRow(context.Background(), `SELECT version, dirty FROM schema_migrations`).Scan(&v, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return v, dirty
}

func newSettings(t testing.TB, url, brokers string) config.Settings {
	t.Helper()
	return config.Settings{
		Runtime:     sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		GRPC:        sdkconfig.GRPC{Host: "127.0.0.1", Port: testkit.FreePort(t), ShutdownGrace: 5 * time.Second},
		Postgres:    sdkconfig.Postgres{URL: url, MaxConns: 5},
		Kafka:       sdkconfig.Kafka{Brokers: brokers},
		LogKeyFP:    sdkconfig.LogKeyFP{Secret: "test-fp-secret"},
		Outbox:      sdkconfig.Outbox{PollInterval: 200 * time.Millisecond, BatchSize: 100, ClaimLock: 60 * time.Second},
		OutboxPurge: config.OutboxPurge{Retention: 168 * time.Hour, Interval: time.Hour},
		Events:      config.Events{IdentityTopic: "identity.events"},
		JWT:         config.JWT{PrivateKey: testSigningKey(t), KID: "test-kid", JWKSHTTPPort: testkit.FreePort(t), TTLSeconds: 3600},
		Upstream:    config.Upstream{AuditAddr: "127.0.0.1:1"},
		Retention: config.Retention{
			Session: 720 * time.Hour, LoginHistory: 4320 * time.Hour, SweepInterval: time.Hour, SweepBatchSize: 500,
		},
	}
}

type running struct {
	conn   *grpc.ClientConn
	cancel context.CancelFunc
	done   chan struct{}
	err    error
	s      config.Settings
}

func start(t *testing.T, s config.Settings) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan struct{}), s: s}
	go func() { r.err = Run(ctx, s); close(r.done) }()
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

func (r *running) stop(t *testing.T) {
	t.Helper()
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(30 * time.Second):
		t.Fatal("app did not stop")
	}
}

func waitLive(t testing.TB, conn *grpc.ClientConn, done <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go func() {
		select {
		case <-done:
			cancel()
		case <-ctx.Done():
		}
	}()
	resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "liveness"}, grpc.WaitForReady(true))
	if err != nil || resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		select {
		case <-done:
			t.Fatal("app exited before serving")
		default:
		}
		t.Fatalf("liveness = %v, %v, want SERVING", resp.GetStatus(), err)
	}
}

func waitServing(t *testing.T, r *running) {
	t.Helper()
	waitLive(t, r.conn, r.done)
	testkit.Eventually(t, 30*time.Second, func() bool {
		return healthStatus(r.conn, "") == grpc_health_v1.HealthCheckResponse_SERVING
	})
}

func healthStatus(conn *grpc.ClientConn, service string) grpc_health_v1.HealthCheckResponse_ServingStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: service})
	if err != nil {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN
	}
	return resp.GetStatus()
}

func dialApp(t *testing.T, port int) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("127.0.0.1:"+strconv.Itoa(port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func asUser(ctx context.Context, id string, scopes ...string) context.Context {
	pairs := []string{"x-principal-id", id, "x-principal-type", "user"}
	if len(scopes) > 0 {
		pairs = append(pairs, "x-principal-scopes", strings.Join(scopes, ","))
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

func assertPortFree(t testing.TB, port int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("port %d is bound: %v", port, err)
	}
	_ = l.Close()
}

type tcpTrap struct {
	url string
	mu  sync.Mutex
	n   int
}

func newTCPTrap(t testing.TB) *tcpTrap {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tr := &tcpTrap{url: "postgres://u:p@" + l.Addr().String() + "/db?sslmode=disable"}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			tr.mu.Lock()
			tr.n++
			tr.mu.Unlock()
			_ = c.Close()
		}
	}()
	t.Cleanup(func() { _ = l.Close() })
	return tr
}

func (tr *tcpTrap) accepted() int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.n
}

type logCapture struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	done chan struct{}
}

func captureLogs(t testing.TB) *logCapture {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	c := &logCapture{done: make(chan struct{})}
	orig := os.Stdout
	os.Stdout = w
	go func() {
		defer close(c.done)
		b := make([]byte, 4096)
		for {
			n, err := r.Read(b)
			if n > 0 {
				c.mu.Lock()
				c.buf.Write(b[:n])
				c.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		os.Stdout = orig
		_ = w.Close()
		<-c.done
		_ = r.Close()
	})
	return c
}

func (c *logCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func (c *logCapture) Lines() []map[string]any {
	var out []map[string]any
	for _, l := range strings.Split(c.String(), "\n") {
		var m map[string]any
		if strings.HasPrefix(strings.TrimSpace(l), "{") && json.Unmarshal([]byte(l), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func captureSpans(t testing.TB) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	})
	return exp
}

func spanText(exp *tracetest.InMemoryExporter) string {
	var b strings.Builder
	for _, s := range exp.GetSpans() {
		b.WriteString(s.Name + "\n")
		for _, a := range s.Attributes {
			b.WriteString(string(a.Key) + "=" + a.Value.String() + "\n")
		}
		for _, e := range s.Events {
			b.WriteString(e.Name + "\n")
			for _, a := range e.Attributes {
				b.WriteString(string(a.Key) + "=" + a.Value.String() + "\n")
			}
		}
		b.WriteString(s.Status.Description + "\n")
	}
	return b.String()
}

type fakeAudit struct {
	auditv1.UnimplementedAuditServiceServer
	mu      sync.Mutex
	reqs    []*auditv1.WriteAuditEventRequest
	block   chan struct{}
	entered chan struct{}
}

func (f *fakeAudit) WriteAuditEvent(ctx context.Context, req *auditv1.WriteAuditEventRequest) (*auditv1.WriteAuditEventResponse, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	f.mu.Unlock()
	if f.entered != nil {
		select {
		case f.entered <- struct{}{}:
		default:
		}
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &auditv1.WriteAuditEventResponse{}, nil
}

func startAudit(t testing.TB, f *fakeAudit) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	auditv1.RegisterAuditServiceServer(srv, f)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func metadataRequestID(ctx context.Context, id string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-request-id", id)
}
