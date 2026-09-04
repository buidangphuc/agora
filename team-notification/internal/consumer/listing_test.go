package consumer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-notification/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-notification/generated/platform/listing/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

// ── test doubles ──

type fakeNotif struct {
	mu      sync.Mutex
	created []*notificationv1.Notification
	err     error
}

func (f *fakeNotif) CreateNotification(_ context.Context, n *notificationv1.Notification) (*notificationv1.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.created = append(f.created, n)
	return n, nil
}

func (f *fakeNotif) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.created)
}

// fakeSubs returns configured subscriptions per (listingID, alertType).
type fakeSubs struct {
	byKey map[string][]*notificationv1.AlertSubscription
	err   error
}

func subKey(listingID string, at notificationv1.AlertType) string {
	return listingID + "|" + at.String()
}

func (f *fakeSubs) ListByListingAndType(_ context.Context, listingID string, at notificationv1.AlertType) ([]*notificationv1.AlertSubscription, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byKey[subKey(listingID, at)], nil
}

func sub(user, listing string, at notificationv1.AlertType) *notificationv1.AlertSubscription {
	return &notificationv1.AlertSubscription{Id: "alert_" + user, UserId: user, ListingId: listing, Type: at}
}

// ── envelope fabrication ──

// listingChangedEnvelope fabricates the ONE event team-domain actually emits: a
// full listing snapshot (price + stock) plus a change_type, wrapped in an
// EventEnvelope of type platform.listing.v1.ListingChanged.
func listingChangedEnvelope(t *testing.T, eventID, listingID string, price int64, stock int32, ct listingv1.ChangeType) []byte {
	t.Helper()
	payload, err := proto.Marshal(&listingv1.ListingChanged{
		Listing:    &listingv1.Listing{Id: listingID, Price: price, Stock: stock},
		ChangeType: ct,
	})
	if err != nil {
		t.Fatalf("marshal ListingChanged payload: %v", err)
	}
	return envelope(t, eventID, listingChangedType, payload)
}

// priceEnvelope is a snapshot that only exercises the price dimension (stock 0).
func priceEnvelope(t *testing.T, eventID, listingID string, price int64) []byte {
	t.Helper()
	return listingChangedEnvelope(t, eventID, listingID, price, 0, listingv1.ChangeType_CHANGE_TYPE_UPDATED)
}

// stockEnvelope is a snapshot that only exercises the stock dimension (price 0).
func stockEnvelope(t *testing.T, eventID, listingID string, stock int32) []byte {
	t.Helper()
	return listingChangedEnvelope(t, eventID, listingID, 0, stock, listingv1.ChangeType_CHANGE_TYPE_UPDATED)
}

func envelope(t *testing.T, eventID, typ string, payload []byte) []byte {
	t.Helper()
	raw, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: typ, Payload: payload})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return raw
}

func newConsumer(notif NotificationCreator, subs SubscriptionStore) *ListingConsumer {
	return NewListingConsumer(notif, subs, nil)
}

// ── tests ──

