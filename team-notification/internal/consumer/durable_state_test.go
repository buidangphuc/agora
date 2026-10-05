package consumer

import (
	"context"
	"testing"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

// stateBundle stands in for the durable stores: it outlives any one consumer
// instance, so building a second consumer over it simulates a process restart.
type stateBundle struct {
	dedupe Deduper
	price  PriceStateStore
	stock  StockStateStore
}

func newBundle() stateBundle {
	return stateBundle{NewInMemoryDeduper(), NewInMemoryPriceStateStore(), NewInMemoryStockStateStore()}
}

func (b stateBundle) listingConsumer(notif NotificationCreator, subs SubscriptionStore) *ListingConsumer {
	return NewListingConsumer(notif, subs, nil, WithDeduper(b.dedupe), WithPriceStateStore(b.price), WithStockStateStore(b.stock))
}

func priceDropSubs() *fakeSubs {
	return &fakeSubs{byKey: map[string][]*notificationv1.AlertSubscription{
		subKey("listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP): {
			sub("user_a", "listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP),
		},
		subKey("listing_1", notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK): {
			sub("user_a", "listing_1", notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK),
		},
	}}
}

// A price drop after a restart is still detected: the baseline survives in the
// injected store, so the fresh consumer diffs against it.
func TestPriceDropDetectedAfterRestart(t *testing.T) {
	notif := &fakeNotif{}
	state := newBundle()
	ctx := context.Background()

	before := state.listingConsumer(notif, priceDropSubs())
	if err := before.HandleRaw(ctx, priceEnvelope(t, "evt_1", "listing_1", 2_000_000)); err != nil {
		t.Fatal(err)
	}
	after := state.listingConsumer(notif, priceDropSubs()) // "restart"
	if err := after.HandleRaw(ctx, priceEnvelope(t, "evt_2", "listing_1", 500_000)); err != nil {
		t.Fatal(err)
	}
	if notif.count() != 1 {
		t.Fatalf("price drop after restart: %d notifications, want 1", notif.count())
	}
}

// A restock after a restart is still detected.
func TestRestockDetectedAfterRestart(t *testing.T) {
	notif := &fakeNotif{}
	state := newBundle()
	ctx := context.Background()

	if err := state.listingConsumer(notif, priceDropSubs()).HandleRaw(ctx, stockEnvelope(t, "evt_1", "listing_1", 0)); err != nil {
		t.Fatal(err)
	}
	if err := state.listingConsumer(notif, priceDropSubs()).HandleRaw(ctx, stockEnvelope(t, "evt_2", "listing_1", 5)); err != nil {
		t.Fatal(err)
	}
	if notif.count() != 1 {
		t.Fatalf("restock after restart: %d notifications, want 1", notif.count())
	}
}

// A redelivered event after a restart notifies once: the durable ledger remembers it.
func TestRedeliveredEventAfterRestartNotifiesOnce(t *testing.T) {
	notif := &fakeNotif{}
	state := newBundle()
	ctx := context.Background()
	seed := priceEnvelope(t, "evt_1", "listing_1", 100)
	drop := priceEnvelope(t, "evt_2", "listing_1", 50)

	first := state.listingConsumer(notif, priceDropSubs())
	if err := first.HandleRaw(ctx, seed); err != nil {
		t.Fatal(err)
	}
	if err := first.HandleRaw(ctx, drop); err != nil {
		t.Fatal(err)
	}
	// Restart before the offset was committed: the same record arrives again.
	if err := state.listingConsumer(notif, priceDropSubs()).HandleRaw(ctx, drop); err != nil {
		t.Fatal(err)
	}
	if notif.count() != 1 {
		t.Fatalf("redelivery after restart: %d notifications, want 1", notif.count())
	}
}

type failingPriceStore struct{ PriceStateStore }

func (failingPriceStore) Get(context.Context, string) (int64, bool, error) {
	return 0, false, context.DeadlineExceeded
}

// A store read failure is transient: the record errors (and is retried) instead of
// being treated as "unknown prior", which would silently reset the baseline.
func TestStateReadFailureIsTransient(t *testing.T) {
	notif := &fakeNotif{}
	c := NewListingConsumer(notif, priceDropSubs(), nil, WithPriceStateStore(failingPriceStore{}))
	err := c.HandleRaw(context.Background(), priceEnvelope(t, "evt_1", "listing_1", 100))
	if err == nil {
		t.Fatal("expected an error when the baseline cannot be read")
	}
}

// A chat event redelivered after a restart notifies once: the ledger outlives the
// consumer instance.
func TestChatRedeliveryAfterRestartNotifiesOnce(t *testing.T) {
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	ledger := NewInMemoryDeduper()
	raw := chatEnvelope(t, "evt-1", sellerReply())
	ctx := context.Background()

	if err := NewChatConsumer(notif, prefs, nil, WithUserEventDeduper(ledger)).HandleRaw(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if err := NewChatConsumer(notif, prefs, nil, WithUserEventDeduper(ledger)).HandleRaw(ctx, raw); err != nil { // "restart"
		t.Fatal(err)
	}
	if notif.count() != 1 {
		t.Fatalf("%d notifications, want 1", notif.count())
	}
}
