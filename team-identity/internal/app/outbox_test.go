package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/store"
)

const (
	evSessionRevoked = "platform.identity.v1.SessionRevoked"
	traceID          = "4bf92f3577b34da6a3ce929d0e0e4736"
)

func enqueue(t testing.TB, pool *pgxpool.Pool, eventID, aggregateID string) {
	t.Helper()
	ob, err := store.Outbox(pool)
	if err != nil {
		t.Fatalf("store.Outbox: %v", err)
	}
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	row := store.OutboxRow{EventID: eventID, AggregateType: "User", AggregateID: aggregateID, EventType: evSessionRevoked, Payload: []byte("payload-" + eventID)}
	if err := ob.EnqueueTx(ctx, tx, row); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("EnqueueTx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func buildForTest(t *testing.T, url string, pub interface {
	PublishRecord(context.Context, *kgo.Record) error
}) *built {
	t.Helper()
	ctx := context.Background()
	s := newSettings(t, url, deadBroker)
	s.LogLevel = "info"
	b, err := build(ctx, s, pub)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if b.pool == nil || b.relayer == nil || b.purgeTick == nil || b.closeAll == nil {
		t.Fatalf("build returned an incomplete assembly: %+v", b)
	}
	t.Cleanup(func() { _ = b.closeAll(context.Background()) })
	return b
}

func remainingIDs(t *testing.T, url string) string {
	t.Helper()
	return queryString(t, url, `SELECT coalesce(string_agg(event_id, ',' ORDER BY event_id), '') FROM identity_outbox_events`)
}

func TestOutboxPurgeKeepsRecentAndUnpublished(t *testing.T) {
	// scenario: Published rows older than seven days are purged
	url := migratedURL(t)
	logs := captureLogs(t)
	b := buildForTest(t, url, testkit.NewRecorder())
	for _, id := range []string{"old-1", "old-2", "recent-1", "recent-2", "pending-1", "parked-1"} {
		enqueue(t, b.pool, id, "u-1")
	}
	execSQL(t, url, `UPDATE identity_outbox_events SET status = 'published', published_at = now() - interval '8 days' WHERE event_id LIKE 'old-%'`)
	execSQL(t, url, `UPDATE identity_outbox_events SET status = 'published', published_at = now() - interval '6 days' WHERE event_id LIKE 'recent-%'`)
	execSQL(t, url, `UPDATE identity_outbox_events SET status = 'failed', error = 'permanent' WHERE event_id = 'parked-1'`)

	if err := b.purgeTick(context.Background()); err != nil {
		t.Fatalf("purge tick = %v, want nil", err)
	}
	if got, want := remainingIDs(t, url), "parked-1,pending-1,recent-1,recent-2"; got != want {
		t.Fatalf("rows after purge = %s, want %s", got, want)
	}
	found := false
	for _, l := range logs.Lines() {
		if v, ok := l["rows_purged"].(float64); ok && v == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("no log line carries rows_purged=2:\n%s", logs.String())
	}
}

