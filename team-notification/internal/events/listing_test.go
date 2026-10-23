package events_test

import (
	"context"
	"strings"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/jackc/pgx/v5/pgxpool"

	listingv1 "github.com/buidangphuc/team-notification/generated/platform/listing/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/events"
	"github.com/buidangphuc/team-notification/internal/store"
)

const (
	listingChangedType      = "platform.listing.v1.ListingChanged"
	listingStockChangedType = "platform.listing.v1.ListingStockChanged"
	lowStockTitle           = "Sắp hết hàng"
	lowStockListingTitle    = "Giày chạy bộ"
	defaultThreshold        = int32(5)
)

var (
	priceDrop   = notificationv1.AlertType_ALERT_TYPE_PRICE_DROP
	backInStock = notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK
)

func listingChanged(t *testing.T, eventID, listingID string, price int64, stock int32, ct listingv1.ChangeType) []byte {
	t.Helper()
	return envelopeOf(t, eventID, listingChangedType, &listingv1.ListingChanged{
		Listing:    &listingv1.Listing{Id: listingID, Price: price, Stock: stock},
		ChangeType: ct,
	})
}

func priceEvent(t *testing.T, eventID, listingID string, price int64) []byte {
	t.Helper()
	return listingChanged(t, eventID, listingID, price, 0, listingv1.ChangeType_CHANGE_TYPE_UPDATED)
}

func stockEvent(t *testing.T, eventID, listingID string, stock int32) []byte {
	t.Helper()
	return listingChanged(t, eventID, listingID, 0, stock, listingv1.ChangeType_CHANGE_TYPE_UPDATED)
}

func snapshot(t *testing.T, eventID, listingID, seller string, stock int32, ct listingv1.ChangeType) []byte {
	t.Helper()
	return envelopeOf(t, eventID, listingChangedType, &listingv1.ListingChanged{
		Listing:    &listingv1.Listing{Id: listingID, SellerId: seller, Title: lowStockListingTitle, Price: 100, Stock: stock},
		ChangeType: ct,
	})
}

func stockChanged(t *testing.T, eventID, listingID string, stock int32) []byte {
	t.Helper()
	return envelopeOf(t, eventID, listingStockChangedType, &listingv1.ListingStockChanged{ListingId: listingID, Stock: stock})
}

func lowNotifications(t *testing.T, pool *pgxpool.Pool, user string) []*notificationv1.Notification {
	t.Helper()
	var out []*notificationv1.Notification
	for _, n := range notificationsOf(t, pool, user) {
		if n.GetTitle() == lowStockTitle {
			out = append(out, n)
		}
	}
	return out
}

func newListing(pool *pgxpool.Pool) consume.Handler {
	return events.NewListingHandler(pool, defaultThreshold, nil)
}

func TestPriceDrop_MatchingSubscription_CreatesOneNotification(t *testing.T) {
	pool := newDB(t)
	subscribe(t, pool, "user_a", "listing_1", priceDrop)
	h := newListing(pool)

	mustDeliver(t, h, priceEvent(t, "evt_1", "listing_1", 100))
	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("after seed want 0, got %d", got)
	}
	mustDeliver(t, h, priceEvent(t, "evt_2", "listing_1", 80))

	n := one(t, pool, "user_a")
	if n.GetType() != notificationv1.NotificationType_NOTIFICATION_TYPE_PRICE_DROP {
		t.Fatalf("type = %v, want PRICE_DROP", n.GetType())
	}
}

func TestPriceDrop_NoSubscription_CreatesNone(t *testing.T) {
	pool := newDB(t)
	h := newListing(pool)

	mustDeliver(t, h, priceEvent(t, "evt_1", "listing_1", 100))
	mustDeliver(t, h, priceEvent(t, "evt_2", "listing_1", 80))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestPriceFirstSeen_CreatesNone(t *testing.T) {
	pool := newDB(t)
	subscribe(t, pool, "user_a", "listing_1", priceDrop)
	h := newListing(pool)

	mustDeliver(t, h, priceEvent(t, "evt_1", "listing_1", 80))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("first-seen price should not fire; got %d", got)
	}
	if price, known, err := store.NewListingPrices(pool).Get(context.Background(), "listing_1"); err != nil || !known || price != 80 {
		t.Fatalf("first sight must record the baseline: price=%d known=%v err=%v", price, known, err)
	}
}

