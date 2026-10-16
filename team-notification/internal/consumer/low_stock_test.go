package consumer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	listingv1 "github.com/buidangphuc/team-notification/generated/platform/listing/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

const (
	lowStockTitle      = "Sắp hết hàng"
	stockChangedType   = "platform.listing.v1.ListingStockChanged"
	lowStockListingTtl = "Giày chạy bộ"
)

// snapshotEnv is a full ListingChanged snapshot carrying owner, title and stock.
func snapshotEnv(t *testing.T, eventID, listingID, seller, title string, stock int32, ct listingv1.ChangeType) []byte {
	t.Helper()
	payload, err := proto.Marshal(&listingv1.ListingChanged{
		Listing:    &listingv1.Listing{Id: listingID, SellerId: seller, Title: title, Price: 100, Stock: stock},
		ChangeType: ct,
	})
	if err != nil {
		t.Fatal(err)
	}
	return envelope(t, eventID, listingChangedType, payload)
}

func stockChangedEnv(t *testing.T, eventID, listingID string, stock int32) []byte {
	t.Helper()
	payload, err := proto.Marshal(&listingv1.ListingStockChanged{ListingId: listingID, Stock: stock})
	if err != nil {
		t.Fatal(err)
	}
	return envelope(t, eventID, stockChangedType, payload)
}

func lowNotifs(n *fakeNotif, user string) []*notificationv1.Notification {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []*notificationv1.Notification
	for _, x := range n.created {
		if x.GetUserId() == user && x.GetTitle() == lowStockTitle {
			out = append(out, x)
		}
	}
	return out
}

func lowTotal(n *fakeNotif) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	c := 0
	for _, x := range n.created {
		if x.GetTitle() == lowStockTitle {
			c++
		}
	}
	return c
}

type lowHarness struct {
	t     *testing.T
	c     *ListingConsumer
	notif *fakeNotif
	store LowStockStateStore
	seq   int
}

func newLowHarness(t *testing.T, opts ...Option) *lowHarness {
	t.Helper()
	h := &lowHarness{t: t, notif: &fakeNotif{}, store: NewInMemoryLowStockStore()}
	all := append([]Option{WithLowStockStore(h.store)}, opts...)
	h.c = NewListingConsumer(h.notif, &fakeSubs{}, nil, all...)
	return h
}

func (h *lowHarness) eid() string {
	h.seq++
	return "evt_" + string(rune('a'+h.seq))
}

func (h *lowHarness) snapshot(listing, seller string, stock int32) {
	h.t.Helper()
	if err := h.c.HandleRaw(context.Background(), snapshotEnv(h.t, h.eid(), listing, seller, lowStockListingTtl, stock, listingv1.ChangeType_CHANGE_TYPE_UPDATED)); err != nil {
		h.t.Fatalf("snapshot: %v", err)
	}
}

func (h *lowHarness) stock(listing string, stock int32) {
	h.t.Helper()
	if err := h.c.HandleRaw(context.Background(), stockChangedEnv(h.t, h.eid(), listing, stock)); err != nil {
		h.t.Fatalf("stock event: %v", err)
	}
}

// Scenario: Order work: Stock falling to the threshold notifies the seller once
func TestLowStockCrossingNotifiesSeller(t *testing.T) {
	h := newLowHarness(t)
	h.snapshot("L1", "S1", 8)
	h.stock("L1", 5)
	got := lowNotifs(h.notif, "S1")
	if len(got) != 1 {
		t.Fatalf("want exactly 1 low-stock notification for S1, got %d", len(got))
	}
	n := got[0]
	if n.GetLinkUrl() != "/seller/L1/edit" {
		t.Fatalf("link_url = %q", n.GetLinkUrl())
	}
	if n.GetType() != notificationv1.NotificationType_NOTIFICATION_TYPE_SYSTEM {
		t.Fatalf("type = %v, want SYSTEM", n.GetType())
	}
	if !strings.Contains(n.GetBody(), lowStockListingTtl) || !strings.Contains(n.GetBody(), "5") {
		t.Fatalf("body %q must name the title and the stock 5", n.GetBody())
	}
}

// Scenario: Order work: Stock staying at or below the threshold does not repeat
func TestLowStockNoRepeatWhileLow(t *testing.T) {
	h := newLowHarness(t)
	h.snapshot("L1", "S1", 8)
	h.stock("L1", 5)
	h.stock("L1", 4)
	h.stock("L1", 2)
	if got := len(lowNotifs(h.notif, "S1")); got != 1 {
		t.Fatalf("want 1 low-stock notification, got %d", got)
	}
}

// Scenario: Order work: A fall after a rise above the threshold notifies again
func TestLowStockRearmsAfterRiseAboveThreshold(t *testing.T) {
	h := newLowHarness(t)
	h.snapshot("L1", "S1", 8)
	h.stock("L1", 5)
	h.stock("L1", 6)
	if got := len(lowNotifs(h.notif, "S1")); got != 1 {
		t.Fatalf("rise must not notify: got %d", got)
	}
	h.stock("L1", 3)
	if got := len(lowNotifs(h.notif, "S1")); got != 2 {
		t.Fatalf("want 2 low-stock notifications, got %d", got)
	}
}

