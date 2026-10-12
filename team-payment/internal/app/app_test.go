package app_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-payment/internal/app"
	"github.com/buidangphuc/team-payment/internal/config"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func baseSettings(env string, port int) config.Settings {
	return config.Settings{
		Runtime: sdkconfig.Runtime{Env: env, LogLevel: "error", LogJSON: true},
		GRPC:    sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGraceSeconds: 10},
		Kafka:   sdkconfig.Kafka{Brokers: "unused:9092"},
		Events: config.Events{
			PaymentTopic: "payment.events", OrderTopic: "order.events",
			SettlementGroup: "team-payment.settlement", SettlementDLQTopic: "order.events.payment-settlement.dlq",
		},
	}
}

func TestBootFailsWhenDatabaseUnreachableDeployed(t *testing.T) {
	port := freePort(t)
	s := baseSettings("staging", port)
	s.Postgres = sdkconfig.Postgres{URL: "postgres://u:p@127.0.0.1:1/db?sslmode=disable"}

	if err := app.Run(context.Background(), s); err == nil {
		t.Fatal("Run succeeded with an unreachable database in ENV=staging")
	}
	// No port was opened before the boot check failed.
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("port %d was opened before boot failed: %v", port, err)
	}
	_ = l.Close()
}

// fakeProducer publishes to memory; a key in failKeys always fails.
type fakeProducer struct {
	mu       sync.Mutex
	failKeys map[string]bool
	ok       []string
}

func (p *fakeProducer) Publish(_ context.Context, _ string, key, _ []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failKeys[string(key)] {
		return errors.New("broker rejected the record")
	}
	p.ok = append(p.ok, string(key))
	return nil
}

func (p *fakeProducer) PublishRecord(context.Context, *kgo.Record) error { return nil }
func (p *fakeProducer) Close()                                           {}

func (p *fakeProducer) published() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.ok...)
}

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

func TestOutboxRelayedViaSDK(t *testing.T) {
	url := testkit.Postgres(t)
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	applyMigrations(t, admin)

	store := outbox.NewStore(admin, "payment_outbox_events")
	for _, r := range []outbox.Row{
		{EventID: "e-fail", AggregateType: "Payment", AggregateID: "order-fail", EventType: "t", Payload: []byte("a")},
		{EventID: "e-ok", AggregateType: "Payment", AggregateID: "order-ok", EventType: "t", Payload: []byte("b")},
	} {
		tx, err := admin.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.EnqueueTx(ctx, tx, r); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	s := baseSettings("local", freePort(t))
	s.Postgres = sdkconfig.Postgres{URL: url, MaxConns: 4}
	// OUTBOX_* now reach the relayer: payment used to ignore them.
	s.Outbox = sdkconfig.Outbox{PollInterval: 10 * time.Millisecond, BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 3}

	prod := &fakeProducer{failKeys: map[string]bool{"order-fail": true}}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- app.RunWith(runCtx, s, app.Options{
			NewProducer: func(sdkconfig.Kafka) (app.Producer, error) { return prod, nil },
		})
	}()

	status := func(id string) (st string, attempts int) {
		if err := admin.QueryRow(ctx, "SELECT status, attempts FROM payment_outbox_events WHERE event_id=$1", id).Scan(&st, &attempts); err != nil {
			t.Fatal(err)
		}
		return st, attempts
	}
	deadline := time.Now().Add(40 * time.Second)
	for {
		st, att := status("e-fail")
		if st == "failed" && att == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("e-fail: status=%s attempts=%d, want parked failed after OUTBOX_MAX_ATTEMPTS=3", st, att)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if st, _ := status("e-ok"); st != "published" {
		t.Fatalf("e-ok status = %s, want published despite the failing row", st)
	}
	if got := prod.published(); len(got) != 1 || got[0] != "order-ok" {
		t.Fatalf("published keys = %v, want [order-ok]", got)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("app did not stop")
	}
}
