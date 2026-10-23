package events_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	listingv1 "github.com/buidangphuc/team-notification/generated/platform/listing/v1"
	orderv1 "github.com/buidangphuc/team-notification/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-notification/generated/platform/payment/v1"
	"github.com/buidangphuc/team-notification/internal/events"
)

func TestRedeliveredEventNotifiesOnce_Postgres(t *testing.T) {
	pool := newDB(t)
	subscribe(t, pool, "user_a", "listing_1", priceDrop)
	mustDeliver(t, newListing(pool), priceEvent(t, "evt_seed", "listing_1", 100))

	cases := []struct {
		name    string
		h       consume.Handler
		eventID string
		raw     []byte
		user    string
	}{
		{"chat", events.NewChatHandler(pool, nil, nil), "evt-chat", chatEnvelope(t, "evt-chat", sellerReply()), "buyer-1"},
		{"order", events.NewOrderHandler(pool, nil), "evt-order", envelopeOf(t, "evt-order", orderShippedType, shipped()), "buyer-1"},
		{"listing", newListing(pool), "evt-listing", priceEvent(t, "evt-listing", "listing_1", 80), "user_a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := len(notificationsOf(t, pool, c.user))
			for i := 0; i < 3; i++ {
				mustDeliver(t, c.h, c.raw)
			}
			if got := len(notificationsOf(t, pool, c.user)) - before; got != 1 {
				t.Fatalf("redelivery must notify once, got %d new notifications", got)
			}
			if got := ledgerRows(t, pool, c.eventID); got != 1 {
				t.Fatalf("ledger rows for %s = %d, want 1", c.eventID, got)
			}
		})
	}
}

func TestSameEventIDIsIndependentAcrossConsumers(t *testing.T) {
	pool := newDB(t)
	chat := events.NewChatHandler(pool, nil, nil)
	order := events.NewOrderHandler(pool, nil)

	mustDeliver(t, chat, chatEnvelope(t, "shared-id", sellerReply()))
	mustDeliver(t, order, envelopeOf(t, "shared-id", orderShippedType, shipped()))

	if got := totalNotifications(t, pool); got != 2 {
		t.Fatalf("each consumer keeps its own ledger, notifications = %d, want 2", got)
	}
	if got := ledgerRows(t, pool, "shared-id"); got != 2 {
		t.Fatalf("ledger rows = %d, want 2", got)
	}
}

func TestFailedWriteRollsBackLedgerAndRetryNotifies_Postgres(t *testing.T) {
	t.Run("chat", func(t *testing.T) {
		pool := newDB(t)
		h := events.NewChatHandler(pool, nil, nil)
		raw := chatEnvelope(t, "evt-retry", sellerReply())

		failInserts(t, pool)
		requireTransient(t, deliver(h, raw))
		if got := ledgerRows(t, pool, "evt-retry"); got != 0 {
			t.Fatalf("a failed write must roll the ledger row back, rows = %d", got)
		}
		healInserts(t, pool)
		mustDeliver(t, h, raw)
		one(t, pool, "buyer-1")
		if got := ledgerRows(t, pool, "evt-retry"); got != 1 {
			t.Fatalf("ledger rows after retry = %d, want 1", got)
		}
	})

	t.Run("order", func(t *testing.T) {
		pool := newDB(t)
		h := events.NewOrderHandler(pool, nil)
		raw := envelopeOf(t, "e-retry", orderDoneType, completed("O1", orderv1.CompletedBy_COMPLETED_BY_SYSTEM))

		failInserts(t, pool)
		requireTransient(t, deliver(h, raw))
		if got := ledgerRows(t, pool, "e-retry"); got != 0 {
			t.Fatalf("a failed write must roll the ledger row back, rows = %d", got)
		}
		healInserts(t, pool)
		mustDeliver(t, h, raw)
		one(t, pool, "B1")
	})

	t.Run("listing keeps the baseline", func(t *testing.T) {
		pool := newDB(t)
		subscribe(t, pool, "user_a", "listing_1", priceDrop)
		h := newListing(pool)
		mustDeliver(t, h, priceEvent(t, "evt_seed", "listing_1", 100))
		raw := priceEvent(t, "evt_drop", "listing_1", 80)

		failInserts(t, pool)
		requireTransient(t, deliver(h, raw))
		if got := ledgerRows(t, pool, "evt_drop"); got != 0 {
			t.Fatalf("a failed write must roll the ledger row back, rows = %d", got)
		}
		healInserts(t, pool)
		mustDeliver(t, h, raw)
		one(t, pool, "user_a")
	})
}