func TestOutboxPurgeSurvivesError(t *testing.T) {
	// scenario: A failed purge is retried and logs counts only
	url := migratedURL(t)
	logs := captureLogs(t)
	s := newSettings(t, url, deadBroker)
	s.LogLevel = "info"
	ctx := context.Background()
	b, err := build(ctx, s, testkit.NewRecorder())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	t.Cleanup(func() { _ = b.closeAll(context.Background()) })
	enqueue(t, b.pool, "old-1", "u-1")
	enqueue(t, b.pool, "old-2", "u-1")
	execSQL(t, url, `UPDATE identity_outbox_events SET status = 'published', published_at = now() - interval '8 days'`)

	execSQL(t, url, `ALTER TABLE identity_outbox_events RENAME TO identity_outbox_events_away`)
	before := len(logs.Lines())
	if err := b.purgeTick(ctx); err != nil {
		t.Fatalf("failing purge tick = %v, want nil so the next tick retries", err)
	}
	warns := 0
	for _, l := range logs.Lines()[before:] {
		if lvl, _ := l["level"].(string); strings.EqualFold(lvl, "WARN") {
			warns++
		}
		raw := strings.ToLower(toJSON(l))
		if strings.Contains(raw, "does not exist") || strings.Contains(raw, "identity_outbox_events") {
			t.Errorf("log line carries the error text: %v", l)
		}
	}
	if warns != 1 {
		t.Errorf("warnings logged by the failed tick = %d, want 1", warns)
	}

	execSQL(t, url, `ALTER TABLE identity_outbox_events_away RENAME TO identity_outbox_events`)
	if err := b.purgeTick(ctx); err != nil {
		t.Fatalf("second purge tick = %v, want nil", err)
	}
	if got := remainingIDs(t, url); got != "" {
		t.Errorf("rows after the retry = %q, want none", got)
	}
	found := false
	for _, l := range logs.Lines()[before:] {
		if v, ok := l["rows_purged"].(float64); ok && v == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("the second tick did not log rows_purged=2:\n%s", logs.String())
	}
}

type flakyPublisher struct {
	mu       sync.Mutex
	failures int
	calls    int
	got      []*kgo.Record
}

func (f *flakyPublisher) PublishRecord(_ context.Context, r *kgo.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.failures {
		return errors.New("broker unavailable")
	}
	f.got = append(f.got, r)
	return nil
}