func TestPriceIncrease_CreatesNone(t *testing.T) {
	pool := newDB(t)
	subscribe(t, pool, "user_a", "listing_1", priceDrop)
	h := newListing(pool)

	mustDeliver(t, h, priceEvent(t, "evt_1", "listing_1", 100))
	mustDeliver(t, h, priceEvent(t, "evt_2", "listing_1", 100))
	mustDeliver(t, h, priceEvent(t, "evt_3", "listing_1", 120))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestBackInStock_FiresOnlyOnZeroToPositive(t *testing.T) {
	pool := newDB(t)
	subscribe(t, pool, "user_a", "listing_1", backInStock)
	h := newListing(pool)

	mustDeliver(t, h, stockEvent(t, "evt_1", "listing_1", 0))
	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("after seed-zero want 0, got %d", got)
	}
	mustDeliver(t, h, stockEvent(t, "evt_2", "listing_1", 5))
	n := one(t, pool, "user_a")
	if n.GetType() != notificationv1.NotificationType_NOTIFICATION_TYPE_BACK_IN_STOCK {
		t.Fatalf("type = %v, want BACK_IN_STOCK", n.GetType())
	}
	mustDeliver(t, h, stockEvent(t, "evt_3", "listing_1", 3))
	if got := totalNotifications(t, pool); got != 1 {
		t.Fatalf("after 5->3 want still 1, got %d", got)
	}
}

func TestBackInStock_UnknownPrior_DoesNotFire(t *testing.T) {
	pool := newDB(t)
	subscribe(t, pool, "user_a", "listing_1", backInStock)
	h := newListing(pool)

	mustDeliver(t, h, stockEvent(t, "evt_1", "listing_1", 7))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("unknown-prior positive should not fire; got %d", got)
	}
}

