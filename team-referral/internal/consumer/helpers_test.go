package consumer_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	eventsv1 "github.com/buidangphuc/team-referral/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-referral/generated/platform/identity/v1"
	orderv1 "github.com/buidangphuc/team-referral/generated/platform/order/v1"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/consumer"
	"github.com/buidangphuc/team-referral/internal/store"
	"github.com/buidangphuc/team-referral/internal/store/fake"
)

var t0 = time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)

const (
	typeUserRegistered  = "platform.identity.v1.UserRegistered"
	typeOrderCompleted  = "platform.order.v1.OrderCompleted"
	typeReturnRequested = "platform.order.v1.ReturnRequested"
	typeAttributed      = "platform.referral.v1.ReferralAttributed"
	typeRejected        = "platform.referral.v1.ReferralRejected"
	typeReleased        = "platform.referral.v1.ReferralRewardReleased"
)

type env struct {
	repo     *fake.InMemoryReferralRepo
	identity consume.Handler
	order    consume.Handler
	cfg      consumer.Config
	seq      int
}

func newEnv(t *testing.T) *env {
	t.Helper()
	repo := fake.NewInMemoryReferralRepo()
	cfg := consumer.Config{RewardXU: 10000, LifetimeCap: 50, Now: func() time.Time { return t0 }}
	return &env{
		repo:     repo,
		identity: consumer.NewIdentityHandler(repo, cfg),
		order:    consumer.NewOrderHandler(repo, cfg),
		cfg:      cfg,
	}
}

func envelope(t testing.TB, eventID, typ string, payload proto.Message) []byte {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		if body, err = proto.Marshal(payload); err != nil {
			t.Fatal(err)
		}
	}
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: typ, OccurredAt: timestamppb.New(t0), Payload: body})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func rawEnvelope(t testing.TB, eventID, typ string, payload []byte) []byte {
	t.Helper()
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: typ, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (e *env) registered(t testing.TB, eventID, user, code string) error {
	return e.identity(context.Background(), consume.Record{
		Topic: "identity.events", Key: []byte(user),
		Value: envelope(t, eventID, typeUserRegistered, &identityv1.UserRegistered{UserId: user, ReferralCode: code, RegisteredAt: timestamppb.New(t0)}),
	})
}

func (e *env) mustRegister(t testing.TB, user, code string) {
	t.Helper()
	e.seq++
	if err := e.registered(t, fmt.Sprintf("ur-%s-%d", user, e.seq), user, code); err != nil {
		t.Fatalf("UserRegistered(%s): %v", user, err)
	}
}

func (e *env) completed(t testing.TB, eventID, order, buyer string, windowEnd time.Time) error {
	return e.order(context.Background(), consume.Record{
		Topic: "order.events", Key: []byte(order),
		Value: envelope(t, eventID, typeOrderCompleted, &orderv1.OrderCompleted{
			OrderId: order, BuyerId: buyer, SellerId: "seller", TotalAmount: 500000, Currency: "VND",
			CompletedAt: timestamppb.New(t0), ReturnWindowEndsAt: timestamppb.New(windowEnd),
		}),
	})
}

func (e *env) mustComplete(t testing.TB, order, buyer string, windowEnd time.Time) {
	t.Helper()
	if err := e.completed(t, "oc-"+order, order, buyer, windowEnd); err != nil {
		t.Fatalf("OrderCompleted(%s): %v", order, err)
	}
}

func (e *env) returned(t testing.TB, eventID, order string) error {
	return e.order(context.Background(), consume.Record{
		Topic: "order.events", Key: []byte(order),
		Value: envelope(t, eventID, typeReturnRequested, &orderv1.ReturnRequested{ReturnId: "ret-" + eventID, OrderId: order, BuyerId: "x", RequestedAt: timestamppb.New(t0)}),
	})
}

func (e *env) rows() []outbox.Row { return e.repo.OutboxRows() }

func (e *env) rowsOfType(typ string) []outbox.Row {
	var out []outbox.Row
	for _, r := range e.rows() {
		if r.EventType == typ {
			out = append(out, r)
		}
	}
	return out
}

func payloadOf(t testing.TB, row outbox.Row, into proto.Message) *eventsv1.EventEnvelope {
	t.Helper()
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(row.Payload, &env); err != nil {
		t.Fatalf("outbox payload is not an EventEnvelope: %v", err)
	}
	if env.GetType() != row.EventType || env.GetEventId() != row.EventID {
		t.Fatalf("envelope type/id %q/%q do not match the row %q/%q", env.GetType(), env.GetEventId(), row.EventType, row.EventID)
	}
	if err := proto.Unmarshal(env.GetPayload(), into); err != nil {
		t.Fatalf("payload: %v", err)
	}
	return &env
}

func (e *env) seedRewards(t testing.TB, inviter string, n int) {
	t.Helper()
	code, err := e.repo.CreateCode(context.Background(), inviter, store.NewCode())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		u := fmt.Sprintf("seed-%s-%d", inviter, i)
		e.mustRegister(t, u, code)
		e.mustComplete(t, "so-"+u, u, t0.Add(48*time.Hour))
	}
}

func isPermanent(err error) bool {
	want := fmt.Sprintf("%T", consume.Permanent(fmt.Errorf("x")))
	for ; err != nil; err = unwrap(err) {
		if fmt.Sprintf("%T", err) == want {
			return true
		}
	}
	return false
}

func unwrap(err error) error {
	u, ok := err.(interface{ Unwrap() error })
	if !ok {
		return nil
	}
	return u.Unwrap()
}

var _ = referralv1.ReferralRejectReason_REFERRAL_REJECT_REASON_UNKNOWN_CODE
