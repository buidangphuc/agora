package consumer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-domain/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/events"
	"github.com/buidangphuc/team-domain/internal/repository"
)

// Contract the implementation must provide (package consumer):
//
//	type SellerStore interface {
//	    HoldSellerListings(ctx, userID, eventID string, changedAt time.Time, enqueue repository.EnqueueFn) (int, error)
//	    RestoreSellerListings(ctx, userID, eventID string, changedAt time.Time, enqueue repository.EnqueueFn) (int, error)
//	}
//	func NewIdentityConsumer(store SellerStore, logger *slog.Logger) *IdentityConsumer
//	func (*IdentityConsumer) Handle(ctx, key, value []byte) error   // consumer.Handler
//	var ErrPermanent error // processRecord parks wrapped ErrPermanent without retrying

var (
	t1 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	t2 = t1.Add(time.Hour)
)

// fakeSellerStore mirrors the contract of repository.PgTxWriter.{Hold,Restore}SellerListings
// (dedupe by event id, stale-changed_at skip, status filters, one outbox row per change).
// It is the true external boundary (the database) for the consumer.
type fakeSellerStore struct {
	mu        sync.Mutex
	listings  map[string]*repository.Listing // id -> listing
	prior     map[string]string              // id -> held_prior_status
	processed map[string]bool
	applied   map[string]time.Time
	outbox    []repository.OutboxRow
	calls     int
	failErr   error
}

func newFakeStore() *fakeSellerStore {
	return &fakeSellerStore{
		listings: map[string]*repository.Listing{}, prior: map[string]string{},
		processed: map[string]bool{}, applied: map[string]time.Time{},
	}
}

func (f *fakeSellerStore) seed(id, seller, status string) {
	f.listings[id] = &repository.Listing{ID: id, SellerID: seller, Status: status}
}

func (f *fakeSellerStore) status(id string) string { return f.listings[id].Status }

func (f *fakeSellerStore) apply(userID, eventID string, at time.Time, enq repository.EnqueueFn, hold bool) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.failErr != nil {
		return 0, f.failErr
	}
	if f.processed[eventID] {
		return 0, nil
	}
	f.processed[eventID] = true
	if prev, ok := f.applied[userID]; ok && !prev.Before(at) {
		return 0, nil
	}
	f.applied[userID] = at
	n := 0
	for _, l := range f.listings {
		if l.SellerID != userID {
			continue
		}
		if hold && (l.Status == "published" || l.Status == "pending_review") {
			f.prior[l.ID], l.Status, l.ModerationReasonCode = l.Status, "held", "seller_enforcement"
		} else if !hold && l.Status == "held" && f.prior[l.ID] != "" {
			l.Status, l.ModerationReasonCode = f.prior[l.ID], ""
			delete(f.prior, l.ID)
		} else {
			continue
		}
		row, err := enq(*l)
		if err != nil {
			return 0, err
		}
		f.outbox = append(f.outbox, row)
		n++
	}
	return n, nil
}

func (f *fakeSellerStore) HoldSellerListings(_ context.Context, userID, eventID string, at time.Time, enq repository.EnqueueFn) (int, error) {
	return f.apply(userID, eventID, at, enq, true)
}

func (f *fakeSellerStore) RestoreSellerListings(_ context.Context, userID, eventID string, at time.Time, enq repository.EnqueueFn) (int, error) {
	return f.apply(userID, eventID, at, enq, false)
}

func envBytes(t *testing.T, eventID, typ string, payload proto.Message) []byte {
	t.Helper()
	var pb []byte
	if payload != nil {
		var err error
		if pb, err = proto.Marshal(payload); err != nil {
			t.Fatal(err)
		}
	}
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: typ, OccurredAt: timestamppb.New(t1), Payload: pb})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const statusChangedType = "platform.identity.v1.UserStatusChanged"

func statusEvent(t *testing.T, eventID, user string, status, prev identityv1.UserStatus, at time.Time) []byte {
	t.Helper()
	m := &identityv1.UserStatusChanged{UserId: user, Status: status, PreviousStatus: prev}
	if !at.IsZero() {
		m.ChangedAt = timestamppb.New(at)
	}
	return envBytes(t, eventID, statusChangedType, m)
}

func newHandler(store SellerStore) *IdentityConsumer {
	return NewIdentityConsumer(store, discardLogger())
}

func decodeChanged(t *testing.T, row repository.OutboxRow) (*eventsv1.EventEnvelope, *listingv1.ListingChanged) {
	t.Helper()
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(row.Payload, &env); err != nil {
		t.Fatalf("outbox payload is not an EventEnvelope: %v", err)
	}
	var lc listingv1.ListingChanged
	if err := proto.Unmarshal(env.GetPayload(), &lc); err != nil {
		t.Fatalf("envelope payload is not a ListingChanged: %v", err)
	}
	return &env, &lc
}