func (f *flakyPublisher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestTransientPublishFailuresNeverPark(t *testing.T) {
	// scenario: A broker outage keeps rows pending and retries without a limit
	url := migratedURL(t)
	pub := &flakyPublisher{failures: 12}
	b := buildForTest(t, url, pub)
	enqueue(t, b.pool, "evt-1", "u-1")
	ctx := context.Background()
	status := func() string {
		return queryString(t, url, `SELECT status FROM identity_outbox_events WHERE event_id = 'evt-1'`)
	}
	for i := 1; i <= 12; i++ {
		execSQL(t, url, `UPDATE identity_outbox_events SET available_at = now() - interval '1 second', locked_until = NULL WHERE event_id = 'evt-1'`)
		rep, err := b.relayer.RelayOnce(ctx)
		if err != nil {
			t.Fatalf("RelayOnce #%d: %v", i, err)
		}
		if rep.Parked != 0 {
			t.Fatalf("pass %d parked %d rows, want 0", i, rep.Parked)
		}
		if got := pub.count(); got != i {
			t.Fatalf("publish attempts after pass %d = %d, want %d", i, got, i)
		}
		if got := status(); got != "pending" {
			t.Fatalf("status after failure %d = %s, want pending", i, got)
		}
	}
	execSQL(t, url, `UPDATE identity_outbox_events SET available_at = now() - interval '1 second', locked_until = NULL WHERE event_id = 'evt-1'`)
	if _, err := b.relayer.RelayOnce(ctx); err != nil {
		t.Fatalf("RelayOnce #13: %v", err)
	}
	if got := pub.count(); got != 13 {
		t.Errorf("publish attempts = %d, want 13", got)
	}
	if got := status(); got != "published" {
		t.Errorf("status after the 13th attempt = %s, want published", got)
	}
}

func header(rec *kgo.Record, key string) string {
	for _, h := range rec.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func TestRelayedRecordCarriesKeyTopicAndHeaders(t *testing.T) {
	// scenario: Published records carry key, topic and headers
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	captureSpans(t)
	s := newSettings(t, url, brokers)
	s.Events.IdentityTopic = "identity.events.test"
	r := start(t, s)
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	auth := identityv1.NewAuthServiceClient(r.conn)
	reg, err := auth.Register(ctx, &identityv1.RegisterRequest{Username: "carol", Password: testPassword, Role: "buyer"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	userID := reg.GetResult().GetPrincipal().GetId()

	sessions := identityv1.NewSessionServiceClient(r.conn)
	uctx := asUser(ctx, userID)
	list, err := sessions.ListSessions(uctx, &identityv1.ListSessionsRequest{})
	if err != nil || len(list.GetSessions()) == 0 {
		t.Fatalf("ListSessions = %v, %v, want the registration session", list, err)
	}
	rctx := metadata.AppendToOutgoingContext(uctx,
		"x-request-id", "req-relay-1",
		"traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")
	if _, err := sessions.RevokeSession(rctx, &identityv1.RevokeSessionRequest{SessionId: list.GetSessions()[0].GetId()}); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	recs := testkit.ReadAll(t, brokers, "identity.events.test", 1, 60*time.Second)
	rec := recs[0]
	if rec.Topic != "identity.events.test" {
		t.Errorf("topic = %s, want identity.events.test", rec.Topic)
	}
	if string(rec.Key) != userID {
		t.Errorf("key = %q, want the user id %q", rec.Key, userID)
	}
	var wantPayload []byte
	if err := dial(t, url).QueryRow(context.Background(), `SELECT payload FROM identity_outbox_events WHERE event_type = $1`, evSessionRevoked).Scan(&wantPayload); err != nil {
		t.Fatalf("read stored envelope: %v", err)
	}
	if !bytes.Equal(rec.Value, wantPayload) {
		t.Error("record value differs from the stored envelope bytes")
	}
	wantEventID := queryString(t, url, `SELECT event_id FROM identity_outbox_events WHERE event_type = $1`, evSessionRevoked)
	if got := header(rec, "event-id"); got != wantEventID {
		t.Errorf("event-id header = %q, want %q", got, wantEventID)
	}
	if got := header(rec, "event-type"); got != evSessionRevoked {
		t.Errorf("event-type header = %q, want %q", got, evSessionRevoked)
	}
	if got := header(rec, "x-request-id"); got != "req-relay-1" {
		t.Errorf("x-request-id header = %q, want req-relay-1", got)
	}
	if got := header(rec, "traceparent"); !strings.Contains(got, traceID) {
		t.Errorf("traceparent header = %q, want it to carry trace %s", got, traceID)
	}
}

func TestKafkaOutageKeepsReadinessAndAuth(t *testing.T) {
	// scenario: A Kafka outage does not fail readiness or login
	url := migratedURL(t)
	r := start(t, newSettings(t, url, deadBroker))
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	auth := identityv1.NewAuthServiceClient(r.conn)
	reg, err := auth.Register(ctx, &identityv1.RegisterRequest{Username: "dave", Password: testPassword, Role: "buyer"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	userID := reg.GetResult().GetPrincipal().GetId()
	if _, err := auth.Login(ctx, &identityv1.LoginRequest{Username: "dave", Password: testPassword}); err != nil {
		t.Fatalf("Login with Kafka unreachable = %v, want OK", err)
	}
	sessions := identityv1.NewSessionServiceClient(r.conn)
	uctx := asUser(ctx, userID)
	list, err := sessions.ListSessions(uctx, &identityv1.ListSessionsRequest{})
	if err != nil || len(list.GetSessions()) == 0 {
		t.Fatalf("ListSessions = %v, %v", list, err)
	}
	if _, err := sessions.RevokeSession(uctx, &identityv1.RevokeSessionRequest{SessionId: list.GetSessions()[0].GetId()}); err != nil {
		t.Fatalf("RevokeSession with Kafka unreachable = %v, want OK", err)
	}
	if n := queryInt(t, url, `SELECT count(*) FROM identity_outbox_events WHERE event_type = $1 AND status = 'pending'`, evSessionRevoked); n != 1 {
		t.Errorf("pending SessionRevoked rows = %d, want 1", n)
	}
	if got := healthStatus(r.conn, ""); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("readiness = %v with Kafka unreachable, want SERVING", got)
	}
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
