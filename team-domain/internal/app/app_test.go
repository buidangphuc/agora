package app_test

import (
	"bytes"
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-domain/internal/app"
	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/migrations"
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
		GRPC:    sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGrace: 10 * time.Second},
	}
}

func TestBootRequiresDatabase(t *testing.T) {
	port := freePort(t)
	s := baseSettings(port)
	s.Postgres = sdkconfig.Postgres{}

	err := app.Run(context.Background(), s)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Run = %v, want an error naming DATABASE_URL", err)
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
	s.Postgres = sdkconfig.Postgres{URL: "postgres://u:p@127.0.0.1:1/db?sslmode=disable"}

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

func (p *fakePublisher) PublishRecord(_ context.Context, rec *kgo.Record) error {
	p.mu.Lock()
	p.calls = append(p.calls, "produce:"+string(rec.Key))
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

var _ app.Publisher = (*fakePublisher)(nil)

func TestShutdownStopsRelayerBeforePoolClose(t *testing.T) {
	// Every migration applied, as the compose job team-domain-migrate does in the stack.
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	ctx := context.Background()
	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ob, err := outbox.NewStore(admin, store.OutboxTable)
	if err != nil {
		t.Fatal(err)
	}
	if err := postgres.WithTx(ctx, admin, func(tx pgx.Tx) error {
		return ob.EnqueueTx(ctx, tx, outbox.Row{
			EventID: "e1", AggregateType: "Listing", AggregateID: "l1", EventType: "t", Payload: []byte("env"),
		})
	}); err != nil {
		t.Fatalf("seed outbox row: %v", err)
	}

	const appName = "team-domain-shutdown-test"
	port := freePort(t)
	s := baseSettings(port)
	s.Postgres = sdkconfig.Postgres{URL: url + "&application_name=" + appName, MaxConns: 4}
	s.Kafka = sdkconfig.Kafka{Brokers: "unused:9092"}
	s.Events = config.Events{ListingTopic: "listing.events"}
	s.Outbox = sdkconfig.Outbox{PollInterval: 10 * time.Millisecond, BatchSize: 10, ClaimLock: time.Minute}

	pub := &fakePublisher{produced: make(chan struct{}, 1)}
	logs := &syncBuffer{}
	opts := app.Options{
		NewPublisher: func(sdkconfig.Kafka) (app.Publisher, error) { return pub, nil },
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