// Scenario "Seller hold: Ban holds the same way".
func TestUserStatusChangedBannedHolds(t *testing.T) {
	st := newFakeStore()
	st.seed("l1", "u1", "published")
	st.seed("l2", "u1", "published")
	st.seed("other", "u2", "published")
	h := newHandler(st)

	err := h.Handle(context.Background(), []byte("u1"),
		statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_BANNED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	for _, id := range []string{"l1", "l2"} {
		if st.status(id) != "held" || st.prior[id] != "published" {
			t.Errorf("%s: status=%q prior=%q, want held/published", id, st.status(id), st.prior[id])
		}
	}
	if st.status("other") != "published" {
		t.Errorf("other seller's listing changed to %q", st.status("other"))
	}
}

// Scenario "Seller hold: Suspension holds published and pending listings" (dispatch side):
// SUSPENDED also holds.
func TestUserStatusChangedSuspendedHolds(t *testing.T) {
	st := newFakeStore()
	st.seed("l1", "u1", "published")
	st.seed("l2", "u1", "pending_review")
	if err := newHandler(st).Handle(context.Background(), nil,
		statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1)); err != nil {
		t.Fatal(err)
	}
	if st.status("l1") != "held" || st.status("l2") != "held" || st.prior["l2"] != "pending_review" {
		t.Errorf("unexpected state: %v %v prior=%v", st.status("l1"), st.status("l2"), st.prior)
	}
}

// Scenario "Seller hold: Hold event carries the status for de-indexing".
func TestHoldEventCarriesHeldStatus(t *testing.T) {
	st := newFakeStore()
	st.seed("l1", "u1", "published")
	if err := newHandler(st).Handle(context.Background(), nil,
		statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1)); err != nil {
		t.Fatal(err)
	}
	if len(st.outbox) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(st.outbox))
	}
	row := st.outbox[0]
	if row.AggregateID != "l1" || row.EventType != events.ListingChangedEventType || row.EventID == "" {
		t.Errorf("row = %+v", row)
	}
	env, lc := decodeChanged(t, row)
	if env.GetType() != events.ListingChangedEventType {
		t.Errorf("envelope type = %q", env.GetType())
	}
	if env.GetEventId() != row.EventID {
		t.Errorf("envelope event_id %q != outbox event id %q", env.GetEventId(), row.EventID)
	}
	if lc.GetChangeType() != listingv1.ChangeType_CHANGE_TYPE_UPDATED {
		t.Errorf("change_type = %v", lc.GetChangeType())
	}
	l := lc.GetListing()
	if l.GetId() != "l1" || l.GetStatus() != listingv1.ListingStatus_LISTING_STATUS_HELD {
		t.Errorf("listing = %v", l)
	}
	if m := l.GetModeration(); m.GetReasonCode() != "seller_enforcement" || m.GetNote() != "" || len(m.GetScreenFlags()) != 0 {
		t.Errorf("moderation = %v, want only reason seller_enforcement", m)
	}
	p := env.GetPrincipal()
	if p.GetId() != "service-team-domain" || p.GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE {
		t.Errorf("principal = %v, want service service-team-domain", p)
	}
}

// Scenario "Seller hold: Seller with no listings is a no-op".
func TestUserStatusChangedUnknownSellerNoop(t *testing.T) {
	st := newFakeStore()
	err := newHandler(st).Handle(context.Background(), nil,
		statusEvent(t, "e1", "ghost", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(st.outbox) != 0 {
		t.Errorf("outbox rows = %d, want 0", len(st.outbox))
	}
	if !st.processed["e1"] {
		t.Error("event id not recorded as processed")
	}
}

// Scenario "Seller hold: Restricted also restores" (suspended -> restricted).
func TestUserStatusChangedRestrictedRestores(t *testing.T) {
	st := newFakeStore()
	st.seed("l1", "u1", "published")
	h := newHandler(st)
	if err := h.Handle(context.Background(), nil,
		statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1)); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), nil,
		statusEvent(t, "e2", "u1", identityv1.UserStatus_USER_STATUS_RESTRICTED, identityv1.UserStatus_USER_STATUS_SUSPENDED, t2)); err != nil {
		t.Fatal(err)
	}
	if st.status("l1") != "published" || st.prior["l1"] != "" {
		t.Errorf("l1 status=%q prior=%q, want published/empty", st.status("l1"), st.prior["l1"])
	}
	// restore emits a ListingChanged with the restored status
	last := st.outbox[len(st.outbox)-1]
	if _, lc := decodeChanged(t, last); lc.GetListing().GetStatus() != listingv1.ListingStatus_LISTING_STATUS_PUBLISHED {
		t.Errorf("restore event status = %v", lc.GetListing().GetStatus())
	}
}