func TestOrder_TransientFailureDoesNotRecordEvent(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)
	mustDeliver(t, h, envelopeOf(t, "e-0", returnRefType, &orderv1.ReturnRefunded{ReturnId: "R0", OrderId: "O0", BuyerId: "B0", RefundAmount: 1}))
	failInserts(t, pool)

	requireTransient(t, deliver(h, envelopeOf(t, "e-x", returnRefType, &orderv1.ReturnRefunded{ReturnId: "R1", OrderId: "O1", BuyerId: "B1", RefundAmount: 1})))

	if got := ledgerRows(t, pool, "e-x"); got != 0 {
		t.Fatalf("ledger rows = %d, want 0", got)
	}
}

func TestEventsNeverCreateRowsForOtherUsers(t *testing.T) {
	order := func(p *pgxpool.Pool) consume.Handler { return events.NewOrderHandler(p, nil) }
	cases := []struct {
		name string
		h    func(p *pgxpool.Pool) consume.Handler
		raw  func(t *testing.T) []byte
		want string
	}{
		{"chat", func(p *pgxpool.Pool) consume.Handler { return events.NewChatHandler(p, nil, nil) },
			func(t *testing.T) []byte { return chatEnvelope(t, "e1", sellerReply()) }, "buyer-1"},
		{"shipped", order, func(t *testing.T) []byte { return envelopeOf(t, "e1", orderShippedType, shipped()) }, "buyer-1"},
		{"placed COD", order, func(t *testing.T) []byte {
			return envelopeOf(t, "e1", orderPlacedType, placed(paymentv1.PaymentMethod_PAYMENT_METHOD_COD))
		}, "S1"},
		{"paid", order, func(t *testing.T) []byte { return envelopeOf(t, "e1", orderPaidType, paid()) }, "S1"},
		{"reminder", order, func(t *testing.T) []byte { return envelopeOf(t, "e1", orderReminderType, reminder()) }, "B1"},
		{"completed", order, func(t *testing.T) []byte {
			return envelopeOf(t, "e1", orderDoneType, completed("O1", orderv1.CompletedBy_COMPLETED_BY_BUYER))
		}, "B1"},
		{"return requested", order, func(t *testing.T) []byte {
			return envelopeOf(t, "e1", returnReqType, &orderv1.ReturnRequested{ReturnId: "R1", OrderId: "O1", BuyerId: "B1", SellerId: "S1"})
		}, "S1"},
		{"return refunded", order, func(t *testing.T) []byte {
			return envelopeOf(t, "e1", returnRefType, &orderv1.ReturnRefunded{ReturnId: "R1", OrderId: "O1", BuyerId: "B1", SellerId: "S1"})
		}, "B1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pool := newDB(t)
			mustDeliver(t, c.h(pool), c.raw(t))
			if got := totalNotifications(t, pool); got != 1 {
				t.Fatalf("want exactly 1 row in total, got %d", got)
			}
			one(t, pool, c.want)
		})
	}

	t.Run("price drop reaches subscribers of that listing only", func(t *testing.T) {
		pool := newDB(t)
		subscribe(t, pool, "user_a", "listing_1", priceDrop)
		subscribe(t, pool, "user_b", "listing_2", priceDrop)
		subscribe(t, pool, "user_c", "listing_1", backInStock)
		h := newListing(pool)

		mustDeliver(t, h, priceEvent(t, "e1", "listing_1", 100))
		mustDeliver(t, h, priceEvent(t, "e2", "listing_1", 80))

		one(t, pool, "user_a")
		if got := totalNotifications(t, pool); got != 1 {
			t.Fatalf("only user_a may be notified, total %d", got)
		}
	})

	t.Run("low stock reaches the owner of that listing only", func(t *testing.T) {
		pool := newDB(t)
		h := newListing(pool)

		mustDeliver(t, h, snapshot(t, "e1", "L1", "S1", 8, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
		mustDeliver(t, h, snapshot(t, "e2", "L2", "S2", 8, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
		mustDeliver(t, h, stockChanged(t, "e3", "L1", 2))

		one(t, pool, "S1")
		if got := totalNotifications(t, pool); got != 1 {
			t.Fatalf("only S1 may be notified, total %d", got)
		}
	})
}
