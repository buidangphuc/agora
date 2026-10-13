package outbox_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/trace"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	sdkpgx "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

const table = "outbox_events"

type recorder struct {
	mu       sync.Mutex
	keys     []string
	recs     []*kgo.Record
	failKeys map[string]bool
	failIDs  map[string]bool
	block    bool // block until the publish ctx ends, like a broker outage
}

func header(r *kgo.Record, name string) string {
	for _, h := range r.Headers {
		if h.Key == name {
			return string(h.Value)
		}
	}
	return ""
}

func (r *recorder) PublishRecord(ctx context.Context, rec *kgo.Record) error {
	if r.block {
		<-ctx.Done()
		return ctx.Err()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failKeys[string(rec.Key)] || r.failIDs[header(rec, "event_id")] {
		return errors.New("broker down")
	}
	r.keys = append(r.keys, string(rec.Key))
	r.recs = append(r.recs, rec)
	return nil
}

func (r *recorder) eventIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	for _, rec := range r.recs {
		ids = append(ids, header(rec, "event_id"))
	}
	return ids
}

func (r *recorder) setFailIDs(ids ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failIDs = map[string]bool{}
	for _, id := range ids {
		r.failIDs[id] = true
	}
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

func TestEnqueueDuplicateIsNoop(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	row := outbox.Row{EventID: "dup", AggregateType: "Agg", AggregateID: "agg-dup", EventType: "T", Payload: []byte("p")}
	err := sdkpgx.WithTx(ctx, pool, func(tx pgx.Tx) error {
		for range 2 {
			if err := s.EnqueueTx(ctx, tx, row); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("duplicate enqueue in one tx: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE event_id = 'dup'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}

func enqueueAgg(t *testing.T, pool *pgxpool.Pool, s *outbox.Store, agg string, ids ...string) {
	t.Helper()
	err := sdkpgx.WithTx(context.Background(), pool, func(tx pgx.Tx) error {
		for _, id := range ids {
			if err := s.EnqueueTx(context.Background(), tx, outbox.Row{
				EventID: id, AggregateType: "Agg", AggregateID: agg, EventType: "T", Payload: []byte(id),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("enqueue %v: %v", ids, err)
	}
}

func drain(t *testing.T, r *outbox.Relayer, passes int) {
	t.Helper()
	for range passes {
		rep, err := r.RelayOnce(context.Background())
		if err != nil {
			t.Fatalf("relay: %v", err)
		}
		if rep.Published == 0 {
			return
		}
	}
}

func relayer(s *outbox.Store, p outbox.Publisher, cfg config.Outbox) *outbox.Relayer {
	return outbox.NewRelayer(s, p, outbox.RelayerOptions{Topic: "t", Config: cfg, BaseBackoff: time.Minute, MaxBackoff: time.Hour})
}

func TestOrderPerAggregate(t *testing.T) {
	pool, s := newStore(t)
	// Events of one transaction share created_at; order must still hold.
	enqueueAgg(t, pool, s, "A", "a1", "a2", "a3", "a4", "a5")
	enqueueAgg(t, pool, s, "B", "b1", "b2")
	pub := &recorder{}
	drain(t, relayer(s, pub, config.Outbox{BatchSize: 3, ClaimLockSeconds: 60, MaxAttempts: 3}), 20)

	var a, b []string
	for _, id := range pub.eventIDs() {
		if id[0] == 'a' {
			a = append(a, id)
		} else {
			b = append(b, id)
		}
	}
	if !slices.Equal(a, []string{"a1", "a2", "a3", "a4", "a5"}) || !slices.Equal(b, []string{"b1", "b2"}) {
		t.Fatalf("per-aggregate order broken: a=%v b=%v", a, b)
	}
}

func TestFailingRowBlocksItsAggregateOnly(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	enqueueAgg(t, pool, s, "A", "a1", "a2")
	enqueueAgg(t, pool, s, "B", "b1")
	pub := &recorder{}
	pub.setFailIDs("a1")
	r := relayer(s, pub, config.Outbox{BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 5})

	rep, err := r.RelayOnce(ctx)
	if err != nil || rep.Retried != 1 || rep.Published != 1 {
		t.Fatalf("pass 1 = %+v, %v", rep, err)
	}
	// a1 backs off; a2 must not overtake it, b1 is already out.
	if _, err := pool.Exec(ctx, `UPDATE `+table+` SET available_at = now() + interval '1 hour' WHERE event_id = 'a1'`); err != nil {
		t.Fatal(err)
	}
	if rep, _ := r.RelayOnce(ctx); rep.Claimed != 0 {
		t.Fatalf("a2 overtook a backing-off a1: %+v", rep)
	}
	if got := pub.eventIDs(); !slices.Equal(got, []string{"b1"}) {
		t.Fatalf("published = %v, want [b1]", got)
	}

	// Once a1 recovers the order is a1, a2.
	pub.setFailIDs()
	if _, err := pool.Exec(ctx, `UPDATE `+table+` SET available_at = now() WHERE event_id = 'a1'`); err != nil {
		t.Fatal(err)
	}
	drain(t, r, 5)
	if got := pub.eventIDs(); !slices.Equal(got, []string{"b1", "a1", "a2"}) {
		t.Fatalf("published = %v, want [b1 a1 a2]", got)
	}
}

func TestParkedRowBlocksItsAggregate(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	enqueueAgg(t, pool, s, "A", "a1", "a2")
	pub := &recorder{}
	pub.setFailIDs("a1")
	r := relayer(s, pub, config.Outbox{BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 1})
	if rep, _ := r.RelayOnce(ctx); rep.Parked != 1 {
		t.Fatalf("a1 not parked: %+v", rep)
	}
	pub.setFailIDs()
	if rep, _ := r.RelayOnce(ctx); rep.Claimed != 0 {
		t.Fatalf("a2 published past a parked a1: %+v", rep)
	}
	// Operator requeues the parked row; order resumes.
	if _, err := pool.Exec(ctx, `UPDATE `+table+` SET status = 'pending', available_at = now() WHERE event_id = 'a1'`); err != nil {
		t.Fatal(err)
	}
	drain(t, r, 5)
	if got := pub.eventIDs(); !slices.Equal(got, []string{"a1", "a2"}) {
		t.Fatalf("published = %v", got)
	}
}

func TestPublishedRecordCarriesHeaders(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})
	eqCtx := reqctx.WithRequestID(trace.ContextWithSpanContext(ctx, sc), "req-42")
	err := sdkpgx.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return s.EnqueueTx(eqCtx, tx, outbox.Row{EventID: "e1", AggregateType: "Agg", AggregateID: "agg-1", EventType: "Thing.Created", Payload: []byte("p")})
	})
	if err != nil {
		t.Fatal(err)
	}
	pub := &recorder{}
	relayer(s, pub, config.Outbox{BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 3}).RelayOnce(ctx) //nolint:errcheck // asserted via the recorder
	if len(pub.recs) != 1 {
		t.Fatalf("published %d records", len(pub.recs))
	}
	rec := pub.recs[0]
	want := map[string]string{
		"event_id":     "e1",
		"event_type":   "Thing.Created",
		"x-request-id": "req-42",
		"traceparent":  "00-0102030405060708090a0b0c0d0e0f10-0102030405060708-01",
	}
	for k, v := range want {
		if got := header(rec, k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
	if rec.Topic != "t" || string(rec.Key) != "agg-1" || string(rec.Value) != "p" {
		t.Errorf("record = topic %q key %q value %q", rec.Topic, rec.Key, rec.Value)
	}
}

func TestRowTopicOverridesRelayerTopic(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	err := sdkpgx.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return s.EnqueueTx(ctx, tx, outbox.Row{EventID: "e1", AggregateType: "A", AggregateID: "a", EventType: "T", Payload: []byte("p"), Topic: "other"})
	})
	if err != nil {
		t.Fatal(err)
	}
	enqueue(t, pool, s, "e2")
	pub := &recorder{}
	drain(t, relayer(s, pub, config.Outbox{BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 3}), 5)
	topics := map[string]string{}
	for _, rec := range pub.recs {
		topics[header(rec, "event_id")] = rec.Topic
	}
	if topics["e1"] != "other" || topics["e2"] != "t" {
		t.Fatalf("topics = %v", topics)
	}
}

func TestEnqueueRejectsEmptyEventID(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	err := sdkpgx.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return s.EnqueueTx(ctx, tx, outbox.Row{AggregateType: "A", AggregateID: "a", EventType: "T", Payload: []byte("p")})
	})
	if !errors.Is(err, outbox.ErrEmptyEventID) {
		t.Fatalf("err = %v, want ErrEmptyEventID", err)
	}
}

func TestRelayBatchBoundedUnderBlockingPublisher(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	enqueue(t, pool, s, "e1")
	enqueue(t, pool, s, "e2")
	enqueue(t, pool, s, "e3")
	const lock = 4
	r := relayer(s, &recorder{block: true}, config.Outbox{BatchSize: 10, ClaimLockSeconds: lock, MaxAttempts: 5})

	start := time.Now()
	if _, err := r.RelayOnce(ctx); err != nil {
		t.Fatalf("relay: %v", err)
	}
	if d := time.Since(start); d >= lock*time.Second {
		t.Fatalf("relay took %v, must end before the %ds claim lock", d, lock)
	}
	var locked int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE locked_until IS NOT NULL`).Scan(&locked); err != nil || locked != 0 {
		t.Fatalf("%d rows still leased after the batch (err %v)", locked, err)
	}
}

func TestPurgeDeletesOnlyOldPublishedRows(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	for _, id := range []string{"old", "new", "pending"} {
		enqueue(t, pool, s, id)
	}
	pub := &recorder{failIDs: map[string]bool{"pending": true}}
	drain(t, relayer(s, pub, config.Outbox{BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 5}), 5)
	if _, err := pool.Exec(ctx, `UPDATE `+table+` SET published_at = now() - interval '48 hours' WHERE event_id = 'old'`); err != nil {
		t.Fatal(err)
	}
	n, err := s.Purge(ctx, 24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("purged %d (err %v), want 1", n, err)
	}
	var left []string
	rows, err := pool.Query(ctx, `SELECT event_id FROM `+table+` ORDER BY event_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		left = append(left, id)
	}
	if !slices.Equal(left, []string{"new", "pending"}) {
		t.Fatalf("rows left = %v", left)
	}
}

func TestSchemaSQLUpgradesTableFromOldSchema(t *testing.T) {
	ctx := context.Background()
	pool, err := sdkpgx.Open(ctx, config.Postgres{URL: testkit.Postgres(t), MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	old := `CREATE TABLE ` + table + ` (
		event_id TEXT PRIMARY KEY, aggregate_type TEXT NOT NULL, aggregate_id TEXT NOT NULL,
		event_type TEXT NOT NULL, payload BYTEA NOT NULL, principal BYTEA,
		request_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending',
		attempts INT NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		locked_until TIMESTAMPTZ, published_at TIMESTAMPTZ, error TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now())`
	if _, err := pool.Exec(ctx, old); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO `+table+` (event_id, aggregate_type, aggregate_id, event_type, payload) VALUES ('legacy','A','a','T','p')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, outbox.SchemaSQL(table)); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	pub := &recorder{}
	drain(t, relayer(outbox.NewStore(pool, table), pub, config.Outbox{BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 3}), 3)
	if got := pub.eventIDs(); !slices.Equal(got, []string{"legacy"}) {
		t.Fatalf("published = %v, want the pre-upgrade row", got)
	}
}