// Scenario "Seller hold: Reinstatement restores prior statuses" (dispatch side): ACTIVE restores.
func TestUserStatusChangedActiveRestores(t *testing.T) {
	st := newFakeStore()
	st.seed("l1", "u1", "published")
	st.seed("l2", "u1", "pending_review")
	h := newHandler(st)
	_ = h.Handle(context.Background(), nil,
		statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_BANNED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1))
	if err := h.Handle(context.Background(), nil,
		statusEvent(t, "e2", "u1", identityv1.UserStatus_USER_STATUS_ACTIVE, identityv1.UserStatus_USER_STATUS_BANNED, t2)); err != nil {
		t.Fatal(err)
	}
	if st.status("l1") != "published" || st.status("l2") != "pending_review" {
		t.Errorf("l1=%q l2=%q", st.status("l1"), st.status("l2"))
	}
}

// Scenario "Seller hold: Restricted seller keeps listings published".
func TestUserStatusChangedRestrictedFromActiveHoldsNothing(t *testing.T) {
	st := newFakeStore()
	st.seed("l1", "u1", "published")
	st.seed("l2", "u1", "published")
	err := newHandler(st).Handle(context.Background(), nil,
		statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_RESTRICTED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1))
	if err != nil {
		t.Fatal(err)
	}
	if st.status("l1") != "published" || st.status("l2") != "published" || len(st.outbox) != 0 {
		t.Errorf("l1=%q l2=%q outbox=%d", st.status("l1"), st.status("l2"), len(st.outbox))
	}
}

// Scenario "Seller hold: Restriction of a held seller is not a new hold".
func TestUserStatusChangedRestrictedFromSuspendedRestores(t *testing.T) {
	st := newFakeStore()
	st.seed("l1", "u1", "published")
	h := newHandler(st)
	_ = h.Handle(context.Background(), nil,
		statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1))
	if st.status("l1") != "held" {
		t.Fatalf("precondition: l1 = %q", st.status("l1"))
	}
	if err := h.Handle(context.Background(), nil,
		statusEvent(t, "e2", "u1", identityv1.UserStatus_USER_STATUS_RESTRICTED, identityv1.UserStatus_USER_STATUS_SUSPENDED, t2)); err != nil {
		t.Fatal(err)
	}
	if st.status("l1") != "published" {
		t.Errorf("l1 = %q, want published (restored, not held again)", st.status("l1"))
	}
}

// Scenario "Seller hold: Redelivered event changes nothing twice".
func TestUserStatusChangedRedeliveryIsIdempotent(t *testing.T) {
	st := newFakeStore()
	st.seed("l1", "u1", "published")
	h := newHandler(st)
	ev := statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1)
	for i := 0; i < 2; i++ {
		if err := h.Handle(context.Background(), nil, ev); err != nil {
			t.Fatalf("delivery %d: %v", i+1, err)
		}
	}
	if len(st.outbox) != 1 {
		t.Errorf("outbox rows = %d, want 1", len(st.outbox))
	}
	if st.status("l1") != "held" || st.prior["l1"] != "published" {
		t.Errorf("l1 status=%q prior=%q", st.status("l1"), st.prior["l1"])
	}
}

// The handler must pass the envelope event id and payload changed_at to the store
// (they drive dedupe and the stale-event guard).
func TestUserStatusChangedPassesEventIDAndChangedAt(t *testing.T) {
	st := newFakeStore()
	st.seed("l1", "u1", "published")
	if err := newHandler(st).Handle(context.Background(), nil,
		statusEvent(t, "evt-42", "u1", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, t2)); err != nil {
		t.Fatal(err)
	}
	if !st.processed["evt-42"] {
		t.Error("envelope event_id was not the dedupe id")
	}
	if got := st.applied["u1"]; !got.Equal(t2) {
		t.Errorf("applied changed_at = %v, want %v", got, t2)
	}
}

// Scenario "Seller hold: Other event types are ignored".
func TestIdentityConsumerIgnoresOtherTypes(t *testing.T) {
	st := newFakeStore()
	st.seed("l1", "u1", "published")
	other := envBytes(t, "e9", "platform.identity.v1.SessionRevoked", &identityv1.UserStatusChanged{UserId: "u1"})
	if err := newHandler(st).Handle(context.Background(), nil, other); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if st.calls != 0 || len(st.processed) != 0 || len(st.outbox) != 0 || st.status("l1") != "published" {
		t.Errorf("calls=%d processed=%v outbox=%d status=%q", st.calls, st.processed, len(st.outbox), st.status("l1"))
	}
}