func TestNonListingEvent_Ignored(t *testing.T) {
	pool := newDB(t)
	h := newListing(pool)

	mustDeliver(t, h, envelope(t, "evt_x", "platform.payment.v1.PaymentSettled", []byte("whatever")))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestMalformedEnvelope_IsPermanent(t *testing.T) {
	h := newListing(newDB(t))
	requirePermanent(t, deliver(h, []byte("not-a-protobuf-envelope-\xff\xfe")))
}

func TestMissingEventID_IsPermanent(t *testing.T) {
	h := newListing(newDB(t))
	requirePermanent(t, deliver(h, listingChanged(t, "", "listing_1", 80, 0, listingv1.ChangeType_CHANGE_TYPE_UPDATED)))
}

func TestPriceDropDetectedAfterRestart(t *testing.T) {
	pool := newDB(t)
	subscribe(t, pool, "user_a", "listing_1", priceDrop)

	mustDeliver(t, newListing(pool), priceEvent(t, "evt_1", "listing_1", 100))
	mustDeliver(t, newListing(pool), priceEvent(t, "evt_2", "listing_1", 80))

	one(t, pool, "user_a")
}

func TestRestockDetectedAfterRestart(t *testing.T) {
	pool := newDB(t)
	subscribe(t, pool, "user_a", "listing_1", backInStock)

	mustDeliver(t, newListing(pool), stockEvent(t, "evt_1", "listing_1", 0))
	mustDeliver(t, newListing(pool), stockEvent(t, "evt_2", "listing_1", 4))

	one(t, pool, "user_a")
}

func TestRedeliveredEventAfterRestartNotifiesOnce(t *testing.T) {
	pool := newDB(t)
	subscribe(t, pool, "user_a", "listing_1", priceDrop)
	mustDeliver(t, newListing(pool), priceEvent(t, "evt_seed", "listing_1", 100))
	raw := priceEvent(t, "evt_dup", "listing_1", 80)

	mustDeliver(t, newListing(pool), raw)
	mustDeliver(t, newListing(pool), raw)
	mustDeliver(t, newListing(pool), raw)

	one(t, pool, "user_a")
}

func TestStateReadFailureIsTransient(t *testing.T) {
	pool := newDB(t)
	subscribe(t, pool, "user_a", "listing_1", priceDrop)
	h := newListing(pool)
	mustDeliver(t, h, priceEvent(t, "evt_0", "listing_0", 100))
	if _, err := pool.Exec(context.Background(), `DROP TABLE listing_last_seen`); err != nil {
		t.Fatalf("drop state table: %v", err)
	}

	requireTransient(t, deliver(h, priceEvent(t, "evt_1", "listing_1", 100)))
	if got := ledgerRows(t, pool, "evt_1"); got != 0 {
		t.Fatalf("a failed event must not be recorded, ledger rows = %d", got)
	}
}

func TestListingAlertsIgnorePrefs(t *testing.T) {
	cases := []struct {
		name  string
		alert notificationv1.AlertType
		typ   notificationv1.NotificationType
		seed  func(t *testing.T) []byte
		fire  func(t *testing.T) []byte
	}{
		{"price drop", priceDrop, notificationv1.NotificationType_NOTIFICATION_TYPE_PRICE_DROP,
			func(t *testing.T) []byte { return priceEvent(t, "evt_1", "listing_1", 100) },
			func(t *testing.T) []byte { return priceEvent(t, "evt_2", "listing_1", 80) }},
		{"back in stock", backInStock, notificationv1.NotificationType_NOTIFICATION_TYPE_BACK_IN_STOCK,
			func(t *testing.T) []byte { return stockEvent(t, "evt_1", "listing_1", 0) },
			func(t *testing.T) []byte { return stockEvent(t, "evt_2", "listing_1", 5) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pool := newDB(t)
			subscribe(t, pool, "user_a", "listing_1", c.alert)
			disable(t, pool, "user_a", c.typ)
			h := newListing(pool)

			mustDeliver(t, h, c.seed(t))
			mustDeliver(t, h, c.fire(t))

			one(t, pool, "user_a")
		})
	}
}

func TestLowStockCrossingNotifiesSeller(t *testing.T) {
	pool := newDB(t)
	h := newListing(pool)

	mustDeliver(t, h, snapshot(t, "e1", "L1", "S1", 8, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	mustDeliver(t, h, stockChanged(t, "e2", "L1", 5))

	got := lowNotifications(t, pool, "S1")
	if len(got) != 1 {
		t.Fatalf("want exactly 1 low-stock notification for S1, got %d", len(got))
	}
	n := got[0]
	if n.GetLinkUrl() != "/seller/L1/edit" {
		t.Errorf("link_url = %q", n.GetLinkUrl())
	}
	if n.GetType() != notificationv1.NotificationType_NOTIFICATION_TYPE_SYSTEM {
		t.Errorf("type = %v, want SYSTEM", n.GetType())
	}
	if b := n.GetBody(); !strings.Contains(b, lowStockListingTitle) || !strings.Contains(b, "5") {
		t.Errorf("body %q must name the title and the stock 5", b)
	}
}

func TestLowStockNoRepeatWhileLow(t *testing.T) {
	pool := newDB(t)
	h := newListing(pool)

	mustDeliver(t, h, snapshot(t, "e1", "L1", "S1", 8, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	mustDeliver(t, h, stockChanged(t, "e2", "L1", 5))
	mustDeliver(t, h, stockChanged(t, "e3", "L1", 4))
	mustDeliver(t, h, stockChanged(t, "e4", "L1", 2))

	if got := len(lowNotifications(t, pool, "S1")); got != 1 {
		t.Fatalf("want 1 low-stock notification, got %d", got)
	}
}

func TestLowStockRearmsAfterRiseAboveThreshold(t *testing.T) {
	pool := newDB(t)
	h := newListing(pool)

	mustDeliver(t, h, snapshot(t, "e1", "L1", "S1", 8, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	mustDeliver(t, h, stockChanged(t, "e2", "L1", 5))
	mustDeliver(t, h, stockChanged(t, "e3", "L1", 6))
	if got := len(lowNotifications(t, pool, "S1")); got != 1 {
		t.Fatalf("rise must not notify: got %d", got)
	}
	mustDeliver(t, h, stockChanged(t, "e4", "L1", 3))
	if got := len(lowNotifications(t, pool, "S1")); got != 2 {
		t.Fatalf("want 2 low-stock notifications, got %d", got)
	}
}

func TestLowStockAboveThresholdCreatesNothing(t *testing.T) {
	pool := newDB(t)
	h := newListing(pool)

	mustDeliver(t, h, snapshot(t, "e1", "L1", "S1", 20, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	mustDeliver(t, h, stockChanged(t, "e2", "L1", 12))
	mustDeliver(t, h, stockChanged(t, "e3", "L1", 6))

	if got := len(lowNotifications(t, pool, "S1")); got != 0 {
		t.Fatalf("want 0 low-stock notifications, got %d", got)
	}
}

func TestLowStockSeedLowDoesNotNotify(t *testing.T) {
	pool := newDB(t)
	h := newListing(pool)

	mustDeliver(t, h, snapshot(t, "e1", "L1", "S1", 3, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	mustDeliver(t, h, stockChanged(t, "e2", "L1", 2))

	if got := len(lowNotifications(t, pool, "S1")); got != 0 {
		t.Fatalf("seed low must not notify, got %d", got)
	}
}

func TestLowStockRearmsOnListingChangedRestock(t *testing.T) {
	pool := newDB(t)
	h := newListing(pool)

	mustDeliver(t, h, snapshot(t, "e1", "L1", "S1", 8, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	mustDeliver(t, h, stockChanged(t, "e2", "L1", 3))
	mustDeliver(t, h, snapshot(t, "e3", "L1", "S1", 20, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	if got := len(lowNotifications(t, pool, "S1")); got != 1 {
		t.Fatalf("restock snapshot must not notify, got %d", got)
	}
	if _, _, low, known, err := store.NewLowStock(pool).Get(context.Background(), "L1"); err != nil || !known || low {
		t.Fatalf("restock must clear low: low=%v known=%v err=%v", low, known, err)
	}
	mustDeliver(t, h, stockChanged(t, "e4", "L1", 4))
	if got := len(lowNotifications(t, pool, "S1")); got != 2 {
		t.Fatalf("want 2 low-stock notifications, got %d", got)
	}
}

func TestLowStockDeletedListingDropsState(t *testing.T) {
	pool := newDB(t)
	h := newListing(pool)

	mustDeliver(t, h, snapshot(t, "e1", "L1", "S1", 8, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	mustDeliver(t, h, snapshot(t, "e_del", "L1", "S1", 0, listingv1.ChangeType_CHANGE_TYPE_DELETED))
	if _, _, _, known, err := store.NewLowStock(pool).Get(context.Background(), "L1"); err != nil || known {
		t.Fatalf("state must be gone: known=%v err=%v", known, err)
	}
	mustDeliver(t, h, stockChanged(t, "e2", "L1", 1))
	if got := len(lowNotifications(t, pool, "S1")); got != 0 {
		t.Fatalf("want no notification, got %d", got)
	}
	if _, _, _, known, _ := store.NewLowStock(pool).Get(context.Background(), "L1"); known {
		t.Fatal("stock event must not recreate state")
	}
}

func TestLowStockUnknownListingIgnored(t *testing.T) {
	pool := newDB(t)
	h := newListing(pool)

	mustDeliver(t, h, stockChanged(t, "e1", "GHOST", 1))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
	if _, _, _, known, err := store.NewLowStock(pool).Get(context.Background(), "GHOST"); err != nil || known {
		t.Fatalf("nothing may be stored: known=%v err=%v", known, err)
	}
}

func TestLowStockFailureRedetectsOnRedelivery(t *testing.T) {
	pool := newDB(t)
	h := newListing(pool)
	mustDeliver(t, h, snapshot(t, "e1", "L1", "S1", 8, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	raw := stockChanged(t, "evt_cross", "L1", 5)

	failInserts(t, pool)
	requireTransient(t, deliver(h, raw))
	if _, _, low, known, _ := store.NewLowStock(pool).Get(context.Background(), "L1"); !known || low {
		t.Fatalf("low must stay false after failure: low=%v known=%v", low, known)
	}

	healInserts(t, pool)
	mustDeliver(t, h, raw)
	if got := len(lowNotifications(t, pool, "S1")); got != 1 {
		t.Fatalf("redelivery must create exactly 1, got %d", got)
	}
}

func TestLowStockStateSurvivesRestart(t *testing.T) {
	pool := newDB(t)
	mustDeliver(t, newListing(pool), snapshot(t, "e1", "L1", "S1", 8, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	mustDeliver(t, newListing(pool), stockChanged(t, "e2", "L1", 4))
	mustDeliver(t, newListing(pool), stockChanged(t, "e3", "L1", 3))
	if got := len(lowNotifications(t, pool, "S1")); got != 1 {
		t.Fatalf("a restart must not re-notify while still low, got %d", got)
	}
	mustDeliver(t, newListing(pool), stockChanged(t, "e4", "L1", 9))
	mustDeliver(t, newListing(pool), stockChanged(t, "e5", "L1", 2))
	if got := len(lowNotifications(t, pool, "S1")); got != 2 {
		t.Fatalf("a restart must keep the re-arm state, got %d", got)
	}
}

func TestLowStockOnlyOwnerNotified(t *testing.T) {
	pool := newDB(t)
	h := newListing(pool)

	mustDeliver(t, h, snapshot(t, "e1", "L1", "S1", 8, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	mustDeliver(t, h, snapshot(t, "e2", "L2", "S2", 30, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	mustDeliver(t, h, stockChanged(t, "e3", "L1", 4))
	mustDeliver(t, h, stockChanged(t, "e4", "L2", 25))

	if got := len(lowNotifications(t, pool, "S1")); got != 1 {
		t.Fatalf("S1 want 1, got %d", got)
	}
	if got := len(lowNotifications(t, pool, "S2")); got != 0 {
		t.Fatalf("S2 want 0, got %d", got)
	}
}

func TestLowStockConfiguredThreshold(t *testing.T) {
	pool := newDB(t)
	h := events.NewListingHandler(pool, 10, nil)

	mustDeliver(t, h, snapshot(t, "e1", "L1", "S1", 12, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	mustDeliver(t, h, stockChanged(t, "e2", "L1", 10))

	if got := len(lowNotifications(t, pool, "S1")); got != 1 {
		t.Fatalf("want 1 at configured threshold 10, got %d", got)
	}
}
