package outbox_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	sdkpgx "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

const table = "outbox_events"

type recorder struct {
	mu       sync.Mutex
	keys     []string
	failKeys map[string]bool
}

func (r *recorder) Publish(_ context.Context, _ string, key, _ []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failKeys[string(key)] {
		return errors.New("broker down")
	}
	r.keys = append(r.keys, string(key))
	return nil
}

func newStore(t *testing.T) (*pgxpool.Pool, *outbox.Store) {
	t.Helper()
	ctx := context.Background()
	pool, err := sdkpgx.Open(ctx, config.Postgres{URL: testkit.Postgres(t), MaxConns: 8})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, outbox.SchemaSQL(table)); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return pool, outbox.NewStore(pool, table)
}

func enqueue(t *testing.T, pool *pgxpool.Pool, s *outbox.Store, id string) {
	t.Helper()
	err := sdkpgx.WithTx(context.Background(), pool, func(tx pgx.Tx) error {
		return s.EnqueueTx(context.Background(), tx, outbox.Row{
			EventID: id, AggregateType: "Agg", AggregateID: "agg-" + id, EventType: "T", Payload: []byte(id),
		})
	})
	if err != nil {
		t.Fatalf("enqueue %s: %v", id, err)
	}
}

func TestConcurrentRelayersPublishOnce(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	const n = 200
	for i := range n {
		enqueue(t, pool, s, fmt.Sprintf("e%03d", i))
	}
	pub := &recorder{}
	opts := outbox.RelayerOptions{Topic: "t", Config: config.Outbox{BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 3}}
	a, b := outbox.NewRelayer(s, pub, opts), outbox.NewRelayer(s, pub, opts)

	var wg sync.WaitGroup
	for _, r := range []*outbox.Relayer{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				rep, err := r.RelayOnce(ctx)
				if err != nil {
					t.Errorf("relay: %v", err)
					return
				}
				if rep.Claimed == 0 {
					return
				}
			}
		}()
	}
	wg.Wait()

	seen := map[string]int{}
	for _, k := range pub.keys {
		seen[k]++
	}
	if len(seen) != n {
		t.Fatalf("published %d distinct keys, want %d", len(seen), n)
	}
	for k, c := range seen {
		if c != 1 {
			t.Errorf("key %s published %d times", k, c)
		}
	}
	var published int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE status = 'published' AND error IS NULL`).Scan(&published); err != nil || published != n {
		t.Fatalf("published rows = %d (err %v), want %d", published, err, n)
	}
}

func TestFailingRowBacksOffThenParks(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	enqueue(t, pool, s, "bad")
	enqueue(t, pool, s, "good")
	pub := &recorder{failKeys: map[string]bool{"agg-bad": true}}
	r := outbox.NewRelayer(s, pub, outbox.RelayerOptions{
		Topic: "t", Config: config.Outbox{BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 3},
		BaseBackoff: time.Minute, MaxBackoff: time.Hour,
	})

	type state struct {
		status   string
		attempts int
		delay    time.Duration
		errText  *string
	}
	read := func() state {
		var st state
		err := pool.QueryRow(ctx, `SELECT status, attempts, available_at - now(), error FROM `+table+` WHERE event_id = 'bad'`).
			Scan(&st.status, &st.attempts, &st.delay, &st.errText)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		return st
	}
	release := func() {
		if _, err := pool.Exec(ctx, `UPDATE `+table+` SET available_at = now() WHERE event_id = 'bad'`); err != nil {
			t.Fatalf("release: %v", err)
		}
	}

	rep, err := r.RelayOnce(ctx)
	if err != nil || rep != (outbox.Report{Claimed: 2, Published: 1, Retried: 1}) {
		t.Fatalf("pass 1 = %+v, %v", rep, err)
	}
	st1 := read()
	if st1.status != "pending" || st1.attempts != 1 || st1.errText == nil || st1.delay < 50*time.Second || st1.delay > 61*time.Second {
		t.Fatalf("after pass 1: %+v", st1)
	}
	// Still gated: nothing to claim.
	if rep, _ := r.RelayOnce(ctx); rep.Claimed != 0 {
		t.Fatalf("gated row was claimed: %+v", rep)
	}

	release()
	if rep, _ := r.RelayOnce(ctx); rep.Retried != 1 {
		t.Fatalf("pass 2 = %+v", rep)
	}
	st2 := read()
	if st2.attempts != 2 || st2.delay < 110*time.Second || st2.delay > 121*time.Second {
		t.Fatalf("backoff did not double: %+v", st2)
	}

	release()
	rep, _ = r.RelayOnce(ctx)
	if rep.Parked != 1 {
		t.Fatalf("pass 3 = %+v", rep)
	}
	if st := read(); st.status != "failed" || st.attempts != 3 {
		t.Fatalf("after pass 3: %+v", st)
	}
	release()
	if rep, _ := r.RelayOnce(ctx); rep.Claimed != 0 {
		t.Fatalf("parked row was claimed: %+v", rep)
	}
	if len(pub.keys) != 1 || pub.keys[0] != "agg-good" {
		t.Fatalf("published = %v, want only agg-good", pub.keys)
	}
}

func TestEnqueueRollsBackWithTx(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	boom := errors.New("boom")
	err := sdkpgx.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if err := s.EnqueueTx(ctx, tx, outbox.Row{EventID: "e1", AggregateType: "A", AggregateID: "a", EventType: "T", Payload: []byte("x")}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows = %d (err %v), want 0", n, err)
	}
}

func TestInvalidTableNamePanics(t *testing.T) {
	for _, name := range []string{"", "Outbox", "a;drop table x", "1abc", "a-b"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewStore(%q) did not panic", name)
				}
			}()
			outbox.NewStore(nil, name)
		}()
	}
}
