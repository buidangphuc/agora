package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	"github.com/buidangphuc/team-domain/internal/store"
)

const (
	listingTopic = "listing.events"
	changedType  = "platform.listing.v1.ListingChanged"
)

type scriptedPublisher struct {
	mu        sync.Mutex
	transient int
	permanent map[string]bool
	published []*kgo.Record
}

func (p *scriptedPublisher) PublishRecord(_ context.Context, r *kgo.Record) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := headerValue(r, kafkax.HeaderEventID)
	if p.permanent[id] {
		return outbox.Permanent(errors.New("record rejected: " + id))
	}
	if p.transient > 0 {
		p.transient--
		return errors.New("broker unavailable")
	}
	p.published = append(p.published, r)
	return nil
}

func (p *scriptedPublisher) eventIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	ids := make([]string, 0, len(p.published))
	for _, r := range p.published {
		ids = append(ids, headerValue(r, kafkax.HeaderEventID))
	}
	return ids
}

func headerValue(r *kgo.Record, key string) string {
	for _, h := range r.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func envelopeBytes(t *testing.T, eventID string) []byte {
	t.Helper()
	b, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       changedType,
		OccurredAt: timestamppb.New(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)),
		Payload:    []byte("listing-changed-payload"),
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return b
}

func enqueueWith(t *testing.T, eventID string) store.EnqueueFn {
	payload := envelopeBytes(t, eventID)
	return func(l store.Listing) (store.OutboxRow, error) {
		return store.OutboxRow{
			EventID: eventID, AggregateType: "Listing", AggregateID: l.ID,
			EventType: changedType, Payload: payload,
		}, nil
	}
}

type relayEnv struct {
	pool    *pgxpool.Pool
	store   *outbox.Store
	writer  *store.PgTxWriter
	pub     *scriptedPublisher
	relayer *outbox.Relayer
}

func newRelayEnv(t *testing.T) *relayEnv {
	t.Helper()
	pool := newPool(t)
	ostore := newOutboxStore(t, pool)
	pub := &scriptedPublisher{permanent: map[string]bool{}}
	rl, err := outbox.NewRelayer(ostore, pub, outbox.RelayerOptions{
		Topic:       listingTopic,
		BaseBackoff: time.Millisecond,
		MaxBackoff:  time.Millisecond,
	})
	if err != nil {
		t.Fatalf("relayer: %v", err)
	}
	return &relayEnv{pool: pool, store: ostore, writer: store.NewPgTxWriter(pool, ostore), pub: pub, relayer: rl}
}

func (e *relayEnv) create(t *testing.T, listingID, eventID string) {
	t.Helper()
	if _, err := e.writer.CreateTx(t.Context(), store.Listing{
		ID: listingID, Title: listingID, Currency: "VND", Status: "published", Stock: 1,
	}, enqueueWith(t, eventID)); err != nil {
		t.Fatalf("CreateTx %s: %v", listingID, err)
	}
}

func (e *relayEnv) update(t *testing.T, listingID, eventID string) {
	t.Helper()
	cur, err := store.NewPostgresListingRepository(e.pool).Get(t.Context(), listingID)
	if err != nil {
		t.Fatalf("get %s: %v", listingID, err)
	}
	cur.Title += "-v"
	if _, err := e.writer.UpdateTx(t.Context(), cur, enqueueWith(t, eventID)); err != nil {
		t.Fatalf("UpdateTx %s: %v", listingID, err)
	}
}

func (e *relayEnv) makeDue(t *testing.T) {
	t.Helper()
	if _, err := e.pool.Exec(t.Context(),
		`UPDATE outbox_events SET available_at = now() - interval '1 minute', locked_until = NULL`); err != nil {
		t.Fatalf("make rows due: %v", err)
	}
}

func (e *relayEnv) pass(t *testing.T) outbox.Report {
	t.Helper()
	e.makeDue(t)
	rep, err := e.relayer.RelayOnce(t.Context())
	if err != nil {
		t.Fatalf("RelayOnce: %v", err)
	}
	return rep
}

type rowState struct {
	status   string
	attempts int
	lastErr  string
}

func (e *relayEnv) state(t *testing.T, eventID string) rowState {
	t.Helper()
	var s rowState
	if err := e.pool.QueryRow(t.Context(),
		`SELECT status, attempts, COALESCE(error, '') FROM outbox_events WHERE event_id = $1`, eventID).
		Scan(&s.status, &s.attempts, &s.lastErr); err != nil {
		t.Fatalf("state of %s: %v", eventID, err)
	}
	return s
}

func TestRelayerRetriesTransientFailureWithoutLimit(t *testing.T) {
	e := newRelayEnv(t)
	e.create(t, "L1", "evt-1")
	e.pub.transient = 3

	for i := 1; i <= 3; i++ {
		rep := e.pass(t)
		if rep.Retried != 1 || rep.Published != 0 {
			t.Fatalf("pass %d report = %+v, want one retry and no publish", i, rep)
		}
		got := e.state(t, "evt-1")
		if got.status != "pending" || got.attempts != i {
			t.Fatalf("after failing pass %d: status=%q attempts=%d, want pending/%d", i, got.status, got.attempts, i)
		}
	}
	if rep := e.pass(t); rep.Published != 1 {
		t.Fatalf("fourth pass report = %+v, want one publish", rep)
	}
	if got := e.state(t, "evt-1"); got.status != "published" {
		t.Fatalf("after fourth pass: status=%q, want published", got.status)
	}
	if ids := e.pub.eventIDs(); len(ids) != 1 || ids[0] != "evt-1" {
		t.Fatalf("produced event ids = %v, want exactly [evt-1]", ids)
	}
}

