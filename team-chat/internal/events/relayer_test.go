package events_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	commonv1 "github.com/buidangphuc/team-chat/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-chat/generated/platform/events/v1"
	"github.com/buidangphuc/team-chat/internal/events"
	"github.com/buidangphuc/team-chat/internal/interceptor"
	"github.com/buidangphuc/team-chat/internal/repository"
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

func (f *fakeKafka) Publish(_ context.Context, topic, key string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return errors.New("broker unavailable")
	}
	f.sent = append(f.sent, sent{topic, key, payload})
	return nil
}

func (f *fakeKafka) setDown(v bool) { f.mu.Lock(); f.down = v; f.mu.Unlock() }
func (f *fakeKafka) count() int     { f.mu.Lock(); defer f.mu.Unlock(); return len(f.sent) }

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func userCtx(id string) context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{
		Id: id, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
	})
}

func TestBuildMessageOutboxRow(t *testing.T) {
	msg := repository.ChatMessage{
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

func relayerFor(repo repository.OutboxRepository, k events.KafkaPublisher, max int) *events.Relayer {
	return events.NewRelayer(repo, k, events.RelayerConfig{
		Topic: "chat.events", MaxAttempts: max, BaseBackoff: time.Millisecond,
	}, quietLogger())
}

func enqueue(t *testing.T, repo repository.OutboxRepository, id, thread string) {
	t.Helper()
	if err := repo.Enqueue(context.Background(), repository.OutboxRow{
		EventID: id, AggregateType: "ChatThread", AggregateID: thread, EventType: "T", Payload: []byte(id),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRelayerPublishesKeyedByThreadAndOnlyOnce(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepository()
	k := &fakeKafka{}
	enqueue(t, repo, "e1", "thread-A")
	enqueue(t, repo, "e2", "thread-A")
	r := relayerFor(repo, k, 3)

	n, err := r.SweepClaims(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("sweep = %d, %v; want 2", n, err)
	}
	if k.sent[0].topic != "chat.events" || k.sent[0].key != "thread-A" || string(k.sent[0].payload) != "e1" || string(k.sent[1].payload) != "e2" {
		t.Fatalf("sent = %+v", k.sent)
	}
	if n, _ := r.SweepClaims(context.Background()); n != 0 || k.count() != 2 {
		t.Fatalf("published rows must not be re-sent: n=%d sent=%d", n, k.count())
	}
}

// A message sent while Kafka is down is still delivered, once, after it recovers.
func TestRelayerDeliversAfterKafkaRecovers(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepository()
	k := &fakeKafka{}
	k.setDown(true)
	enqueue(t, repo, "e1", "thread-A")
	r := relayerFor(repo, k, 10)

	for i := 0; i < 3; i++ {
		if n, err := r.SweepClaims(context.Background()); err != nil || n != 0 {
			t.Fatalf("sweep while down = %d, %v", n, err)
		}
	}
	if k.count() != 0 {
		t.Fatal("nothing may be published while Kafka is down")
	}

	k.setDown(false)
	if n, err := r.SweepClaims(context.Background()); err != nil || n != 1 {
		t.Fatalf("sweep after recovery = %d, %v; want 1", n, err)
	}
	if n, _ := r.SweepClaims(context.Background()); n != 0 || k.count() != 1 {
		t.Fatalf("exactly one delivery expected: n=%d sent=%d", n, k.count())
	}
}

func TestRelayerParksAfterMaxAttempts(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepository()
	k := &fakeKafka{}
	k.setDown(true)
	enqueue(t, repo, "e1", "thread-A")
	r := relayerFor(repo, k, 2)

	for i := 0; i < 4; i++ {
		_, _ = r.SweepClaims(context.Background())
	}
	k.setDown(false)
	if n, _ := r.SweepClaims(context.Background()); n != 0 || k.count() != 0 {
		t.Fatalf("a parked row must not be claimed again: n=%d sent=%d", n, k.count())
	}
}
