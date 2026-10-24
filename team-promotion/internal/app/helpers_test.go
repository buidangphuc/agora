package app_test

import (
	"context"
	"io"
	"io/fs"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
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

	"github.com/buidangphuc/team-promotion/internal/app"
	"github.com/buidangphuc/team-promotion/internal/config"
	"github.com/buidangphuc/team-promotion/internal/store"
)

const (
	closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"
	deadBroker        = "127.0.0.1:1"
	healthWindow      = healthx.DefaultInterval + 1*time.Second
	eventsTopic       = "promotion.events"
)

var promotionTables = []string{"vouchers", "voucher_reservations", "flash_sale_campaigns", "flash_sale_holds", "promotion_outbox"}

var strayKeys = []string{
	"KAFKA_BROKERS", "GRPC_PORT", "GRPC_HOST", "LOG_LEVEL", "LOG_JSON", "OTEL_ENABLED",
	"GRPC_REFLECTION_ENABLED", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY", "DB_MAX_CONNS",
	"UPSTREAM_DOMAIN_ADDR", "UPSTREAM_AUDIT_ADDR", "FLIPT_ADDR",
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

func migrationsFS(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(store.Migrations, "migrations")
	if err != nil {
		t.Fatalf("sub migrations: %v", err)
	}
	return sub
}

func migratedURL(t *testing.T) string {
	t.Helper()
	url := freshURL(t)
	if err := migrate.Up(t.Context(), url, migrationsFS(t)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return url
}

func dial(t *testing.T, url string) *pgx.Conn {
	t.Helper()
	base := t.Context()
	c, err := pgx.Connect(base, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.WithoutCancel(base)) })
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

func schemaVersion(t *testing.T, url string) (int, bool) {
	t.Helper()
	var v int
	var dirty bool
	if err := dial(t, url).QueryRow(t.Context(), `SELECT version, dirty FROM schema_migrations`).Scan(&v, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return v, dirty
}

func promoSettings(port int, url, brokers string) config.Settings {
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGrace: 5 * time.Second},
		Postgres: sdkconfig.Postgres{URL: url, MaxConns: 5},
		Kafka:    sdkconfig.Kafka{Brokers: brokers},
		Outbox:   sdkconfig.Outbox{PollInterval: 200 * time.Millisecond, BatchSize: 100, ClaimLock: 60 * time.Second},
		Flags:    sdkconfig.Flags{FliptAddr: "127.0.0.1:1", EvalTimeout: 200 * time.Millisecond},
		Audit:    sdkconfig.Audit{Addr: "127.0.0.1:1"},
		Upstream: config.Upstream{DomainAddr: "127.0.0.1:1"},
		Ads:      config.Ads{MaxBid: 1000000, MaxBudget: 1000000000},
		Events:   config.Events{Topic: eventsTopic},
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
		case <-time.After(20 * time.Second):
			t.Error("app did not stop")
		}
	})
	return r
}

func waitServing(t *testing.T, r *running) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
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

func waitLiveness(t *testing.T, r *running) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	_, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "liveness"}, grpc.WaitForReady(true))
	if err != nil {
		select {
		case <-r.done:
			t.Fatalf("app exited before serving: %v", r.err)
		default:
		}
		t.Fatalf("server not reachable: %v", err)
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

func withPrincipal(ctx context.Context, id, typ string, scopes ...string) context.Context {
	return metadata.AppendToOutgoingContext(ctx,
		"x-principal-id", id, "x-principal-type", typ, "x-principal-scopes", strings.Join(scopes, ","))
}

func asSeller(ctx context.Context) context.Context {
	return withPrincipal(ctx, "seller-1", "user", "listing.write")
}

func asOrderService(ctx context.Context) context.Context {
	return withPrincipal(ctx, "service-team-order", "service", "promotion.reserve")
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
	base := context.WithoutCancel(t.Context())
	admin, err := pgx.Connect(base, strings.Replace(url, "/test?", "/postgres?", 1))
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer func() { _ = admin.Close(context.WithoutCancel(base)) }()
	qs := []string{"ALTER DATABASE test ALLOW_CONNECTIONS " + strconv.FormatBool(available)}
	if !available {
		qs = append(qs, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'test'")
	}
	for _, q := range qs {
		if _, err := admin.Exec(base, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

func seedCampaign(t *testing.T, url string, stockCap int64) (id, listingID string, price int64) {
	t.Helper()
	id, listingID, price = "camp-1", "l1", 90000
	_, err := dial(t, url).Exec(t.Context(),
		`INSERT INTO flash_sale_campaigns (id, listing_id, variant_id, sale_price, stock_cap, stock_sold, starts_at, ends_at)
		 VALUES ($1, $2, '', $3, $4, 0, now() - interval '1 hour', now() + interval '1 hour')`,
		id, listingID, price, stockCap)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return id, listingID, price
}

type forwarder struct {
	mu     sync.Mutex
	ln     net.Listener
	listen string
	target string
}

func newForwarder(t *testing.T, port int, target string) *forwarder {
	t.Helper()
	f := &forwarder{listen: "127.0.0.1:" + strconv.Itoa(port), target: target}
	t.Cleanup(f.stop)
	return f
}

func (f *forwarder) open(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", f.listen)
	if err != nil {
		t.Fatalf("forwarder listen: %v", err)
	}
	f.mu.Lock()
	f.ln = ln
	f.mu.Unlock()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.pipe(c)
		}
	}()
}

func (f *forwarder) pipe(c net.Conn) {
	var d net.Dialer
	up, err := d.DialContext(context.Background(), "tcp", f.target)
	if err != nil {
		_ = c.Close()
		return
	}
	go func() { _, _ = io.Copy(up, c); _ = up.Close() }()
	_, _ = io.Copy(c, up)
	_ = c.Close()
}

func (f *forwarder) stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ln != nil {
		_ = f.ln.Close()
	}
}
