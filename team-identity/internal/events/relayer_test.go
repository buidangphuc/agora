package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/migrations"
)

// fakePublisher is an outbox.Publisher whose result is configurable: err for
// every record, or errFor[key] for the records of one aggregate.
type fakePublisher struct {
	err    error
	errFor map[string]error
	calls  int
}

func (p *fakePublisher) PublishRecord(_ context.Context, r *kgo.Record) error {
	p.calls++
	if err, ok := p.errFor[string(r.Key)]; ok {
		return err
	}
	return p.err
}

// outboxFixture is a disposable Postgres with the identity migrations applied,
// the SDK store over identity_outbox_events and the SDK relayer on top of it.
type outboxFixture struct {
	pool  *pgxpool.Pool
	store *outbox.Store
}

func newOutboxFixture(t *testing.T) *outboxFixture {
	t.Helper()
	ctx := context.Background()
	url := testkit.Postgres(t)
	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	st, err := outbox.NewStore(pool, store.OutboxTable)
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	return &outboxFixture{pool: pool, store: st}
}

func (f *outboxFixture) enqueue(t *testing.T, row outbox.Row) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.EnqueueTx(ctx, tx, row); err != nil {
		t.Fatalf("enqueue %s: %v", row.EventID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func (f *outboxFixture) relayer(t *testing.T, pub outbox.Publisher) *outbox.Relayer {
	t.Helper()
	r, err := outbox.NewRelayer(f.store, pub, outbox.RelayerOptions{
		Topic:  "identity.events",
		Config: config.Outbox{BatchSize: 10, ClaimLock: time.Minute},
		// BaseBackoff 1s and MaxBackoff 1m, as the old relayer test used.
		BaseBackoff: time.Second,
		MaxBackoff:  time.Minute,
	})
	if err != nil {
		t.Fatalf("outbox relayer: %v", err)
	}
	return r
}

type rowState struct {
	status    string
	attempts  int
	available time.Time
}

func (f *outboxFixture) state(t *testing.T, id string) rowState {
	t.Helper()
	var s rowState
	err := f.pool.QueryRow(context.Background(),
		`SELECT status, attempts, available_at FROM `+store.OutboxTable+` WHERE event_id = $1`, id).
		Scan(&s.status, &s.attempts, &s.available)
	if err != nil {
		t.Fatalf("read outbox row %s: %v", id, err)
	}
	return s
}

func TestRelayer_MarksPublishedOnSuccess(t *testing.T) {
	f := newOutboxFixture(t)
	f.enqueue(t, outbox.Row{EventID: "e1", AggregateType: "Session", AggregateID: "l1", EventType: "T", Payload: []byte("env")})
	prod := &fakePublisher{}

	rep, err := f.relayer(t, prod).RelayOnce(context.Background())
	if err != nil {
		t.Fatalf("RelayOnce: %v", err)
	}
	if rep.Claimed != 1 || rep.Published != 1 || rep.Retried != 0 || rep.Parked != 0 {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if prod.calls != 1 {
		t.Fatalf("want 1 produce call, got %d", prod.calls)
	}
	if st := f.state(t, "e1").status; st != "published" {
		t.Fatalf("e1 should be marked published, status=%q", st)
	}
}

func TestRelayer_RetriesWithBackoffOnFailure(t *testing.T) {
	f := newOutboxFixture(t)
	f.enqueue(t, outbox.Row{EventID: "e1", AggregateType: "Session", AggregateID: "l1", EventType: "T", Payload: []byte("env")})
	prod := &fakePublisher{err: errors.New("broker unavailable")}

	before := time.Now()
	rep, err := f.relayer(t, prod).RelayOnce(context.Background())
	if err != nil {
		t.Fatalf("RelayOnce: %v", err)
	}
	after := time.Now()
	if rep.Retried != 1 || rep.Published != 0 || rep.Parked != 0 {
		t.Fatalf("want a single retry, got %+v", rep)
	}
	// Retry schedules a backoff (attempt 1 => BaseBackoff of 1s) and does NOT park.
	got := f.state(t, "e1")
	if got.status != "pending" {
		t.Fatalf("e1 must be scheduled for retry, not parked: status=%q", got.status)
	}
	if next := got.available; next.Before(before.Add(time.Second-time.Millisecond)) || next.After(after.Add(time.Second+time.Millisecond)) {
		t.Fatalf("want 1s backoff on first retry, available_at=%v (call between %v and %v)", next, before, after)
	}
	if got.attempts != 1 {
		t.Fatalf("want attempts=1, got %d", got.attempts)
	}
}

// v1 has no attempt ceiling: every error is retried with backoff, and only an
// error permanent for the record (outbox.Permanent) parks the row at once. A
// parked row does not block the other aggregates.
func TestRelayer_ParksPermanentErrorRowWithoutBlockingOthers(t *testing.T) {
	f := newOutboxFixture(t)
	f.enqueue(t, outbox.Row{EventID: "poison", AggregateType: "Session", AggregateID: "l1", EventType: "T", Payload: []byte("a")})
	f.enqueue(t, outbox.Row{EventID: "ok", AggregateType: "Session", AggregateID: "l2", EventType: "T", Payload: []byte("b")})
	// The poison record can never be published; the other one fails for now.
	prod := &fakePublisher{errFor: map[string]error{
		"l1": outbox.Permanent(errors.New("record too large")),
		"l2": errors.New("still down"),
	}}

	rep, err := f.relayer(t, prod).RelayOnce(context.Background())
	if err != nil {
		t.Fatalf("RelayOnce: %v", err)
	}
	if rep.Claimed != 2 {
		t.Fatalf("want 2 claimed, got %d", rep.Claimed)
	}
	if st := f.state(t, "poison").status; st != "failed" {
		t.Fatalf("poison row should be parked failed, status=%q", st)
	}
	if rep.Parked != 1 {
		t.Fatalf("want 1 parked failure, got %+v", rep)
	}
	// The healthy row is not parked — it is retried, proving the poison row did
	// not block the queue.
	if st := f.state(t, "ok").status; st != "pending" {
		t.Fatalf("healthy row should be scheduled for retry, status=%q", st)
	}
	if rep.Retried != 1 {
		t.Fatalf("healthy row should be counted as retried, got %+v", rep)
	}
}

// Regression: UPDATE ... RETURNING does not preserve the claim subquery's ORDER BY,
// so two events for one aggregate in the same batch could be relayed newest-first
// (a consumer replaying a user's events out of order). Rows are inserted so the
// physical order is the reverse of seq; the relay must still publish them in seq
// order, oldest-first.
func TestRelayPublishesInCreationOrder(t *testing.T) {
	f := newOutboxFixture(t)
	ctx := context.Background()

	base := time.Now().Add(-time.Minute)
	// Inserted newer-first so heap order disagrees with seq (and created_at).
	for _, r := range []struct {
		id  string
		age time.Duration
		seq int64
	}{{"evt-updated", 2 * time.Second, 2}, {"evt-created", 0, 1}} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO `+store.OutboxTable+`
			(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at, seq)
			VALUES ($1, 'Session', 'user-1', 'T', $3, '', 'pending', 0, $2, $2, $4)`, r.id, base.Add(r.age), []byte(r.id), r.seq); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}

	pub := &orderPublisher{}
	if _, err := f.relayer(t, pub).RelayOnce(ctx); err != nil {
		t.Fatalf("relay: %v", err)
	}
	if len(pub.payloads) != 2 || pub.payloads[0] != "evt-created" || pub.payloads[1] != "evt-updated" {
		t.Fatalf("relay order = %v, want [evt-created evt-updated]", pub.payloads)
	}
}

// orderPublisher records the payloads in the order they are published.
type orderPublisher struct{ payloads []string }

func (p *orderPublisher) PublishRecord(_ context.Context, r *kgo.Record) error {
	p.payloads = append(p.payloads, string(r.Value))
	return nil
}
