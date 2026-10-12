package app_test

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-domain/internal/app"
	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/events"
	"github.com/buidangphuc/team-domain/internal/store"
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

func baseSettings(port int) config.Settings {
	return config.Settings{
		Runtime: sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		GRPC:    sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGraceSeconds: 10},
	}
}

func TestBootRequiresDatabase(t *testing.T) {
	port := freePort(t)
	s := baseSettings(port)
	s.Postgres = sdkconfig.Postgres{Enabled: false}

	err := app.Run(context.Background(), s)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_ENABLED") {
		t.Fatalf("Run = %v, want an error naming DATABASE_ENABLED", err)
	}
	// No port was opened: the service port is still free to bind.
	l, lerr := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if lerr != nil {
		t.Fatalf("port %d was opened before the boot check failed: %v", port, lerr)
	}
	_ = l.Close()
}

func TestBootFailsWhenDatabaseUnreachable(t *testing.T) {
	s := baseSettings(freePort(t))
	s.Postgres = sdkconfig.Postgres{Enabled: true, URL: "postgres://u:p@127.0.0.1:1/db?sslmode=disable"}

	if err := app.Run(context.Background(), s); err == nil {
		t.Fatal("Run succeeded with an unreachable database in ENV=local")
	}
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// fakePublisher records, in order, what the relayer produced and when the
// publisher was closed.
type fakePublisher struct {
	mu       sync.Mutex
	calls    []string
	produced chan struct{}
}

func (p *fakePublisher) ProduceRaw(_ context.Context, key string, _ []byte) error {
	p.mu.Lock()
	p.calls = append(p.calls, "produce:"+key)
	p.mu.Unlock()
	select {
	case p.produced <- struct{}{}:
	default:
	}
	return nil
}

func (p *fakePublisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "close")
}

func (p *fakePublisher) log() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

var _ events.Publisher = (*fakePublisher)(nil)

// applyMigrations applies every migrations/*.up.sql in order, as the compose
// job team-domain-migrate does in the stack.
func applyMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v (n=%d)", err, len(files))
	}
	sort.Strings(files)
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(f), err)
		}
	}
}

func TestShutdownStopsRelayerBeforePoolClose(t *testing.T) {
	url := testkit.Postgres(t)
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	applyMigrations(t, admin)
	if err := store.NewOutboxStore(admin).EnqueueTx(ctx, admin, store.OutboxRow{
		EventID: "e1", AggregateType: "Listing", AggregateID: "l1", EventType: "t", Payload: []byte("env"),
	}); err != nil {
		t.Fatalf("seed outbox row: %v", err)
	}

	const appName = "team-domain-shutdown-test"
	port := freePort(t)
	s := baseSettings(port)
	s.Postgres = sdkconfig.Postgres{Enabled: true, URL: url + "&application_name=" + appName, MaxConns: 4}
	s.Events = config.Events{KafkaEnabled: true, Brokers: "unused:9092", ListingTopic: "listing.events"}
	s.Outbox = config.Outbox{Enabled: true, PollInterval: "10ms", BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 10}

	pub := &fakePublisher{produced: make(chan struct{}, 1)}
	logs := &syncBuffer{}
	opts := app.Options{
		NewPublisher: func([]string, string) (events.Publisher, error) { return pub, nil },
		LogOut:       logs,
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.RunWith(runCtx, s, opts) }()

	conn, err := grpc.NewClient("127.0.0.1:"+strconv.Itoa(port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	watchCtx, stopWatch := context.WithTimeout(ctx, 30*time.Second)
	defer stopWatch()
	stream, err := grpc_health_v1.NewHealthClient(conn).Watch(watchCtx, &grpc_health_v1.HealthCheckRequest{}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	first, err := stream.Recv()
	if err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("first status = %v, %v, want SERVING", first.GetStatus(), err)
	}
	select {
	case <-pub.produced:
	case <-time.After(15 * time.Second):
		t.Fatal("the relayer never produced the seeded outbox row")
	}

	cancel()

	// Health drains first: NOT_SERVING reaches the open watcher, which keeps the
	// graceful stop pending.
	next, err := stream.Recv()
	if err != nil || next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("status after cancel = %v, %v, want NOT_SERVING", next.GetStatus(), err)
	}
	stopWatch()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("app did not stop")
	}

	calls := pub.log()
	closes := 0
	for i, c := range calls {
		if c == "close" {
			closes++
			if i != len(calls)-1 {
				t.Fatalf("publisher used after Close: %v", calls)
			}
		}
	}
	if closes != 1 {
		t.Fatalf("publisher closed %d times, want 1: %v", closes, calls)
	}
	if out := logs.String(); strings.Contains(out, "outbox relay pass failed") {
		t.Fatalf("relayer ran against a closed pool:\n%s", out)
	}

	// The app's pool is closed by the time Run returns.
	var open int
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM pg_stat_activity WHERE application_name = $1", appName).Scan(&open); err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("%d connections of the service pool still open after Run returned", open)
	}
}
