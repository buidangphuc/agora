package app_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-identity/internal/app"
	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/migrations"
)

// closedDatabaseURL points at a port nothing listens on.
const closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

const outboxTable = "identity_outbox_events"

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

func signingKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func settings(t *testing.T, env string, db sdkconfig.Postgres) config.Settings {
	t.Helper()
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: env, LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: freePort(t), ShutdownGraceSeconds: 5},
		Postgres: db,
		Kafka:    sdkconfig.Kafka{Brokers: "127.0.0.1:1"},
		Events:   config.Events{IdentityTopic: "identity.events"},
		Outbox: sdkconfig.Outbox{
			PollInterval: 50 * time.Millisecond, BatchSize: 10, ClaimLockSeconds: 5, MaxAttempts: 2,
		},
		JWT: config.JWT{PrivateKey: signingKeyPEM(t), KID: "test-kid", JWKSHTTPPort: freePort(t), TTLSeconds: 3600},
	}
}

func TestBootFailsWhenDatabaseUnreachableDeployed(t *testing.T) {
	s := settings(t, "staging", sdkconfig.Postgres{URL: closedDatabaseURL})

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
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(s.Port))
	if err != nil {
		t.Fatalf("port %d was bound by the failed boot: %v", s.Port, err)
	}
	_ = l.Close()
}

// recordingPublisher fails every publish for one key and records each call.
type recordingPublisher struct {
	failKey string

	mu    sync.Mutex
	calls []publishCall
}

type publishCall struct {
	topic, key string
	at         time.Time
}

func (p *recordingPublisher) Publish(_ context.Context, topic string, key, _ []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, publishCall{topic: topic, key: string(key), at: time.Now()})
	if string(key) == p.failKey {
		return context.DeadlineExceeded
	}
	return nil
}

func (p *recordingPublisher) callsFor(key string) []publishCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []publishCall
	for _, c := range p.calls {
		if c.key == key {
			out = append(out, c)
		}
	}
	return out
}

func rowState(t *testing.T, pool *pgxpool.Pool, id string) (status string, attempts int, lastErr *string) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT status, attempts, error FROM `+outboxTable+` WHERE event_id = $1`, id).Scan(&status, &attempts, &lastErr)
	if err != nil {
		t.Fatalf("read outbox row %s: %v", id, err)
	}
	return status, attempts, lastErr
}

func TestOutboxRelayedViaSDK(t *testing.T) {
	url := testkit.Postgres(t)
	ctx := context.Background()
	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Two rows enqueued by the SDK store, as RevokeSession does: the first
	// cannot be published, the second can.
	store := outbox.NewStore(pool, outboxTable)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []outbox.Row{
		{EventID: "evt-bad", AggregateType: "Session", AggregateID: "user-bad", EventType: "T", Payload: []byte("a")},
		{EventID: "evt-good", AggregateType: "Session", AggregateID: "user-good", EventType: "T", Payload: []byte("b")},
	} {
		if err := store.EnqueueTx(ctx, tx, r); err != nil {
			t.Fatalf("enqueue %s: %v", r.EventID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	pub := &recordingPublisher{failKey: "user-bad"}
	s := settings(t, "local", sdkconfig.Postgres{URL: url})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- app.RunWith(runCtx, s, app.Options{Publisher: pub}) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("app did not stop")
		}
	})

	deadline := time.Now().Add(30 * time.Second)
	for {
		if st, _, _ := rowState(t, pool, "evt-bad"); st == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("evt-bad was not parked in time")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The failing row did not block the other one.
	if st, _, _ := rowState(t, pool, "evt-good"); st != "published" {
		t.Errorf("evt-good status = %q, want published", st)
	}
	if got := len(pub.callsFor("user-good")); got != 1 {
		t.Errorf("evt-good published %d times, want 1", got)
	}
	// The failing row was retried behind a backoff, then parked at MaxAttempts.
	st, attempts, lastErr := rowState(t, pool, "evt-bad")
	if st != "failed" || attempts != s.Outbox.MaxAttempts || lastErr == nil || *lastErr == "" {
		t.Errorf("evt-bad = status %q attempts %d error %v, want failed, %d, a message", st, attempts, lastErr, s.Outbox.MaxAttempts)
	}
	bad := pub.callsFor("user-bad")
	if len(bad) != s.Outbox.MaxAttempts {
		t.Fatalf("evt-bad was published %d times, want %d", len(bad), s.Outbox.MaxAttempts)
	}
	if gap := bad[1].at.Sub(bad[0].at); gap < 900*time.Millisecond {
		t.Errorf("retry came after %v, want a backoff of about 1s", gap)
	}
	for _, c := range pub.calls {
		if c.topic != s.Events.IdentityTopic {
			t.Errorf("published to %q, want %q", c.topic, s.Events.IdentityTopic)
		}
	}
}
