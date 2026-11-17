package events_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	"github.com/buidangphuc/team-order/internal/events"
)

const (
	userErasedType            = "platform.identity.v1.UserErased"
	userRegisteredType        = "platform.identity.v1.UserRegistered"
	identityStatusChangedType = "platform.identity.v1.UserStatusChanged"
	maskText                  = "Đã xóa"
)

func erasureEnvelope(t *testing.T, eventID, typ string, payload proto.Message) consume.Record {
	t.Helper()
	b, err := proto.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	v, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: typ, Payload: b})
	if err != nil {
		t.Fatal(err)
	}
	return consume.Record{Topic: "identity.events", Key: []byte("k"), Value: v}
}

func userErased(t *testing.T, eventID, userID string) consume.Record {
	return erasureEnvelope(t, eventID, userErasedType, &identityv1.UserErased{UserId: userID, ErasedAt: timestamppb.Now()})
}

func (r *rig) seedErasable(user string) {
	r.t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO orders (id, buyer_id, seller_id, status, total_amount, shipping_address) VALUES ('o-` + user + `','` + user + `','s-1',2,1000,
			'{"recipient_name":"Nguyễn Văn A","phone":"0901234567","street":"12 Lê Lợi","ward":"w","district":"d","city":"c"}'::jsonb)`,
		`INSERT INTO order_returns (id, order_id, buyer_id, seller_id, reason, refund_amount, status) VALUES ('r-` + user + `','o-` + user + `','` + user + `','s-1','Hàng bị vỡ',1,1)`,
		`INSERT INTO cart_items (id, user_id, listing_id, unit_price, title, seller_id) VALUES ('c-` + user + `','` + user + `','l-1',1,'Áo','s-1')`,
	} {
		if _, err := r.pool.Exec(ctx, q); err != nil {
			r.t.Fatalf("seed: %v", err)
		}
	}
}

func (r *rig) snapshot() string {
	r.t.Helper()
	var s string
	err := r.pool.QueryRow(context.Background(), `SELECT COALESCE(string_agg(x, '|' ORDER BY x), '') FROM (
		SELECT row_to_json(o)::text AS x FROM orders o
		UNION ALL SELECT row_to_json(t)::text FROM order_returns t
		UNION ALL SELECT row_to_json(c)::text FROM cart_items c) q`).Scan(&s)
	if err != nil {
		r.t.Fatal(err)
	}
	return s
}

func (r *rig) maskedOrder(user string) bool {
	return r.count(`SELECT count(*) FROM orders WHERE buyer_id = $1 AND shipping_address->>'recipient_name' = $2 AND shipping_address->>'phone' = $2`, user, maskText) == 1
}

func TestErasureConsumerMasksBuyerData(t *testing.T) {
	// ORD-1, ORD-2
	r := newRig(t)
	r.seedErasable("u-1")
	r.seedErasable("w-1")
	h := events.NewErasureConsumer(r.orders, quietLogger())

	if err := h.Handle(context.Background(), userErased(t, "e-1", "u-1")); err != nil {
		t.Fatalf("ORD-1 Handle: %v", err)
	}
	if !r.maskedOrder("u-1") {
		t.Fatal("ORD-1 u-1 order not masked")
	}
	if got := r.count(`SELECT count(*) FROM order_returns WHERE buyer_id='u-1' AND reason = $1`, maskText); got != 1 {
		t.Fatalf("ORD-1 return reason masked rows = %d, want 1", got)
	}
	if got := r.count(`SELECT count(*) FROM cart_items WHERE user_id='u-1'`); got != 0 {
		t.Fatalf("ORD-1 cart items = %d, want 0", got)
	}
	if r.maskedOrder("w-1") || r.count(`SELECT count(*) FROM cart_items WHERE user_id='w-1'`) != 1 ||
		r.count(`SELECT count(*) FROM order_returns WHERE buyer_id='w-1' AND reason = 'Hàng bị vỡ'`) != 1 {
		t.Fatal("ORD-2 another user's data changed")
	}
}

func TestErasureConsumerRedeliveryChangesNothing(t *testing.T) {
	// ORD-3
	r := newRig(t)
	r.seedErasable("u-1")
	h := events.NewErasureConsumer(r.orders, quietLogger())
	if err := h.Handle(context.Background(), userErased(t, "e-1", "u-1")); err != nil {
		t.Fatal(err)
	}
	first := r.snapshot()
	for _, id := range []string{"e-1", "e-2"} {
		if err := h.Handle(context.Background(), userErased(t, id, "u-1")); err != nil {
			t.Fatalf("ORD-3 redelivery %s: %v", id, err)
		}
	}
	if again := r.snapshot(); again != first {
		t.Fatalf("ORD-3 rows differ after redelivery\nfirst: %s\nagain: %s", first, again)
	}
}

func TestErasureConsumerParksMalformed(t *testing.T) {
	// ORD-6
	r := newRig(t)
	r.seedErasable("u-1")
	before := r.snapshot()
	h := events.NewErasureConsumer(r.orders, quietLogger())

	mustPermanent(t, h.Handle(context.Background(), userErased(t, "e-1", "")))
	mustPermanent(t, h.Handle(context.Background(), consume.Record{Topic: "identity.events", Value: []byte("\xff\xff not an envelope")}))
	badPayload, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "e-2", Type: userErasedType, Payload: []byte("\xff\xff")})
	if err != nil {
		t.Fatal(err)
	}
	mustPermanent(t, h.Handle(context.Background(), consume.Record{Topic: "identity.events", Value: badPayload}))
	if after := r.snapshot(); after != before {
		t.Fatalf("ORD-6 rows changed\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestErasureConsumerStoreErrorIsRetried(t *testing.T) {
	// ORD-7: a store failure is a plain error so the consume loop retries it
	r := newRig(t)
	r.seedErasable("u-1")
	h := events.NewErasureConsumer(r.orders, quietLogger())
	r.pool.Close()
	mustTransient(t, h.Handle(context.Background(), userErased(t, "e-1", "u-1")))
}

func TestErasureConsumerIgnoresOtherEvents(t *testing.T) {
	// ORD-8
	r := newRig(t)
	r.seedErasable("u-1")
	before := r.snapshot()
	h := events.NewErasureConsumer(r.orders, quietLogger())

	reg := erasureEnvelope(t, "e-1", userRegisteredType, &identityv1.UserRegistered{UserId: "u-1"})
	sc := erasureEnvelope(t, "e-2", identityStatusChangedType, &identityv1.UserStatusChanged{
		UserId: "u-1", Status: identityv1.UserStatus_USER_STATUS_BANNED, ChangedAt: timestamppb.Now(),
	})
	for _, rec := range []consume.Record{reg, sc} {
		if err := h.Handle(context.Background(), rec); err != nil {
			t.Fatalf("ORD-8 Handle: %v", err)
		}
	}
	if after := r.snapshot(); after != before {
		t.Fatalf("ORD-8 rows changed\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestErasureConsumerLogsNoRawPersonalData(t *testing.T) {
	// ORD-1 logging check (tasks 1.15)
	r := newRig(t)
	const userID = "user-raw-id-7f3a9c"
	r.seedErasable(userID)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := events.NewErasureConsumer(r.orders, logger)
	if err := h.Handle(context.Background(), userErased(t, "e-1", userID)); err != nil {
		t.Fatal(err)
	}
	mustPermanent(t, h.Handle(context.Background(), userErased(t, "e-2", "")))
	for _, raw := range []string{userID, "Nguyễn Văn A", "0901234567", "12 Lê Lợi", "Hàng bị vỡ"} {
		if strings.Contains(buf.String(), raw) {
			t.Fatalf("captured log contains raw personal data %q:\n%s", raw, buf.String())
		}
	}
}