func TestPoisonRowParkedOthersPublished(t *testing.T) {
	e := newRelayEnv(t)
	e.create(t, "LP", "evt-poison")
	e.create(t, "LA", "evt-a")
	e.create(t, "LB", "evt-b")
	e.pub.permanent["evt-poison"] = true

	rep := e.pass(t)
	if rep.Parked != 1 || rep.Published != 2 {
		t.Fatalf("report = %+v, want 1 parked and 2 published in the same pass", rep)
	}
	poison := e.state(t, "evt-poison")
	if poison.status != "failed" || poison.lastErr == "" {
		t.Fatalf("poison row = %+v, want failed with its error recorded", poison)
	}
	for _, id := range []string{"evt-a", "evt-b"} {
		if got := e.state(t, id); got.status != "published" {
			t.Fatalf("%s status = %q, want published", id, got.status)
		}
	}
}

func TestParkedRowHoldsBackSameListingOnly(t *testing.T) {
	e := newRelayEnv(t)
	e.create(t, "LX", "evt-x1")
	e.update(t, "LX", "evt-x2")
	e.create(t, "LY", "evt-y1")
	e.pub.permanent["evt-x1"] = true

	e.pass(t)
	e.pass(t)
	if got := e.state(t, "evt-x1"); got.status != "failed" {
		t.Fatalf("evt-x1 status = %q, want failed", got.status)
	}
	if got := e.state(t, "evt-x2"); got.status != "pending" {
		t.Fatalf("evt-x2 status = %q, want pending (held behind the parked row of its own listing)", got.status)
	}
	if got := e.state(t, "evt-y1"); got.status != "published" {
		t.Fatalf("evt-y1 status = %q, want published (other listing not held back)", got.status)
	}

	delete(e.pub.permanent, "evt-x1")
	if err := e.store.Requeue(t.Context(), "evt-x1"); err != nil {
		t.Fatalf("Requeue: %v", err)
	}
	for i := 0; i < 3 && e.state(t, "evt-x2").status != "published"; i++ {
		e.pass(t)
	}
	var order []string
	for _, id := range e.pub.eventIDs() {
		if id == "evt-x1" || id == "evt-x2" {
			order = append(order, id)
		}
	}
	if len(order) != 2 || order[0] != "evt-x1" || order[1] != "evt-x2" {
		t.Fatalf("listing X delivery order = %v, want [evt-x1 evt-x2]", order)
	}
	for _, id := range []string{"evt-x1", "evt-x2"} {
		if got := e.state(t, id); got.status != "published" {
			t.Fatalf("%s status = %q, want published after requeue", id, got.status)
		}
	}
}

func TestRelayedRecordKeepsEnvelopeBytes(t *testing.T) {
	e := newRelayEnv(t)
	e.create(t, "L-wire", "evt-wire")
	var stored []byte
	if err := e.pool.QueryRow(t.Context(),
		`SELECT payload FROM outbox_events WHERE event_id = 'evt-wire'`).Scan(&stored); err != nil {
		t.Fatalf("read payload: %v", err)
	}

	if rep := e.pass(t); rep.Published != 1 {
		t.Fatalf("report = %+v, want one publish", rep)
	}
	if len(e.pub.published) != 1 {
		t.Fatalf("records produced = %d, want 1", len(e.pub.published))
	}
	rec := e.pub.published[0]
	if rec.Topic != listingTopic {
		t.Errorf("topic = %q, want %q", rec.Topic, listingTopic)
	}
	if string(rec.Key) != "L-wire" {
		t.Errorf("key = %q, want the listing id", rec.Key)
	}
	if string(rec.Value) != string(stored) || string(rec.Value) != string(envelopeBytes(t, "evt-wire")) {
		t.Errorf("record value differs from the stored envelope bytes")
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		t.Fatalf("unmarshal record value: %v", err)
	}
	if env.GetEventId() != "evt-wire" {
		t.Errorf("EventEnvelope.event_id = %q, want evt-wire", env.GetEventId())
	}
}

func TestPurgeDeletesOnlyOldPublishedRows(t *testing.T) {
	e := newRelayEnv(t)
	insert := func(id, status, age string) {
		t.Helper()
		if _, err := e.pool.Exec(t.Context(), `INSERT INTO outbox_events
			(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at, published_at)
			VALUES ($1, 'Listing', $1, $2, '\x00', '', $3, 0, now(), now() - $4::interval,
			        CASE WHEN $3 = 'published' THEN now() - $4::interval END)`,
			id, changedType, status, age); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insert("pub-8d", "published", "8 days")
	insert("pub-6d", "published", "6 days")
	insert("pend-8d", "pending", "8 days")
	insert("fail-8d", "failed", "8 days")

	n, err := e.store.Purge(t.Context(), 7*24*time.Hour)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if n != 1 {
		t.Fatalf("purged %d rows, want 1", n)
	}
	var gone int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_events WHERE event_id = 'pub-8d'`).Scan(&gone); err != nil || gone != 0 {
		t.Fatalf("pub-8d count = %d err=%v, want 0", gone, err)
	}
	var left int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_events`).Scan(&left); err != nil || left != 3 {
		t.Fatalf("rows left = %d err=%v, want 3", left, err)
	}
}
