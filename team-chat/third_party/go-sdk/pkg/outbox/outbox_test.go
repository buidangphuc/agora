package outbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/trace"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
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
	block    bool          // block until the publish ctx ends, like a broker outage
	failErr  error         // returned for failing keys/ids; default is a transient error
	delay    time.Duration // each publish takes this long
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
	if r.delay > 0 {
		time.Sleep(r.delay)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failKeys[string(rec.Key)] || r.failIDs[header(rec, kafkax.HeaderEventID)] {
		if r.failErr != nil {
			return r.failErr
		}
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
		ids = append(ids, header(rec, kafkax.HeaderEventID))
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
	pool, err := postgres.Open(ctx, config.Postgres{URL: testkit.Postgres(t), MaxConns: 8})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	ddl, err := outbox.SchemaSQL(table)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, ddl); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return pool, mustStore(t, pool)
}

func mustStore(t *testing.T, pool *pgxpool.Pool) *outbox.Store {
	t.Helper()
	s, err := outbox.NewStore(pool, table)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustRelayer(t *testing.T, s *outbox.Store, p outbox.Publisher, o outbox.RelayerOptions) *outbox.Relayer {
	t.Helper()
	r, err := outbox.NewRelayer(s, p, o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func enqueue(t *testing.T, pool *pgxpool.Pool, s *outbox.Store, id string) {
	t.Helper()
	err := postgres.WithTx(context.Background(), pool, func(tx pgx.Tx) error {
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
	opts := outbox.RelayerOptions{Topic: "t", Config: config.Outbox{BatchSize: 10, ClaimLock: time.Minute}}
	a, b := mustRelayer(t, s, pub, opts), mustRelayer(t, s, pub, opts)

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

func TestTransientFailureBacksOffAndNeverParks(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	enqueue(t, pool, s, "bad")
	enqueue(t, pool, s, "good")
	pub := &recorder{failKeys: map[string]bool{"agg-bad": true}}
	r := mustRelayer(t, s, pub, outbox.RelayerOptions{
		Topic: "t", Config: config.Outbox{BatchSize: 10, ClaimLock: time.Minute},
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

	// A transient error keeps retrying however often: it never parks.
	// The delay is capped at MaxBackoff (1h) once the doubling reaches it.
	for want := 3; want <= 8; want++ {
		release()
		rep, _ := r.RelayOnce(ctx)
		if rep.Retried != 1 || rep.Parked != 0 {
			t.Fatalf("attempt %d = %+v, want a retry", want, rep)
		}
		st := read()
		if st.status != "pending" || st.attempts != want || st.delay > time.Hour+time.Second {
			t.Fatalf("after attempt %d: %+v", want, st)
		}
	}
	if st := read(); st.delay < 59*time.Minute {
		t.Fatalf("backoff not capped at MaxBackoff: %+v", st)
	}
	if len(pub.keys) != 1 || pub.keys[0] != "agg-good" {
		t.Fatalf("published = %v, want only agg-good", pub.keys)
	}
}

func TestNonRetriableErrorParksAndRequeueResumes(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	enqueueAgg(t, pool, s, "A", "a1", "a2")
	enqueueAgg(t, pool, s, "B", "b1")
	pub := &recorder{failErr: fmt.Errorf("kafkax: produce: %w", kerr.MessageTooLarge)}
	pub.setFailIDs("a1")
	r := relayer(t, s, pub, config.Outbox{BatchSize: 10, ClaimLock: time.Minute})

	rep, err := r.RelayOnce(ctx)
	if err != nil || rep.Parked != 1 || rep.Retried != 0 || rep.Published != 1 {
		t.Fatalf("pass 1 = %+v, %v; want a1 parked at once, b1 published", rep, err)
	}
	st, err := s.Stats(ctx)
	if err != nil || st != (outbox.Stats{Pending: 1, Parked: 1}) || st.Blocked() != 1 {
		t.Fatalf("stats = %+v (err %v)", st, err)
	}
	// a2 must not pass the parked a1, however often we poll.
	if rep, _ := r.RelayOnce(ctx); rep.Claimed != 0 {
		t.Fatalf("parked aggregate was claimed: %+v", rep)
	}

	pub.setFailIDs()
	if err := s.Requeue(ctx, "a1"); err != nil {
		t.Fatal(err)
	}
	var status string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT status, attempts FROM `+table+` WHERE event_id = 'a1'`).Scan(&status, &attempts); err != nil || status != "pending" || attempts != 0 {
		t.Fatalf("after Requeue: %s %d (err %v)", status, attempts, err)
	}
	drain(t, r, 5)
	if got := pub.eventIDs(); !slices.Equal(got, []string{"b1", "a1", "a2"}) {
		t.Fatalf("published = %v, want [b1 a1 a2]", got)
	}
	if err := s.Requeue(ctx, "a1"); !errors.Is(err, outbox.ErrNotFound) {
		t.Fatalf("Requeue of a published row = %v, want ErrNotFound", err)
	}
	if err := s.Requeue(ctx, "nope"); !errors.Is(err, outbox.ErrNotFound) {
		t.Fatalf("Requeue of a missing row = %v, want ErrNotFound", err)
	}
}

func TestPublishErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		park bool
	}{
		{"record too large", kerr.MessageTooLarge, true},
		{"record list too large", kerr.RecordListTooLarge, true},
		{"corrupt message", kerr.CorruptMessage, true},
		{"invalid record", kerr.InvalidRecord, true},
		{"invalid timestamp", kerr.InvalidTimestamp, true},
		{"wrapped record error", fmt.Errorf("kafkax: produce: %w", kerr.MessageTooLarge), true},
		{"custom permanent", outbox.Permanent(errors.New("schema rejected")), true},
		{"topic authorization", kerr.TopicAuthorizationFailed, false},
		{"cluster authorization", kerr.ClusterAuthorizationFailed, false},
		{"unknown topic", fmt.Errorf("produce: %w", kerr.UnknownTopicOrPartition), false},
		{"invalid topic", kerr.InvalidTopicException, false},
		{"leader moved", kerr.NotLeaderForPartition, false},
		{"request timed out", kerr.RequestTimedOut, false},
		{"client record timeout", kgo.ErrRecordTimeout, false},
		{"plain error", errors.New("connection reset"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, s := newStore(t)
			enqueueAgg(t, pool, s, "A", "a1")
			pub := &recorder{failErr: tc.err}
			pub.setFailIDs("a1")
			rep, err := relayer(t, s, pub, config.Outbox{BatchSize: 10, ClaimLock: time.Minute}).RelayOnce(context.Background())
			if err != nil || (rep.Parked == 1) != tc.park || (rep.Retried == 1) == tc.park {
				t.Fatalf("report = %+v (err %v), want park=%v", rep, err, tc.park)
			}
		})
	}
	if outbox.Permanent(nil) != nil {
		t.Fatal("Permanent(nil) must be nil")
	}
	cause := errors.New("x")
	if !errors.Is(outbox.Permanent(cause), cause) {
		t.Fatal("Permanent must keep the cause reachable")
	}
}

func TestRepeatedTransientFailureLogsErrorAfterAlertAfter(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	enqueueAgg(t, pool, s, "A", "a1")
	pub := &recorder{}
	pub.setFailIDs("a1")
	var logs bytes.Buffer
	r := mustRelayer(t, s, pub, outbox.RelayerOptions{
		Topic: "t", Config: config.Outbox{BatchSize: 10, ClaimLock: time.Minute},
		BaseBackoff: time.Minute, MaxBackoff: time.Hour, AlertAfter: 2,
		Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := pool.Exec(ctx, `UPDATE `+table+` SET available_at = now()`); err != nil {
			t.Fatal(err)
		}
		if rep, err := r.RelayOnce(ctx); err != nil || rep.Retried != 1 {
			t.Fatalf("attempt %d = %+v, %v", attempt, rep, err)
		}
		hasErr := strings.Contains(logs.String(), "level=ERROR")
		if want := attempt > 2; hasErr != want {
			t.Fatalf("after attempt %d: ERROR logged = %v, want %v\n%s", attempt, hasErr, want, logs.String())
		}
	}
	if !strings.Contains(logs.String(), "a1") {
		t.Fatalf("ERROR log does not name the event: %s", logs.String())
	}
}

func TestRequeueParkedFiltersByTopicAndError(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	insert := func(id, agg, topic, status, errText string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO `+table+` (event_id, aggregate_type, aggregate_id, event_type, payload, topic, status, attempts, error)
			VALUES ($1::text, 'Agg', $2, 'T', convert_to($1::text, 'UTF8'), NULLIF($3, ''), $4, 3, $5)`, id, agg, topic, status, errText); err != nil {
			t.Fatal(err)
		}
	}
	insert("p1", "A", "orders", "failed", "kafka: message too large")
	insert("p2", "B", "orders", "failed", "kafka: corrupt message")
	insert("p3", "C", "billing", "failed", "kafka: message too large")
	insert("p4", "D", "", "failed", "kafka: message too large")
	insert("done", "E", "orders", "published", "")
	insert("wait", "F", "orders", "pending", "retrying")

	state := func() map[string]string {
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT event_id, status || ':' || attempts FROM `+table)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]string{}
		for rows.Next() {
			var id, st string
			if err := rows.Scan(&id, &st); err != nil {
				t.Fatal(err)
			}
			out[id] = st
		}
		return out
	}
	n, err := s.RequeueParked(ctx, outbox.RequeueFilter{Topic: "orders", ErrorContains: "too large"})
	if err != nil || n != 1 {
		t.Fatalf("topic+error requeue = %d, %v; want 1 (p1)", n, err)
	}
	if st := state(); st["p1"] != "pending:0" || st["p2"] != "failed:3" || st["p3"] != "failed:3" {
		t.Fatalf("state = %v", st)
	}
	if n, err = s.RequeueParked(ctx, outbox.RequeueFilter{ErrorContains: "too large"}); err != nil || n != 2 {
		t.Fatalf("error-only requeue = %d, %v; want 2 (p3, p4)", n, err)
	}
	if n, err = s.RequeueParked(ctx, outbox.RequeueFilter{}); err != nil || n != 1 {
		t.Fatalf("empty filter = %d, %v; want the last parked row (p2)", n, err)
	}
	st := state()
	if st["done"] != "published:3" || st["wait"] != "pending:3" || st["p2"] != "pending:0" {
		t.Fatalf("final state = %v; published and pending rows must be untouched", st)
	}
	if n, err = s.RequeueParked(ctx, outbox.RequeueFilter{}); err != nil || n != 0 {
		t.Fatalf("nothing parked = %d, %v", n, err)
	}
}

func TestHeadScanCursorServesEveryAggregate(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	const aggs, batch, depth = 10, 3, 6
	for i := range aggs {
		ids := make([]string, depth)
		for j := range ids {
			ids[j] = fmt.Sprintf("e%02d-%d", i, j)
		}
		enqueueAgg(t, pool, s, fmt.Sprintf("a%02d", i), ids...)
	}
	pub := &recorder{}
	r := relayer(t, s, pub, config.Outbox{BatchSize: batch, ClaimLock: time.Minute})
	for range (aggs+batch-1)/batch + 1 {
		if _, err := r.RelayOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	served := map[string]bool{}
	for _, k := range pub.keys {
		served[k] = true
	}
	if len(served) != aggs {
		t.Fatalf("%d of %d aggregates published within %d passes: %v", len(served), aggs, (aggs+batch-1)/batch+1, pub.keys)
	}
}

func TestEnqueueRollsBackWithTx(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	boom := errors.New("boom")
	err := postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
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

func TestConstructorsRejectBadInput(t *testing.T) {
	for _, name := range []string{"", "Outbox", "a;drop table x", "1abc", "a-b", strings.Repeat("a", 42)} {
		if _, err := outbox.NewStore(nil, name); err == nil {
			t.Errorf("NewStore(%q) accepted the name", name)
		}
		if _, err := outbox.SchemaSQL(name); err == nil {
			t.Errorf("SchemaSQL(%q) accepted the name", name)
		}
	}
	if _, err := outbox.NewStore(nil, table); err == nil {
		t.Error("NewStore accepted a nil pool")
	}

	pool, s := newStore(t)
	pub := &recorder{}
	good := config.Outbox{BatchSize: 1, ClaimLock: time.Minute}
	for name, tc := range map[string]struct {
		s *outbox.Store
		p outbox.Publisher
		o outbox.RelayerOptions
	}{
		"nil store":         {nil, pub, outbox.RelayerOptions{Config: good}},
		"nil publisher":     {s, nil, outbox.RelayerOptions{Config: good}},
		"negative batch":    {s, pub, outbox.RelayerOptions{Config: config.Outbox{BatchSize: -1}}},
		"negative lock":     {s, pub, outbox.RelayerOptions{Config: config.Outbox{ClaimLock: -time.Second}}},
		"negative backoff":  {s, pub, outbox.RelayerOptions{BaseBackoff: -time.Second}},
		"max below base":    {s, pub, outbox.RelayerOptions{BaseBackoff: time.Minute, MaxBackoff: time.Second}},
		"negative mark tmo": {s, pub, outbox.RelayerOptions{MarkTimeout: -time.Second}},
		"negative alert":    {s, pub, outbox.RelayerOptions{AlertAfter: -1}},
		// Three marks (published, failed, released) must fit well inside the lease.
		"marks outlast lease": {s, pub, outbox.RelayerOptions{Config: config.Outbox{ClaimLock: 10 * time.Second}, MarkTimeout: 2 * time.Second}},
	} {
		if _, err := outbox.NewRelayer(tc.s, tc.p, tc.o); err == nil {
			t.Errorf("NewRelayer(%s) did not fail", name)
		}
	}
	if _, err := outbox.NewRelayer(s, pub, outbox.RelayerOptions{}); err != nil {
		t.Errorf("zero options must take defaults: %v", err)
	}
	for _, lock := range []time.Duration{time.Second, 5 * time.Second, time.Minute, time.Hour} {
		if _, err := outbox.NewRelayer(s, pub, outbox.RelayerOptions{Config: config.Outbox{ClaimLock: lock}}); err != nil {
			t.Errorf("default MarkTimeout must fit ClaimLock %v: %v", lock, err)
		}
	}
	_ = pool
}

func TestRowWithoutTopicIsNotAnAttempt(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	enqueueAgg(t, pool, s, "A", "a1", "a2")
	enqueueAgg(t, pool, s, "B", "b1")
	pub := &recorder{}
	var logs bytes.Buffer
	r := mustRelayer(t, s, pub, outbox.RelayerOptions{
		Config: config.Outbox{BatchSize: 10, ClaimLock: time.Minute},
		Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	// B names its own topic; A has none and the relayer has no default.
	if _, err := pool.Exec(ctx, `UPDATE `+table+` SET topic = 'own' WHERE aggregate_id = 'B'`); err != nil {
		t.Fatal(err)
	}
	rep, err := r.RelayOnce(ctx)
	if err != nil || rep.Published != 1 || rep.Retried != 0 || rep.Parked != 0 {
		t.Fatalf("pass = %+v, %v", rep, err)
	}
	var attempts, leased int
	if err := pool.QueryRow(ctx, `SELECT coalesce(sum(attempts), 0), count(locked_until) FROM `+table).Scan(&attempts, &leased); err != nil || attempts != 0 || leased != 0 {
		t.Fatalf("attempts = %d, leased = %d (err %v); want neither", attempts, leased, err)
	}
	if !strings.Contains(logs.String(), "level=ERROR") || !strings.Contains(logs.String(), "no topic") {
		t.Fatalf("no ERROR log for the missing topic: %s", logs.String())
	}
	if got := pub.eventIDs(); !slices.Equal(got, []string{"b1"}) {
		t.Fatalf("published = %v, want [b1]", got)
	}
	// The misconfigured row is pushed back, not re-claimed (and re-logged) every pass.
	if rep, err := r.RelayOnce(ctx); err != nil || rep.Claimed != 0 {
		t.Fatalf("second pass = %+v, %v; want the topic-less row left alone", rep, err)
	}
	if n := strings.Count(logs.String(), "level=ERROR"); n != 1 {
		t.Fatalf("%d ERROR logs after two passes, want 1: %s", n, logs.String())
	}
}

func TestEnqueueDuplicateIsNoop(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	row := outbox.Row{EventID: "dup", AggregateType: "Agg", AggregateID: "agg-dup", EventType: "T", Payload: []byte("p")}
	err := postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
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
	err := postgres.WithTx(context.Background(), pool, func(tx pgx.Tx) error {
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

func relayer(t *testing.T, s *outbox.Store, p outbox.Publisher, cfg config.Outbox) *outbox.Relayer {
	t.Helper()
	return mustRelayer(t, s, p, outbox.RelayerOptions{Topic: "t", Config: cfg, BaseBackoff: time.Minute, MaxBackoff: time.Hour})
}

func TestOrderPerAggregate(t *testing.T) {
	pool, s := newStore(t)
	// Events of one transaction share created_at; order must still hold.
	enqueueAgg(t, pool, s, "A", "a1", "a2", "a3", "a4", "a5")
	enqueueAgg(t, pool, s, "B", "b1", "b2")
	pub := &recorder{}
	drain(t, relayer(t, s, pub, config.Outbox{BatchSize: 3, ClaimLock: time.Minute}), 20)

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
	r := relayer(t, s, pub, config.Outbox{BatchSize: 10, ClaimLock: time.Minute})

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

func TestPublishedRecordCarriesHeaders(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})
	eqCtx := reqctx.WithRequestID(trace.ContextWithSpanContext(ctx, sc), "req-42")
	err := postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return s.EnqueueTx(eqCtx, tx, outbox.Row{EventID: "e1", AggregateType: "Agg", AggregateID: "agg-1", EventType: "Thing.Created", Payload: []byte("p")})
	})
	if err != nil {
		t.Fatal(err)
	}
	pub := &recorder{}
	relayer(t, s, pub, config.Outbox{BatchSize: 10, ClaimLock: time.Minute}).RelayOnce(ctx) //nolint:errcheck // asserted via the recorder
	if len(pub.recs) != 1 {
		t.Fatalf("published %d records", len(pub.recs))
	}
	rec := pub.recs[0]
	want := map[string]string{
		"event-id":     "e1",
		"event-type":   "Thing.Created",
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
	err := postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return s.EnqueueTx(ctx, tx, outbox.Row{EventID: "e1", AggregateType: "A", AggregateID: "a", EventType: "T", Payload: []byte("p"), Topic: "other"})
	})
	if err != nil {
		t.Fatal(err)
	}
	enqueue(t, pool, s, "e2")
	pub := &recorder{}
	drain(t, relayer(t, s, pub, config.Outbox{BatchSize: 10, ClaimLock: time.Minute}), 5)
	topics := map[string]string{}
	for _, rec := range pub.recs {
		topics[header(rec, kafkax.HeaderEventID)] = rec.Topic
	}
	if topics["e1"] != "other" || topics["e2"] != "t" {
		t.Fatalf("topics = %v", topics)
	}
}

func TestEnqueueRejectsEmptyEventID(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	err := postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
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
	r := relayer(t, s, &recorder{block: true}, config.Outbox{BatchSize: 10, ClaimLock: lock * time.Second})

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
	// The budget ran out, the broker did not fail: no attempt is counted and no
	// backoff is set, so the next pass takes the rows again at once.
	var attempts, delayed int
	if err := pool.QueryRow(ctx, `SELECT coalesce(sum(attempts), 0), count(*) FILTER (WHERE available_at > now() + interval '1 second') FROM `+table).Scan(&attempts, &delayed); err != nil || attempts != 0 || delayed != 0 {
		t.Fatalf("attempts = %d, delayed rows = %d (err %v); an expired budget must count neither", attempts, delayed, err)
	}
}

func TestPurgeDeletesOnlyOldPublishedRows(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	for _, id := range []string{"old", "new", "pending"} {
		enqueue(t, pool, s, id)
	}
	pub := &recorder{failIDs: map[string]bool{"pending": true}}
	drain(t, relayer(t, s, pub, config.Outbox{BatchSize: 10, ClaimLock: time.Minute}), 5)
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

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := postgres.Open(context.Background(), config.Postgres{URL: testkit.Postgres(t), MaxConns: 8})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// v0Table creates a table as the v0 SchemaSQL made it and returns a pool on it.
func v0Table(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := openPool(t)
	for _, q := range []string{
		`CREATE TABLE ` + table + ` (
		event_id TEXT PRIMARY KEY, aggregate_type TEXT NOT NULL, aggregate_id TEXT NOT NULL,
		event_type TEXT NOT NULL, payload BYTEA NOT NULL, principal BYTEA,
		request_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending',
		attempts INT NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		locked_until TIMESTAMPTZ, published_at TIMESTAMPTZ, error TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`CREATE INDEX ` + table + `_claim_idx ON ` + table + ` (available_at, created_at) WHERE status = 'pending'`,
		`CREATE INDEX ` + table + `_status_idx ON ` + table + ` (status)`,
	} {
		if _, err := pool.Exec(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

// v0Insert adds a row the way a v0 writer does: no seq, created hoursAgo hours ago.
func v0Insert(t *testing.T, pool *pgxpool.Pool, agg, id string, hoursAgo int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO `+table+` (event_id, aggregate_type, aggregate_id, event_type, payload, created_at)
		VALUES ($1::text, 'Agg', $2, 'T', convert_to($1::text, 'UTF8'), now() - ($3 * interval '1 hour'))`, id, agg, hoursAgo); err != nil {
		t.Fatal(err)
	}
}

// backfillUntilDone runs the Backfill statement until it reports 0 rows. The
// first result column is the number of rows updated.
func backfillUntilDone(t *testing.T, pool *pgxpool.Pool, q string) {
	t.Helper()
	for range 20 {
		rows, err := pool.Query(context.Background(), q)
		if err != nil {
			t.Errorf("backfill: %v", err)
			return
		}
		var updated int64
		if rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				rows.Close()
				t.Errorf("backfill: %v", err)
				return
			}
			updated, _ = vals[0].(int64)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Errorf("backfill: %v", err)
			return
		}
		if updated == 0 {
			return
		}
	}
	t.Error("backfill did not finish")
}

// runEach runs each statement on its own, as Prepare and Finish require.
func runEach(t *testing.T, pool *pgxpool.Pool, stmts []string) {
	t.Helper()
	for _, q := range stmts {
		if _, err := pool.Exec(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

// inOneTx runs statements in a single transaction, as Upgrade.Switch requires.
func inOneTx(t *testing.T, pool *pgxpool.Pool, stmts []string) {
	t.Helper()
	err := postgres.WithTx(context.Background(), pool, func(tx pgx.Tx) error {
		for _, q := range stmts {
			if _, err := tx.Exec(context.Background(), q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func seqOrder(t *testing.T, pool *pgxpool.Pool) (order []string, distinct, nulls int) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `SELECT count(DISTINCT seq), count(*) FILTER (WHERE seq IS NULL) FROM `+table).Scan(&distinct, &nulls); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `SELECT event_id FROM `+table+` ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		order = append(order, id)
	}
	return order, distinct, nulls
}

// TestUpgradeSQLKeepsOrderOfAV0Table applies the documented v0 to v1 migration
// to a table made by the v0 SchemaSQL, with a retried row whose heap position
// no longer matches its age, and rows a v0 writer inserted between the last
// Backfill and Switch. Every row is numbered in creation order, new rows go after
// them, and the relayer publishes in that order.
func TestUpgradeSQLKeepsOrderOfAV0Table(t *testing.T) {
	ctx := context.Background()
	pool := v0Table(t)
	// a1 is the oldest. A v0 retry rewrote it, which moves it to the end of the
	// heap; BIGSERIAL numbering in heap order would put it last.
	for i, id := range []string{"a1", "a2", "a3", "a4"} {
		v0Insert(t, pool, "a", id, 20-i)
	}
	if _, err := pool.Exec(ctx, `UPDATE `+table+` SET attempts = 1, error = 'retried' WHERE event_id = 'a1'`); err != nil {
		t.Fatal(err)
	}

	// The v1 schema statements refuse a table without seq instead of rewriting it.
	ddl, err := outbox.SchemaSQL(table)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, ddl); err == nil {
		t.Fatal("SchemaSQL applied to a v0 table; it must fail until the table is migrated")
	}

	up, err := outbox.UpgradeSQL(table)
	if err != nil {
		t.Fatal(err)
	}
	runEach(t, pool, up.Prepare)
	backfillUntilDone(t, pool, up.Backfill)
	// A v0 writer that is still running inserts rows without a seq; l2 is
	// inserted first but is the newer of the two.
	v0Insert(t, pool, "a", "l2", 4)
	v0Insert(t, pool, "a", "l1", 5)
	inOneTx(t, pool, up.Switch)
	if order, _, nulls := seqOrder(t, pool); nulls != 0 {
		t.Fatalf("%d rows without seq after Switch (order %v)", nulls, order)
	}
	// Rows written after Switch, by a v1 writer, take later numbers.
	s := mustStore(t, pool)
	enqueueAgg(t, pool, s, "a", "n1", "n2")
	runEach(t, pool, up.Finish)

	want := []string{"a1", "a2", "a3", "a4", "l1", "l2", "n1", "n2"}
	order, distinct, nulls := seqOrder(t, pool)
	if !slices.Equal(order, want) || distinct != len(want) || nulls != 0 {
		t.Fatalf("order by seq = %v (distinct %d, null %d), want %v with unique numbers", order, distinct, nulls, want)
	}
	var notNull bool
	var claimIdx int
	if err := pool.QueryRow(ctx, `SELECT (SELECT is_nullable = 'NO' FROM information_schema.columns WHERE table_name = $1 AND column_name = 'seq'),
		(SELECT count(*) FROM pg_indexes WHERE indexname = $2)`, table, table+"_claim_idx").Scan(&notNull, &claimIdx); err != nil || !notNull || claimIdx != 0 {
		t.Fatalf("seq not null = %v, claim_idx left = %d (err %v)", notNull, claimIdx, err)
	}
	// The migrated table is a v1 table: SchemaSQL is a no-op and the relayer
	// publishes the rows in order.
	if _, err := pool.Exec(ctx, ddl); err != nil {
		t.Fatal(err)
	}
	pub := &recorder{}
	drain(t, relayer(t, s, pub, config.Outbox{BatchSize: 3, ClaimLock: time.Minute}), 10)
	if got := pub.eventIDs(); !slices.Equal(got, want) {
		t.Fatalf("published = %v, want %v", got, want)
	}
}

// TestConcurrentBackfillsNeverDuplicate runs four Backfill loops at once on a
// table larger than one batch whose rows are in the reverse of creation order on
// disk, then Switch on rows that arrived meanwhile: numbers are unique and follow
// creation order.
func TestConcurrentBackfillsNeverDuplicate(t *testing.T) {
	ctx := context.Background()
	pool := v0Table(t)
	const first, later = 25000, 3000
	if _, err := pool.Exec(ctx, `INSERT INTO `+table+` (event_id, aggregate_type, aggregate_id, event_type, payload, created_at)
		SELECT 'e' || lpad(g::text, 6, '0'), 'Agg', 'a', 'T', 'x', now() - (g * interval '1 minute') FROM generate_series(1, $1) g`, first); err != nil {
		t.Fatal(err)
	}
	up, err := outbox.UpgradeSQL(table)
	if err != nil {
		t.Fatal(err)
	}
	runEach(t, pool, up.Prepare)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			backfillUntilDone(t, pool, up.Backfill)
		}()
	}
	close(start)
	wg.Wait()
	// created_at falls as g rises, so creation order is descending g.
	check := func(total int, want func(i int) string) {
		t.Helper()
		order, distinct, nulls := seqOrder(t, pool)
		if len(order) != total || distinct != total || nulls != 0 {
			t.Fatalf("rows = %d, distinct seq = %d, null = %d; want %d, %d, 0", len(order), distinct, nulls, total, total)
		}
		for i, id := range order {
			if id != want(i) {
				t.Fatalf("position %d = %s, want %s", i, id, want(i))
			}
		}
	}
	check(first, func(i int) string { return fmt.Sprintf("e%06d", first-i) })

	if _, err := pool.Exec(ctx, `INSERT INTO `+table+` (event_id, aggregate_type, aggregate_id, event_type, payload, created_at)
		SELECT 'l' || lpad(g::text, 6, '0'), 'Agg', 'a', 'T', 'x', now() - (g * interval '1 second') FROM generate_series(1, $1) g`, later); err != nil {
		t.Fatal(err)
	}
	inOneTx(t, pool, up.Switch)
	check(first+later, func(i int) string {
		if i < first {
			return fmt.Sprintf("e%06d", first-i)
		}
		return fmt.Sprintf("l%06d", later-(i-first))
	})
}

// TestUpgradeFinishCanBeRerun re-runs Finish after an interrupted run left its
// temporary constraint behind.
func TestUpgradeFinishCanBeRerun(t *testing.T) {
	ctx := context.Background()
	pool := v0Table(t)
	v0Insert(t, pool, "a", "a1", 2)
	up, err := outbox.UpgradeSQL(table)
	if err != nil {
		t.Fatal(err)
	}
	runEach(t, pool, up.Prepare)
	backfillUntilDone(t, pool, up.Backfill)
	inOneTx(t, pool, up.Switch)
	if _, err := pool.Exec(ctx, `ALTER TABLE `+table+` ADD CONSTRAINT `+table+`_seq_nn CHECK (seq IS NOT NULL) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		for _, q := range up.Finish {
			if _, err := pool.Exec(ctx, q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	}
}

func TestMarksGetTheirOwnTimeout(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	enqueueAgg(t, pool, s, "A", "a1", "a2", "a3", "a4")
	// The run takes 4 x 250ms = 1s to publish, longer than one MarkTimeout (400ms)
	// but the claim lock (one minute) is far away. A single timeout started
	// before the loop would have expired for the marks of the later rows.
	pub := &recorder{delay: 250 * time.Millisecond}
	r := mustRelayer(t, s, pub, outbox.RelayerOptions{
		Topic: "t", Config: config.Outbox{BatchSize: 10, ClaimLock: time.Minute}, MarkTimeout: 400 * time.Millisecond,
	})
	rep, err := r.RelayOnce(ctx)
	if err != nil || rep.Published != 4 {
		t.Fatalf("pass = %+v, %v; want 4 published", rep, err)
	}
	var published int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE status = 'published'`).Scan(&published); err != nil || published != 4 {
		t.Fatalf("%d rows marked published (err %v), want 4", published, err)
	}
}

func TestBacklogOfOneAggregateDrainsInBatchSizedPasses(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	const n, batch = 50, 10
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("e%03d", i)
	}
	enqueueAgg(t, pool, s, "A", ids...)
	pub := &recorder{}
	r := relayer(t, s, pub, config.Outbox{BatchSize: batch, ClaimLock: time.Minute})
	passes := 0
	for {
		rep, err := r.RelayOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Published == 0 {
			break
		}
		if rep.Claimed > batch {
			t.Fatalf("pass claimed %d rows, batch is %d", rep.Claimed, batch)
		}
		passes++
	}
	if passes != n/batch {
		t.Fatalf("drained in %d passes, want %d", passes, n/batch)
	}
	if got := pub.eventIDs(); !slices.Equal(got, ids) {
		t.Fatalf("order broken: %v", got)
	}
}

func sameSet(got []string, want ...string) bool {
	got = slices.Clone(got)
	slices.Sort(got)
	slices.Sort(want)
	return slices.Equal(got, want)
}

func TestFailureStopsTheRunAndReleasesTheRest(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	enqueueAgg(t, pool, s, "A", "a1", "a2", "a3", "a4")
	enqueueAgg(t, pool, s, "B", "b1", "b2")
	pub := &recorder{}
	pub.setFailIDs("a2")
	r := relayer(t, s, pub, config.Outbox{BatchSize: 10, ClaimLock: time.Minute})
	rep, err := r.RelayOnce(ctx)
	if err != nil || rep != (outbox.Report{Claimed: 6, Published: 3, Retried: 1}) {
		t.Fatalf("pass = %+v, %v", rep, err)
	}
	if got := pub.eventIDs(); !sameSet(got, "a1", "b1", "b2") {
		t.Fatalf("published = %v; a3 and a4 must not pass a2", got)
	}
	var leased int
	if err := pool.QueryRow(ctx, `SELECT count(locked_until) FROM `+table).Scan(&leased); err != nil || leased != 0 {
		t.Fatalf("%d rows still leased (err %v); the unattempted rest must be released", leased, err)
	}
}

func TestConcurrentRelayersKeepAggregateOrder(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	const aggs, per = 30, 10
	for a := range aggs {
		ids := make([]string, per)
		for i := range ids {
			ids[i] = fmt.Sprintf("g%02d-%02d", a, i)
		}
		enqueueAgg(t, pool, s, fmt.Sprintf("g%02d", a), ids...)
	}
	pub := &recorder{}
	opts := outbox.RelayerOptions{Topic: "t", Config: config.Outbox{BatchSize: 25, ClaimLock: time.Minute}}
	var wg sync.WaitGroup
	for range 3 {
		r := mustRelayer(t, s, pub, opts)
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
	ids := pub.eventIDs()
	if len(ids) != aggs*per {
		t.Fatalf("published %d events, want %d", len(ids), aggs*per)
	}
	last := map[string]string{}
	for _, id := range ids {
		agg := id[:3]
		if prev := last[agg]; prev >= id {
			t.Fatalf("%s published after %s", id, prev)
		}
		last[agg] = id
	}
}

// explain runs EXPLAIN (ANALYZE, BUFFERS) on the claim's head scan and returns
// the rows its index scans produced and the buffers it touched.
func explain(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) (indexRows, buffers float64, plan string) {
	t.Helper()
	var raw []byte
	err := pool.QueryRow(context.Background(), `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) `+sql, args...).Scan(&raw)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	var top []map[string]any
	if err := json.Unmarshal(raw, &top); err != nil || len(top) == 0 {
		t.Fatalf("plan json: %v", err)
	}
	var walk func(n map[string]any)
	walk = func(n map[string]any) {
		kind, _ := n["Node Type"].(string)
		if strings.Contains(kind, "Index") {
			rows, _ := n["Actual Rows"].(float64)
			loops, _ := n["Actual Loops"].(float64)
			indexRows += rows * loops
			if n["Index Name"] != table+"_agg_idx" {
				t.Errorf("head scan used %v, want %s_agg_idx", n["Index Name"], table)
			}
		}
		if kind == "Seq Scan" || kind == "Bitmap Heap Scan" {
			t.Errorf("head scan fell back to %s", kind)
		}
		if _, ok := n["Subplan Name"]; ok && strings.Contains(fmt.Sprint(n["Subplan Name"]), "SubPlan") {
			t.Errorf("per-row subplan in the head scan: %v", n["Subplan Name"])
		}
		kids, _ := n["Plans"].([]any)
		for _, k := range kids {
			walk(k.(map[string]any))
		}
	}
	root := top[0]["Plan"].(map[string]any)
	walk(root)
	buffers, _ = root["Shared Hit Blocks"].(float64)
	read, _ := root["Shared Read Blocks"].(float64)
	return indexRows, buffers + read, string(raw)
}

// TestParkedBacklogDoesNotSlowOtherAggregates puts a large backlog behind a
// parked row and shows the head scan does not look at it: the rows its index
// scans return depend on the number of aggregates, not on the backlog, and the
// other aggregates still go out.
func TestParkedBacklogDoesNotSlowOtherAggregates(t *testing.T) {
	ctx := context.Background()
	pool, s := newStore(t)
	const others = 20
	insertBacklog := func(n int) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO `+table+` (event_id, aggregate_type, aggregate_id, event_type, payload)
			SELECT 'p-' || g || '-' || clock_timestamp(), 'Agg', 'parked', 'T', 'x' FROM generate_series(1, $1) g`, n); err != nil {
			t.Fatal(err)
		}
	}
	// The parked head is the oldest row of its aggregate.
	if _, err := pool.Exec(ctx, `INSERT INTO `+table+` (event_id, aggregate_type, aggregate_id, event_type, payload, status, attempts, error)
		VALUES ('p-head', 'Agg', 'parked', 'T', 'x', 'failed', 1, 'too large')`); err != nil {
		t.Fatal(err)
	}
	insertBacklog(1000)
	for i := range others {
		enqueue(t, pool, s, fmt.Sprintf("o%02d", i))
	}
	if _, err := pool.Exec(ctx, `ANALYZE `+table); err != nil {
		t.Fatal(err)
	}
	smallRows, smallBuf, plan := explain(t, pool, outbox.HeadsSQL(table), 100)

	insertBacklog(30000)
	if _, err := pool.Exec(ctx, `ANALYZE `+table); err != nil {
		t.Fatal(err)
	}
	bigRows, bigBuf, _ := explain(t, pool, outbox.HeadsSQL(table), 100)
	t.Logf("head scan, 1k backlog: %v index rows, %v buffers; 31k backlog: %v index rows, %v buffers\n%s", smallRows, smallBuf, bigRows, bigBuf, plan)

	if want := float64(others + 1 + 1); bigRows != smallRows || bigRows > want {
		t.Fatalf("index rows = %v (1k) and %v (31k); want equal and at most %v (aggregates + 1)", smallRows, bigRows, want)
	}
	if bigBuf > smallBuf*1.5+20 {
		t.Fatalf("buffers grew with the backlog: %v -> %v", smallBuf, bigBuf)
	}

	// The cursor scans, which a relayer runs after its first pass, read no more.
	for name, q := range map[string]string{"after": outbox.HeadsAfterSQL(table), "through": outbox.HeadsThroughSQL(table)} {
		rows, buf, _ := explain(t, pool, q, 100, "Agg", "agg-o10")
		if rows > bigRows || buf > smallBuf*1.5+20 {
			t.Fatalf("%s scan read %v index rows and %v buffers with a 31k backlog (full scan: %v, %v)", name, rows, buf, bigRows, smallBuf)
		}
	}

	pub := &recorder{}
	rep, err := relayer(t, s, pub, config.Outbox{BatchSize: 100, ClaimLock: time.Minute}).RelayOnce(ctx)
	if err != nil || rep.Published != others || rep.Claimed != others {
		t.Fatalf("pass = %+v, %v; want %d published and nothing claimed from the parked aggregate", rep, err, others)
	}
	st, err := s.Stats(ctx)
	if err != nil || st.Parked != 1 || st.Pending != 31000 {
		t.Fatalf("stats = %+v (err %v)", st, err)
	}
}
