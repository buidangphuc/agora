package app_test

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	sdkpgx "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-chat/internal/app"
	"github.com/buidangphuc/team-chat/internal/config"
	"github.com/buidangphuc/team-chat/migrations"
)

// closedDatabaseURL points at a port nothing listens on.
const closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

func settings(port int, env string, db sdkconfig.Postgres) config.Settings {
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: env, LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGrace: 5 * time.Second},
		Postgres: db,
		Events:   config.Events{ChatTopic: "chat.events"},
	}
}

func TestBootFailsWhenDatabaseUnreachableDeployed(t *testing.T) {
	port := testkit.FreePort(t)
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

// recorder is the Kafka seam: a testkit.Recorder that also has the Close the app
// calls on shutdown.
type recorder struct{ *testkit.Recorder }

func (recorder) Close() {}

func (r recorder) published() []string {
	var keys []string
	for _, rec := range r.Records() {
		keys = append(keys, string(rec.Key))
	}
	return keys
}

// Two rows are enqueued and publishing the first fails: the second (another
// aggregate) is still published, the first is retried with backoff and is never
// parked by a transient error (the relayer has no attempt limit since v1).
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

	rec := testkit.NewRecorder()
	rec.FailOn("thread-bad")
	pub := recorder{rec}
	s := settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url})
	s.Outbox = sdkconfig.Outbox{PollInterval: 50 * time.Millisecond, BatchSize: 10, ClaimLock: time.Minute}

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

	store, err := outbox.NewStore(pool, "chat_outbox_events")
	if err != nil {
		t.Fatal(err)
	}
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

	testkit.Eventually(t, 10*time.Second, func() bool {
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

	// Keep failing: the row stays pending, never parked, never published.
	time.Sleep(500 * time.Millisecond)
	if status, _, _ := row(); status != "pending" {
		t.Fatalf("failing row status = %q, want pending (transient errors never park)", status)
	}
	if got := pub.published(); len(got) != 1 {
		t.Fatalf("published = %v; the failing row must not be published", got)
	}
}

// assertRetriedWithBackoff waits for the first failed attempt and checks that
// the row went back to pending with a retry time in the future.
func assertRetriedWithBackoff(t *testing.T, row func() (string, int, time.Duration)) {
	t.Helper()
	var status string
	var attempts int
	var delay time.Duration
	testkit.Eventually(t, 10*time.Second, func() bool {
		status, attempts, delay = row()
		return attempts >= 1
	})
	if attempts == 1 && (status != "pending" || delay <= 0) {
		t.Fatalf("after the first failure: status=%q delay=%v, want pending with a backoff in the future", status, delay)
	}
}