func TestPriceDrop_MatchingSubscription_CreatesOneNotification(t *testing.T) {
	notif := &fakeNotif{}
	subs := &fakeSubs{byKey: map[string][]*notificationv1.AlertSubscription{
		subKey("listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP): {
			sub("user_a", "listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP),
		},
	}}
	c := newConsumer(notif, subs)
	ctx := context.Background()

	// Seed the last-seen price at 100 (first-seen never fires).
	if err := c.HandleRaw(ctx, priceEnvelope(t, "evt_1", "listing_1", 100)); err != nil {
		t.Fatalf("HandleRaw seed: %v", err)
	}
	if got := notif.count(); got != 0 {
		t.Fatalf("after seed want 0, got %d", got)
	}

	// Snapshot at 80 (< last-seen 100) → exactly one price-drop notification.
	if err := c.HandleRaw(ctx, priceEnvelope(t, "evt_2", "listing_1", 80)); err != nil {
		t.Fatalf("HandleRaw drop: %v", err)
	}
	if got := notif.count(); got != 1 {
		t.Fatalf("want 1 notification, got %d", got)
	}
	if notif.created[0].Type != notificationv1.NotificationType_NOTIFICATION_TYPE_PRICE_DROP {
		t.Fatalf("want PRICE_DROP notification type, got %v", notif.created[0].Type)
	}
	if notif.created[0].UserId != "user_a" {
		t.Fatalf("want user_a, got %q", notif.created[0].UserId)
	}
}

func TestPriceDrop_NoSubscription_CreatesNone(t *testing.T) {
	notif := &fakeNotif{}
	subs := &fakeSubs{byKey: map[string][]*notificationv1.AlertSubscription{}}
	c := newConsumer(notif, subs)
	ctx := context.Background()

	if err := c.HandleRaw(ctx, priceEnvelope(t, "evt_1", "listing_1", 100)); err != nil {
		t.Fatalf("HandleRaw seed: %v", err)
	}
	// A real drop, but no subscribers → still zero notifications.
	if err := c.HandleRaw(ctx, priceEnvelope(t, "evt_2", "listing_1", 80)); err != nil {
		t.Fatalf("HandleRaw drop: %v", err)
	}
	if got := notif.count(); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestPriceFirstSeen_CreatesNone(t *testing.T) {
	notif := &fakeNotif{}
	subs := &fakeSubs{byKey: map[string][]*notificationv1.AlertSubscription{
		subKey("listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP): {
			sub("user_a", "listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP),
		},
	}}
	c := newConsumer(notif, subs)

	// First-ever snapshot for the listing → unknown prior price, never fires.
	if err := c.HandleRaw(context.Background(), priceEnvelope(t, "evt_1", "listing_1", 80)); err != nil {
		t.Fatalf("HandleRaw: %v", err)
	}
	if got := notif.count(); got != 0 {
		t.Fatalf("first-seen price should not fire; got %d", got)
	}
}

func TestPriceIncrease_CreatesNone(t *testing.T) {
	notif := &fakeNotif{}
	subs := &fakeSubs{byKey: map[string][]*notificationv1.AlertSubscription{
		subKey("listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP): {
			sub("user_a", "listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP),
		},
	}}
	c := newConsumer(notif, subs)
	ctx := context.Background()

	if err := c.HandleRaw(ctx, priceEnvelope(t, "evt_1", "listing_1", 100)); err != nil {
		t.Fatalf("HandleRaw seed: %v", err)
	}
	// Same price (no change) → not a drop.
	if err := c.HandleRaw(ctx, priceEnvelope(t, "evt_2", "listing_1", 100)); err != nil {
		t.Fatalf("HandleRaw same: %v", err)
	}
	// Higher price (increase) → not a drop.
	if err := c.HandleRaw(ctx, priceEnvelope(t, "evt_3", "listing_1", 120)); err != nil {
		t.Fatalf("HandleRaw increase: %v", err)
	}
	if got := notif.count(); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestBackInStock_FiresOnlyOnZeroToPositive(t *testing.T) {
	notif := &fakeNotif{}
	subs := &fakeSubs{byKey: map[string][]*notificationv1.AlertSubscription{
		subKey("listing_1", notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK): {
			sub("user_a", "listing_1", notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK),
		},
	}}
	c := newConsumer(notif, subs)
	ctx := context.Background()

	// Seed the last-seen stock at 0 (this event itself is not a rise → no notify).
	if err := c.HandleRaw(ctx, stockEnvelope(t, "evt_1", "listing_1", 0)); err != nil {
		t.Fatalf("HandleRaw seed: %v", err)
	}
	if got := notif.count(); got != 0 {
		t.Fatalf("after seed-zero want 0, got %d", got)
	}

	// 0 → positive: fires exactly one back-in-stock notification.
	if err := c.HandleRaw(ctx, stockEnvelope(t, "evt_2", "listing_1", 5)); err != nil {
		t.Fatalf("HandleRaw rise: %v", err)
	}
	if got := notif.count(); got != 1 {
		t.Fatalf("after 0->5 want 1, got %d", got)
	}
	if notif.created[0].Type != notificationv1.NotificationType_NOTIFICATION_TYPE_BACK_IN_STOCK {
		t.Fatalf("want BACK_IN_STOCK type, got %v", notif.created[0].Type)
	}

	// positive → positive: no new notification.
	if err := c.HandleRaw(ctx, stockEnvelope(t, "evt_3", "listing_1", 3)); err != nil {
		t.Fatalf("HandleRaw pos->pos: %v", err)
	}
	if got := notif.count(); got != 1 {
		t.Fatalf("after 5->3 want still 1, got %d", got)
	}
}

func TestBackInStock_UnknownPrior_DoesNotFire(t *testing.T) {
	notif := &fakeNotif{}
	subs := &fakeSubs{byKey: map[string][]*notificationv1.AlertSubscription{
		subKey("listing_1", notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK): {
			sub("user_a", "listing_1", notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK),
		},
	}}
	c := newConsumer(notif, subs)

	// First-ever event for the listing arrives positive → unknown prior, no fire.
	if err := c.HandleRaw(context.Background(), stockEnvelope(t, "evt_1", "listing_1", 7)); err != nil {
		t.Fatalf("HandleRaw: %v", err)
	}
	if got := notif.count(); got != 0 {
		t.Fatalf("unknown-prior positive should not fire; got %d", got)
	}
}

func TestIdempotent_SameEventID_NotifiesOnce(t *testing.T) {
	notif := &fakeNotif{}
	subs := &fakeSubs{byKey: map[string][]*notificationv1.AlertSubscription{
		subKey("listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP): {
			sub("user_a", "listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP),
		},
	}}
	c := newConsumer(notif, subs)
	ctx := context.Background()

	// Seed the last-seen price so the drop event is a real drop.
	if err := c.HandleRaw(ctx, priceEnvelope(t, "evt_seed", "listing_1", 100)); err != nil {
		t.Fatalf("HandleRaw seed: %v", err)
	}
	raw := priceEnvelope(t, "evt_dup", "listing_1", 80)

	for i := 0; i < 3; i++ {
		if err := c.HandleRaw(ctx, raw); err != nil {
			t.Fatalf("HandleRaw #%d: %v", i, err)
		}
	}
	if got := notif.count(); got != 1 {
		t.Fatalf("want exactly 1 notification for repeated event_id, got %d", got)
	}
}

func TestNonListingEvent_Ignored(t *testing.T) {
	notif := &fakeNotif{}
	subs := &fakeSubs{byKey: map[string][]*notificationv1.AlertSubscription{}}
	c := newConsumer(notif, subs)

	raw := envelope(t, "evt_x", "platform.payment.v1.PaymentSettled", []byte("whatever"))
	if err := c.HandleRaw(context.Background(), raw); err != nil {
		t.Fatalf("non-listing event should be a no-op, got err: %v", err)
	}
	if got := notif.count(); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestMalformedEnvelope_IsPermanent(t *testing.T) {
	c := newConsumer(&fakeNotif{}, &fakeSubs{})
	err := c.HandleRaw(context.Background(), []byte("not-a-protobuf-envelope-\xff\xfe"))
	if err == nil || !errors.Is(err, ErrPermanent) {
		t.Fatalf("want ErrPermanent for malformed envelope, got %v", err)
	}
}

func TestMissingEventID_IsPermanent(t *testing.T) {
	c := newConsumer(&fakeNotif{}, &fakeSubs{})
	payload, _ := proto.Marshal(&listingv1.ListingChanged{Listing: &listingv1.Listing{Id: "listing_1", Price: 80}})
	raw, _ := proto.Marshal(&eventsv1.EventEnvelope{Type: listingChangedType, Payload: payload}) // no event_id
	err := c.HandleRaw(context.Background(), raw)
	if err == nil || !errors.Is(err, ErrPermanent) {
		t.Fatalf("want ErrPermanent for missing event_id, got %v", err)
	}
}

// ── transport loop: malformed record is DLQ'd, committed, no crash ──

type fakeReader struct {
	recs   []Record
	idx    int
	commit []Record
}

func (r *fakeReader) Fetch(ctx context.Context) (Record, error) {
	if r.idx >= len(r.recs) {
		<-ctx.Done() // block until cancelled once drained
		return Record{}, ctx.Err()
	}
	rec := r.recs[r.idx]
	r.idx++
	return rec, nil
}

func (r *fakeReader) Commit(_ context.Context, rec Record) error {
	r.commit = append(r.commit, rec)
	return nil
}

type fakeDLQ struct {
	produced []Record
	topic    string
}

func (d *fakeDLQ) Produce(_ context.Context, topic string, rec Record) error {
	d.topic = topic
	d.produced = append(d.produced, rec)
	return nil
}

func TestRun_MalformedRecord_RoutedToDLQAndCommitted(t *testing.T) {
	notif := &fakeNotif{}
	subs := &fakeSubs{byKey: map[string][]*notificationv1.AlertSubscription{
		subKey("listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP): {
			sub("user_a", "listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP),
		},
	}}
	c := newConsumer(notif, subs)

	// Seed the last-seen price so the "good" record (price 80) reads as a drop.
	if err := c.HandleRaw(context.Background(), priceEnvelope(t, "evt_seed", "listing_1", 100)); err != nil {
		t.Fatalf("HandleRaw seed: %v", err)
	}

	good := Record{Key: "k1", Value: priceEnvelope(t, "evt_ok", "listing_1", 80)}
	poison := Record{Key: "k2", Value: []byte("\xff\xff not a protobuf")}
	reader := &fakeReader{recs: []Record{poison, good}}
	dlq := &fakeDLQ{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, reader, dlq, RunConfig{DLQTopic: "listing.events.dlq", MaxAttempts: 1}) }()

	// Both records should be handled, then Fetch blocks; cancel to end the loop.
	waitFor(t, func() bool { return len(reader.commit) == 2 })
	cancel()
	<-done

	if len(dlq.produced) != 1 {
		t.Fatalf("want 1 DLQ record, got %d", len(dlq.produced))
	}
	if dlq.topic != "listing.events.dlq" {
		t.Fatalf("want DLQ topic listing.events.dlq, got %q", dlq.topic)
	}
	if len(reader.commit) != 2 {
		t.Fatalf("want both offsets committed (poison DLQ'd + good applied), got %d", len(reader.commit))
	}
	if notif.count() != 1 {
		t.Fatalf("want the good record to produce 1 notification, got %d", notif.count())
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if !cond() {
		t.Fatalf("condition not met in time")
	}
}
