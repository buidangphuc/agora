package rpc_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

var decidedAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func modListing(id, st string) store.Listing {
	return store.Listing{
		ID: id, Title: "t", Price: 1, Currency: "VND", Status: st, Stock: 1, SellerID: "seller-a",
		ModerationReasonCode: "counterfeit", ModerationNote: "Fake logo",
		ScreenFlags: []string{"keyword:replica"}, ModerationDecidedAt: decidedAt, SubmittedAt: decidedAt,
	}
}

type modFixture struct {
	h   *rpc.ListingHandler
	txw *storetest.InMemoryTxWriter
}

func newModFixture(seed ...store.Listing) *modFixture {
	repo := storetest.NewInMemoryListingRepository(seed...)
	txw := storetest.NewInMemoryTxWriter(repo)
	return &modFixture{
		h:   rpc.NewListingHandler(listing.NewListingServiceWithOutbox(repo, txw), nil, nil),
		txw: txw,
	}
}

func (f *modFixture) get(ctx context.Context, t *testing.T, id string) *listingv1.Listing {
	t.Helper()
	res, err := f.h.GetListing(ctx, &listingv1.GetListingRequest{Id: id})
	if err != nil {
		t.Fatalf("GetListing(%s): %v", id, err)
	}
	return res.Listing
}

// Scenario: Status ownership: Owner sees status and reason but not flags.
func TestGetListingOwnerSeesReasonNotFlags(t *testing.T) {
	f := newModFixture(modListing("L-rej", "rejected"))
	l := f.get(typedCtx("seller-a", "user", "listing.read", "listing.write"), t, "L-rej")
	if l.Status != listingv1.ListingStatus_LISTING_STATUS_REJECTED {
		t.Errorf("status = %v, want REJECTED", l.Status)
	}
	m := l.GetModeration()
	if m.GetReasonCode() != "counterfeit" || m.GetNote() != "Fake logo" {
		t.Errorf("moderation = %v, want reason counterfeit and note Fake logo", m)
	}
	if len(m.GetScreenFlags()) != 0 {
		t.Errorf("owner must not see screen_flags, got %v", m.GetScreenFlags())
	}
	if m.GetDecidedAt() == nil || !m.GetDecidedAt().AsTime().Equal(decidedAt) {
		t.Errorf("decided_at = %v, want %v", m.GetDecidedAt(), decidedAt)
	}
}

// Scenario: Status ownership: Owner sees status and reason but not flags
// (the three server-only wire statuses map to their enum values).
func TestGetListingOwnerSeesServerOnlyStatuses(t *testing.T) {
	want := map[string]listingv1.ListingStatus{
		"pending_review": listingv1.ListingStatus_LISTING_STATUS_PENDING_REVIEW,
		"banned":         listingv1.ListingStatus_LISTING_STATUS_BANNED,
		"held":           listingv1.ListingStatus_LISTING_STATUS_HELD,
	}
	for st, enum := range want {
		t.Run(st, func(t *testing.T) {
			f := newModFixture(modListing("L-x", st))
			l := f.get(typedCtx("seller-a", "user", "listing.read", "listing.write"), t, "L-x")
			if l.Status != enum {
				t.Errorf("status = %v, want %v", l.Status, enum)
			}
		})
	}
}

// Scenario: Status ownership: Admin sees screen flags.
func TestGetListingAdminSeesFlags(t *testing.T) {
	f := newModFixture(modListing("L-pend", "pending_review"))
	for name, ctx := range map[string]context.Context{
		"admin":   typedCtx("admin-1", "user", "listing.read", "admin"),
		"service": typedCtx("service-team-promotion", "service", "listing.read"),
	} {
		l := f.get(ctx, t, "L-pend")
		if got := l.GetModeration().GetScreenFlags(); !reflect.DeepEqual(got, []string{"keyword:replica"}) {
			t.Errorf("%s screen_flags = %v, want [keyword:replica]", name, got)
		}
		if l.Status != listingv1.ListingStatus_LISTING_STATUS_PENDING_REVIEW {
			t.Errorf("%s status = %v, want PENDING_REVIEW", name, l.Status)
		}
	}
}

// Scenario: Status ownership: Screen flags are hidden from non-owners.
func TestGetListingStripsModerationForStranger(t *testing.T) {
	f := newModFixture(modListing("L-pub", "published"))
	for name, ctx := range map[string]context.Context{
		"anonymous": typedCtx("anonymous", "anonymous", "listing.read"),
		"stranger":  typedCtx("seller-b", "user", "listing.read", "listing.write"),
	} {
		m := f.get(ctx, t, "L-pub").GetModeration()
		if len(m.GetScreenFlags()) != 0 || m.GetNote() != "" {
			t.Errorf("%s: flags/note must be empty, got %v", name, m)
		}
		// Design: everyone else gets an empty moderation.
		if m.GetReasonCode() != "" {
			t.Errorf("%s: reason_code must be empty for non-owners, got %q", name, m.GetReasonCode())
		}
	}
}

// Scenario: Status ownership: Pending listing is hidden from a stranger.
func TestGetListingPendingReviewHiddenFromStranger(t *testing.T) {
	f := newModFixture(modListing("L-pend", "pending_review"))
	for name, ctx := range map[string]context.Context{
		"anonymous": typedCtx("anonymous", "anonymous", "listing.read"),
		"stranger":  typedCtx("seller-b", "user", "listing.read", "listing.write"),
	} {
		_, err := f.h.GetListing(ctx, &listingv1.GetListingRequest{Id: "L-pend"})
		if code(err) != codes.NotFound {
			t.Errorf("%s: want NotFound got %v", name, err)
		}
	}
}

// Scenario: Moderation: ListingChanged carries no note or flags.
func TestListingChangedEventStripsNoteAndFlags(t *testing.T) {
	f := newModFixture(modListing("L-rej", "rejected"))
	_, err := f.h.DeleteListing(typedCtx("seller-a", "user", "listing.read", "listing.write"),
		&listingv1.DeleteListingRequest{Id: "L-rej"})
	if err != nil {
		t.Fatalf("DeleteListing: %v", err)
	}
	rows := f.txw.Rows()
	if len(rows) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(rows))
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rows[0].Payload, &env); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	var ev listingv1.ListingChanged
	if err := proto.Unmarshal(env.Payload, &ev); err != nil {
		t.Fatalf("ListingChanged: %v", err)
	}
	m := ev.GetListing().GetModeration()
	if m.GetReasonCode() != "counterfeit" {
		t.Errorf("event reason_code = %q, want counterfeit", m.GetReasonCode())
	}
	if m.GetNote() != "" || len(m.GetScreenFlags()) != 0 {
		t.Errorf("event must carry no note or flags, got note=%q flags=%v", m.GetNote(), m.GetScreenFlags())
	}
	if ev.GetListing().GetStatus() != listingv1.ListingStatus_LISTING_STATUS_REJECTED {
		t.Errorf("event status = %v, want REJECTED", ev.GetListing().GetStatus())
	}
}