// Scenario "Seller hold: Malformed event is parked, not applied".
func TestUserStatusChangedMalformedGoesToDLQ(t *testing.T) {
	garbledPayload, _ := proto.Marshal(&eventsv1.EventEnvelope{EventId: "e1", Type: statusChangedType, Payload: []byte{0xff, 0xff, 0xff}})
	cases := map[string][]byte{
		"unparsable envelope": []byte{0xff, 0xff, 0xff, 0xff},
		"unparsable payload":  garbledPayload,
		"empty user_id":       statusEvent(t, "e1", "", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1),
		"missing changed_at":  statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, time.Time{}),
		"unspecified status":  statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_UNSPECIFIED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1),
		"unknown status 9":    statusEvent(t, "e1", "u1", identityv1.UserStatus(9), identityv1.UserStatus_USER_STATUS_ACTIVE, t1),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			st := newFakeStore()
			st.seed("l1", "u1", "published")
			dlq := &fakeDLQ{}
			c := testConsumer(dlq)
			h := newHandler(st)

			var attempts int
			counting := func(ctx context.Context, k, v []byte) error { attempts++; return h.Handle(ctx, k, v) }
			if err := c.processRecord(context.Background(), []byte("u1"), value, counting, discardLogger()); err != nil {
				t.Fatalf("processRecord: %v (record must be parked so the offset advances)", err)
			}
			if err := h.Handle(context.Background(), nil, value); !errors.Is(err, ErrPermanent) {
				t.Errorf("Handle err = %v, want wrapping ErrPermanent", err)
			}
			if attempts != 1 {
				t.Errorf("attempts = %d, want 1 (permanent error skips retries)", attempts)
			}
			if len(dlq.records) != 1 || string(dlq.records[0][1]) != string(value) {
				t.Errorf("dlq records = %d, want the original record parked", len(dlq.records))
			}
			if st.status("l1") != "published" || len(st.outbox) != 0 || st.calls != 0 {
				t.Errorf("listing touched: status=%q outbox=%d store calls=%d", st.status("l1"), len(st.outbox), st.calls)
			}
		})
	}
}

// Scenario "Seller hold: Database failure retries then parks".
func TestUserStatusChangedDBFailureRetriesThenDLQ(t *testing.T) {
	st := newFakeStore()
	st.seed("l1", "u1", "published")
	st.failErr = errors.New("db down")
	dlq := &fakeDLQ{}
	c := &Consumer{dlq: dlq, cfg: RetryConfig{MaxAttempts: 5, BaseBackoff: time.Microsecond, MaxBackoff: time.Millisecond}}
	h := newHandler(st)
	value := statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1)

	if err := c.processRecord(context.Background(), []byte("u1"), value, h.Handle, discardLogger()); err != nil {
		t.Fatalf("processRecord: %v", err)
	}
	if st.calls != 5 {
		t.Errorf("store attempts = %d, want 5", st.calls)
	}
	if len(dlq.records) != 1 {
		t.Fatalf("dlq records = %d, want 1", len(dlq.records))
	}
	if st.status("l1") != "published" || len(st.processed) != 0 || len(st.outbox) != 0 {
		t.Errorf("state changed: status=%q processed=%v outbox=%d", st.status("l1"), st.processed, len(st.outbox))
	}
}

// Transient errors must NOT be classified permanent (they must be retried).
func TestUserStatusChangedDBErrorIsNotPermanent(t *testing.T) {
	st := newFakeStore()
	st.seed("l1", "u1", "published")
	st.failErr = errors.New("db down")
	err := newHandler(st).Handle(context.Background(), nil,
		statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1))
	if err == nil || errors.Is(err, ErrPermanent) {
		t.Fatalf("err = %v, want a retryable error", err)
	}
}

// Consumer-level support for the permanent-error rule (scenario "Malformed event is
// parked, not applied"): a permanent error is parked after exactly one attempt.
func TestProcessRecord_PermanentErrorParksWithoutRetry(t *testing.T) {
	dlq := &fakeDLQ{}
	c := testConsumer(dlq)
	var calls int
	h := func(context.Context, []byte, []byte) error { calls++; return ErrPermanent }
	if err := c.processRecord(context.Background(), []byte("k"), []byte("v"), h, discardLogger()); err != nil {
		t.Fatalf("err = %v", err)
	}
	if calls != 1 || len(dlq.records) != 1 {
		t.Fatalf("calls=%d dlq=%d, want 1/1", calls, len(dlq.records))
	}
}

// fakeDLQ records records parked to the dead-letter topic.
type fakeDLQ struct {
	mu      sync.Mutex
	records [][2][]byte
}

func (f *fakeDLQ) Produce(_ context.Context, key, value []byte) error {
	f.mu.Lock()
	f.records = append(f.records, [2][]byte{key, value})
	f.mu.Unlock()
	return nil
}
func (f *fakeDLQ) Close() {}

func testConsumer(dlq DLQProducer) *Consumer {
	return &Consumer{dlq: dlq, cfg: RetryConfig{MaxAttempts: 3, BaseBackoff: time.Microsecond, MaxBackoff: time.Millisecond}}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
