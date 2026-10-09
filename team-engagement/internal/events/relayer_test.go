package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	"github.com/buidangphuc/team-engagement/internal/repository"
)

type fakeStore struct{ msgs []repository.OutboxMessage }

func (f *fakeStore) Relay(ctx context.Context, _ int, _ time.Duration,
	publish func(context.Context, repository.OutboxMessage) error) (int, error) {
	n := 0
	for len(f.msgs) > 0 {
		if err := publish(ctx, f.msgs[0]); err != nil {
			return n, err
		}
		f.msgs = f.msgs[1:]
		n++
	}
	return n, nil
}

type rec struct {
	topic, key string
	value      []byte
}
type fakePub struct {
	got  []rec
	fail bool
}

func (p *fakePub) Publish(_ context.Context, topic, key string, v []byte) error {
	if p.fail {
		return errors.New("down")
	}
	p.got = append(p.got, rec{topic, key, v})
	return nil
}

func TestSweepPublishesEnvelopesInOrder(t *testing.T) {
	at := time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC)
	st := &fakeStore{msgs: []repository.OutboxMessage{
		{Seq: 1, EventID: "e1", Type: "platform.engagement.v1.FavoriteAdded", Key: "L1", Payload: []byte("a"),
			PrincipalID: "buyer-1", PrincipalType: "PRINCIPAL_TYPE_USER", RequestID: "r1", OccurredAt: at},
		{Seq: 2, EventID: "e2", Type: "platform.engagement.v1.SellerFollowed", Key: "S1", Payload: []byte("b"), OccurredAt: at},
	}}
	pub := &fakePub{}
	n, err := NewRelayer(st, pub, RelayerConfig{}, nil).Sweep(context.Background())
	if err != nil || n != 2 || len(pub.got) != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if pub.got[0].topic != "engagement.events" || pub.got[0].key != "L1" || pub.got[1].key != "S1" {
		t.Fatalf("order/key: %+v", pub.got)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(pub.got[0].value, &env); err != nil {
		t.Fatal(err)
	}
	if env.GetEventId() != "e1" || env.GetType() != "platform.engagement.v1.FavoriteAdded" ||
		env.GetPrincipal().GetId() != "buyer-1" || env.GetPrincipal().GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER ||
		env.GetRequestId() != "r1" || string(env.GetPayload()) != "a" || !env.GetOccurredAt().AsTime().Equal(at) {
		t.Fatalf("envelope = %v", &env)
	}
}

func TestSweepPublishFailureKeepsRow(t *testing.T) {
	st := &fakeStore{msgs: []repository.OutboxMessage{{Seq: 1, EventID: "e1", Key: "L1"}}}
	pub := &fakePub{fail: true}
	if n, err := NewRelayer(st, pub, RelayerConfig{}, nil).Sweep(context.Background()); err == nil || n != 0 || len(st.msgs) != 1 {
		t.Fatalf("n=%d err=%v left=%d", n, err, len(st.msgs))
	}
}
