package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	sdkpgx "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	eventsv1 "github.com/buidangphuc/team-chat/generated/platform/events/v1"
	"github.com/buidangphuc/team-chat/internal/events"
	"github.com/buidangphuc/team-chat/internal/store"
	"github.com/buidangphuc/team-chat/migrations"
)

type sent struct {
	topic, key string
	payload    []byte
}

type fakeKafka struct {
	mu   sync.Mutex
	down bool
	sent []sent
}

func (f *fakeKafka) Publish(_ context.Context, topic string, key, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return errors.New("broker unavailable")
	}
	f.sent = append(f.sent, sent{topic, string(key), payload})
	return nil
}

func (f *fakeKafka) setDown(v bool) { f.mu.Lock(); f.down = v; f.mu.Unlock() }
func (f *fakeKafka) count() int     { f.mu.Lock(); defer f.mu.Unlock(); return len(f.sent) }

func userCtx(id string) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{ID: id, Type: "user"})
}

func TestBuildMessageOutboxRow(t *testing.T) {
	msg := store.ChatMessage{
		ID: "m1", ThreadID: "t1", SenderID: "seller-1", RecipientID: "buyer-1", SellerID: "seller-1",
		Content: "hi", CreatedAt: time.Unix(1700000000, 0),
	}
	row, err := events.BuildMessageOutboxRow(userCtx("seller-1"), msg)
	if err != nil {
		t.Fatal(err)
	}
	if row.AggregateID != "t1" || row.EventType != events.ChatMessageSentType || row.EventID != events.ChatMessageEventID("m1") {
		t.Fatalf("row = %+v", row)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(row.Payload, &env); err != nil {
		t.Fatal(err)
	}
	if env.GetEventId() != row.EventID || env.GetPrincipal().GetId() != "seller-1" {
		t.Fatalf("envelope = %+v", &env)
	}
	var out chatv1.ChatMessage
	if err := proto.Unmarshal(env.GetPayload(), &out); err != nil {
		t.Fatal(err)
	}
	if out.GetSellerId() != "seller-1" || out.GetRecipientId() != "buyer-1" || out.GetSenderId() != "seller-1" || out.GetThreadId() != "t1" {
		t.Fatalf("payload = %+v", &out)
	}
	// The id is stable per message so a retried enqueue collapses onto one row.
	row2, _ := events.BuildMessageOutboxRow(context.Background(), msg)
	if row2.EventID != row.EventID {
		t.Fatalf("event id not deterministic: %s vs %s", row.EventID, row2.EventID)
	}
}

// newOutbox opens a migrated testkit Postgres and returns the pool with the
// chat outbox store over it.
func newOutbox(t *testing.T) (*pgxpool.Pool, *outbox.Store) {
	t.Helper()
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
	return pool, outbox.NewStore(pool, store.OutboxTable)
}

func relayerFor(s *outbox.Store, k outbox.Publisher, maxAttempts int) *outbox.Relayer {
	return outbox.NewRelayer(s, k, outbox.RelayerOptions{
		Topic:  "chat.events",
		Config: sdkconfig.Outbox{BatchSize: 100, ClaimLockSeconds: 60, MaxAttempts: maxAttempts},
	})
}

func enqueue(t *testing.T, pool *pgxpool.Pool, s *outbox.Store, id, thread string) {
	t.Helper()
	err := sdkpgx.WithTx(context.Background(), pool, func(tx pgx.Tx) error {
		return s.EnqueueTx(context.Background(), tx, store.OutboxRow{
			EventID: id, AggregateType: "ChatThread", AggregateID: thread, EventType: "T", Payload: []byte(id),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// release lifts the retry backoff gate so the next sweep may claim a failed row
// again, without waiting out the delay or depending on clock agreement with the
// database.
func release(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE chat_outbox_events SET available_at = now() WHERE status = 'pending'`); err != nil {
		t.Fatal(err)
	}
}

func sweep(t *testing.T, r *outbox.Relayer) int {
	t.Helper()
	rep, err := r.RelayOnce(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	return rep.Published
}

func TestRelayerPublishesKeyedByThreadAndOnlyOnce(t *testing.T) {
	pool, s := newOutbox(t)
	k := &fakeKafka{}
	enqueue(t, pool, s, "e1", "thread-A")
	enqueue(t, pool, s, "e2", "thread-A")
	r := relayerFor(s, k, 3)

	if n := sweep(t, r); n != 2 {
		t.Fatalf("sweep = %d; want 2", n)
	}
	if k.sent[0].topic != "chat.events" || k.sent[0].key != "thread-A" || string(k.sent[0].payload) != "e1" || string(k.sent[1].payload) != "e2" {
		t.Fatalf("sent = %+v", k.sent)
	}
	if n := sweep(t, r); n != 0 || k.count() != 2 {
		t.Fatalf("published rows must not be re-sent: n=%d sent=%d", n, k.count())
	}
}

// A message sent while Kafka is down is still delivered, once, after it recovers.
func TestRelayerDeliversAfterKafkaRecovers(t *testing.T) {
	pool, s := newOutbox(t)
	k := &fakeKafka{}
	k.setDown(true)
	enqueue(t, pool, s, "e1", "thread-A")
	r := relayerFor(s, k, 10)

	for i := 0; i < 3; i++ {
		release(t, pool)
		if n := sweep(t, r); n != 0 {
			t.Fatalf("sweep while down = %d", n)
		}
	}
	if k.count() != 0 {
		t.Fatal("nothing may be published while Kafka is down")
	}

	k.setDown(false)
	release(t, pool)
	if n := sweep(t, r); n != 1 {
		t.Fatalf("sweep after recovery = %d; want 1", n)
	}
	if n := sweep(t, r); n != 0 || k.count() != 1 {
		t.Fatalf("exactly one delivery expected: n=%d sent=%d", n, k.count())
	}
}

func TestRelayerParksAfterMaxAttempts(t *testing.T) {
	pool, s := newOutbox(t)
	k := &fakeKafka{}
	k.setDown(true)
	enqueue(t, pool, s, "e1", "thread-A")
	r := relayerFor(s, k, 2)

	for i := 0; i < 4; i++ {
		release(t, pool)
		sweep(t, r)
	}
	k.setDown(false)
	release(t, pool)
	if n := sweep(t, r); n != 0 || k.count() != 0 {
		t.Fatalf("a parked row must not be claimed again: n=%d sent=%d", n, k.count())
	}
}