// Scenario: Order work: Stock above the threshold creates nothing
func TestLowStockAboveThresholdCreatesNothing(t *testing.T) {
	h := newLowHarness(t)
	h.snapshot("L1", "S1", 20)
	h.stock("L1", 12)
	h.stock("L1", 6) // boundary: threshold + 1
	if got := lowTotal(h.notif); got != 0 {
		t.Fatalf("want 0 low-stock notifications, got %d", got)
	}
}

// Scenario: Order work: A listing seen first with low stock does not notify
func TestLowStockSeedLowDoesNotNotify(t *testing.T) {
	h := newLowHarness(t)
	h.snapshot("L1", "S1", 3)
	if got := lowTotal(h.notif); got != 0 {
		t.Fatalf("seed snapshot notified: %d", got)
	}
	h.stock("L1", 2)
	if got := lowTotal(h.notif); got != 0 {
		t.Fatalf("stock event after low seed notified: %d", got)
	}
}

// Scenario: Order work: A restock through a listing edit re-arms low stock
func TestLowStockRearmsOnListingChangedRestock(t *testing.T) {
	h := newLowHarness(t)
	h.snapshot("L1", "S1", 8)
	h.stock("L1", 3)
	if got := len(lowNotifs(h.notif, "S1")); got != 1 {
		t.Fatalf("after crossing want 1, got %d", got)
	}
	h.snapshot("L1", "S1", 20)
	if got := len(lowNotifs(h.notif, "S1")); got != 1 {
		t.Fatalf("restock snapshot must not notify, got %d", got)
	}
	if _, _, low, known, err := h.store.Get(context.Background(), "L1"); err != nil || !known || low {
		t.Fatalf("restock must clear low: low=%v known=%v err=%v", low, known, err)
	}
	h.stock("L1", 4)
	if got := len(lowNotifs(h.notif, "S1")); got != 2 {
		t.Fatalf("want 2 low-stock notifications, got %d", got)
	}
}

// Scenario: Order work: A deleted listing drops its low-stock state
func TestLowStockDeletedListingDropsState(t *testing.T) {
	h := newLowHarness(t)
	h.snapshot("L1", "S1", 8)
	if err := h.c.HandleRaw(context.Background(), snapshotEnv(t, "evt_del", "L1", "S1", lowStockListingTtl, 0, listingv1.ChangeType_CHANGE_TYPE_DELETED)); err != nil {
		t.Fatalf("DELETED: %v", err)
	}
	if _, _, _, known, err := h.store.Get(context.Background(), "L1"); err != nil || known {
		t.Fatalf("state must be gone: known=%v err=%v", known, err)
	}
	h.stock("L1", 1)
	if got := lowTotal(h.notif); got != 0 {
		t.Fatalf("want no notification, got %d", got)
	}
	if _, _, _, known, _ := h.store.Get(context.Background(), "L1"); known {
		t.Fatal("stock event must not recreate state")
	}
}

// Scenario: Order work: A stock event for an unknown listing is ignored
func TestLowStockUnknownListingIgnored(t *testing.T) {
	h := newLowHarness(t)
	h.stock("GHOST", 1)
	if got := lowTotal(h.notif); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
	if _, _, _, known, err := h.store.Get(context.Background(), "GHOST"); err != nil || known {
		t.Fatalf("nothing may be stored: known=%v err=%v", known, err)
	}
}

// Scenario: Order work: A failed low-stock notification re-detects on redelivery
func TestLowStockFailureRedetectsOnRedelivery(t *testing.T) {
	h := newLowHarness(t)
	h.snapshot("L1", "S1", 8)
	raw := stockChangedEnv(t, "evt_cross", "L1", 5)

	h.notif.err = errors.New("db down")
	err := h.c.HandleRaw(context.Background(), raw)
	if err == nil {
		t.Fatal("want an error when the notification cannot be created")
	}
	if errors.Is(err, ErrPermanent) {
		t.Fatalf("error must be transient, got %v", err)
	}
	if _, _, low, known, _ := h.store.Get(context.Background(), "L1"); !known || low {
		t.Fatalf("low must stay false after failure: low=%v known=%v", low, known)
	}

	h.notif.err = nil
	if err := h.c.HandleRaw(context.Background(), raw); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if got := len(lowNotifs(h.notif, "S1")); got != 1 {
		t.Fatalf("redelivery must create exactly 1, got %d", got)
	}
}

// Scenario: Order work: The notification goes to the listing's owner only
func TestLowStockOnlyOwnerNotified(t *testing.T) {
	h := newLowHarness(t)
	h.snapshot("L1", "S1", 8)
	h.snapshot("L2", "S2", 30)
	h.stock("L1", 4)
	h.stock("L2", 25)
	if got := len(lowNotifs(h.notif, "S1")); got != 1 {
		t.Fatalf("S1 want 1, got %d", got)
	}
	if got := len(lowNotifs(h.notif, "S2")); got != 0 {
		t.Fatalf("S2 want 0, got %d", got)
	}
}

// Scenario: Order work: The threshold is configurable
func TestLowStockConfiguredThreshold(t *testing.T) {
	h := newLowHarness(t, WithLowStockThreshold(10))
	h.snapshot("L1", "S1", 12)
	h.stock("L1", 10)
	if got := len(lowNotifs(h.notif, "S1")); got != 1 {
		t.Fatalf("want 1 at configured threshold 10, got %d", got)
	}
}
