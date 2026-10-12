package app_test

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	sdkpgx "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-chat/internal/app"
	"github.com/buidangphuc/team-chat/internal/config"
	"github.com/buidangphuc/team-chat/migrations"
)

// closedDatabaseURL points at a port nothing listens on.
const closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

// freePort asks the kernel for an unused port, then releases it for the
// service to bind. GRPC_PORT has no default and 0 is not usable.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func settings(port int, env string, db sdkconfig.Postgres) config.Settings {
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: env, LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGraceSeconds: 5},
		Postgres: db,
		Events:   config.Events{ChatTopic: "chat.events"},
	}
}

func TestBootFailsWhenDatabaseUnreachableDeployed(t *testing.T) {
	port := freePort(t)
	s := settings(port, "staging", sdkconfig.Postgres{URL: closedDatabaseURL})

	done := make(chan error, 1)
	go func() { done <- app.Run(context.Background(), s) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil with an unreachable database in ENV=staging")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not fail; it is serving without its database")
	}

	// No listener was opened: the port is still free.
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("port %d was bound by the failed boot: %v", port, err)
	}
	_ = l.Close()
}

// recorder is the Kafka seam: it fails every publish for failKey and records the
// keys it accepted.
type recorder struct {
	mu      sync.Mutex
	failKey string
	keys    []string
}

func (r *recorder) Publish(_ context.Context, _ string, key, _ []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if string(key) == r.failKey {
		return errors.New("broker unavailable")
	}
	r.keys = append(r.keys, string(key))
	return nil
}

func (r *recorder) Close() {}

func (r *recorder) published() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.keys...)
}

func eventually(t *testing.T, within time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for %s", within, what)
}

// Two rows are enqueued and publishing the first fails: the second is still
// published, the first is retried with backoff and parked after
// OUTBOX_MAX_ATTEMPTS.
func TestOutboxRelayedViaSDK(t *testing.T) {
	ctx := context.Background()
	url := testkit.Postgres(t)
	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := sdkpgx.Open(ctx, sdkconfig.Postgres{URL: url, MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	pub := &recorder{failKey: "thread-bad"}
	s := settings(freePort(t), "local", sdkconfig.Postgres{URL: url})
	s.Outbox = sdkconfig.Outbox{PollInterval: 50 * time.Millisecond, BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 2}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- app.RunWith(runCtx, s, app.Options{
			NewPublisher: func(sdkconfig.Kafka) (app.Publisher, error) { return pub, nil },
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("app did not stop")
		}
	})

	store := outbox.NewStore(pool, "chat_outbox_events")
	for _, id := range []string{"bad", "good"} { // the failing row is the older one
		err := sdkpgx.WithTx(ctx, pool, func(tx pgx.Tx) error {
			return store.EnqueueTx(ctx, tx, outbox.Row{
				EventID: "e-" + id, AggregateType: "ChatThread", AggregateID: "thread-" + id,
				EventType: "T", Payload: []byte(id),
			})
		})
		if err != nil {
			t.Fatalf("enqueue %s: %v", id, err)
		}
	}

	eventually(t, 10*time.Second, "the good row to be published past the failing one", func() bool {
		return len(pub.published()) == 1 && pub.published()[0] == "thread-good"
	})

	row := func() (status string, attempts int, delay time.Duration) {
		t.Helper()
		if err := pool.QueryRow(ctx,
			`SELECT status, attempts, available_at - now() FROM chat_outbox_events WHERE event_id = 'e-bad'`,
		).Scan(&status, &attempts, &delay); err != nil {
			t.Fatalf("read failing row: %v", err)
		}
		return status, attempts, delay
	}
	assertRetriedWithBackoff(t, row)

	eventually(t, 15*time.Second, "the failing row to be parked", func() bool {
		status, attempts, _ := row()
		return status == "failed" && attempts == 2
	})
	if got := pub.published(); len(got) != 1 {
		t.Fatalf("published = %v; the parked row must never be published", got)
	}
}

// assertRetriedWithBackoff waits for the first failed attempt and checks that
// the row went back to pending with a retry time in the future.
func assertRetriedWithBackoff(t *testing.T, row func() (string, int, time.Duration)) {
	t.Helper()
	var status string
	var attempts int
	var delay time.Duration
	eventually(t, 10*time.Second, "the failing row to be retried", func() bool {
		status, attempts, delay = row()
		return attempts >= 1
	})
	if attempts == 1 && (status != "pending" || delay <= 0) {
		t.Fatalf("after the first failure: status=%q delay=%v, want pending with a backoff in the future", status, delay)
	}
}
