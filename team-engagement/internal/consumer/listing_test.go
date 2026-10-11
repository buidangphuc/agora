package consumer

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-engagement/generated/platform/listing/v1"
	"github.com/buidangphuc/team-engagement/internal/repository"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func envelope(t *testing.T, typ string, msg proto.Message, at time.Time) []byte {
	t.Helper()
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId: "evt", Type: typ, OccurredAt: timestamppb.New(at), Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func changed(t *testing.T, id, seller string, st listingv1.ListingStatus, ct listingv1.ChangeType, at time.Time) []byte {
	return envelope(t, listingChangedType, &listingv1.ListingChanged{
		Listing:    &listingv1.Listing{Id: id, SellerId: seller, Status: st},
		ChangeType: ct,
	}, at)
}

const (
	published = listingv1.ListingStatus_LISTING_STATUS_PUBLISHED
	draft     = listingv1.ListingStatus_LISTING_STATUS_DRAFT
	created   = listingv1.ChangeType_CHANGE_TYPE_CREATED
	updated   = listingv1.ChangeType_CHANGE_TYPE_UPDATED
	deleted   = listingv1.ChangeType_CHANGE_TYPE_DELETED
)

func feed(t *testing.T, r *repository.InMemoryRepository, user string) []string {
	t.Helper()
	ids, _, _, err := r.ListFollowedListings(context.Background(), user, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func setup(t *testing.T) (*repository.InMemoryRepository, Handler) {
	t.Helper()
	r := repository.NewInMemoryRepository()
	if _, err := r.Follow(context.Background(), "buyer", "seller"); err != nil {
		t.Fatal(err)
	}
	return r, ListingEventHandler(r)
}

func handle(t *testing.T, h Handler, ev []byte) {
	t.Helper()
	if err := h(context.Background(), nil, ev); err != nil {
		t.Fatalf("handler: %v", err)
	}
}

func TestPublishedListingAppearsInFeed(t *testing.T) {
	r, h := setup(t)
	handle(t, h, changed(t, "l1", "seller", published, created, t0))
	if got := feed(t, r, "buyer"); len(got) != 1 || got[0] != "l1" {
		t.Fatalf("feed = %v, want [l1]", got)
	}
	// A seller the buyer does not follow never leaks in.
	handle(t, h, changed(t, "lx", "other", published, created, t0))
	if got := feed(t, r, "buyer"); len(got) != 1 {
		t.Fatalf("feed = %v, want only l1", got)
	}
}

func TestDraftListingIsNotInFeed(t *testing.T) {
	r, h := setup(t)
	handle(t, h, changed(t, "l1", "seller", draft, created, t0))
	if got := feed(t, r, "buyer"); len(got) != 0 {
		t.Fatalf("draft must not be in feed, got %v", got)
	}
}

func TestUpdateKeepsCreatedAtOrdering(t *testing.T) {
	r, h := setup(t)
	handle(t, h, changed(t, "l1", "seller", published, created, t0))
	handle(t, h, changed(t, "l2", "seller", published, created, t0.Add(time.Hour)))
	// Editing the older listing later must not bump it above the newer one.
	handle(t, h, changed(t, "l1", "seller", published, updated, t0.Add(2*time.Hour)))
	got := feed(t, r, "buyer")
	if len(got) != 2 || got[0] != "l2" || got[1] != "l1" {
		t.Fatalf("feed = %v, want [l2 l1]", got)
	}
}

func TestUnpublishAndDeleteRemoveFromFeed(t *testing.T) {
	r, h := setup(t)
	handle(t, h, changed(t, "l1", "seller", published, created, t0))
	handle(t, h, changed(t, "l2", "seller", published, created, t0.Add(time.Minute)))
	handle(t, h, changed(t, "l3", "seller", published, created, t0.Add(2*time.Minute)))

	// published -> draft via ListingChanged
	handle(t, h, changed(t, "l1", "seller", draft, updated, t0.Add(time.Hour)))
	// published -> rejected via ListingStatusChanged
	handle(t, h, envelope(t, listingStatusChangedType, &listingv1.ListingStatusChanged{
		ListingId: "l2", Status: listingv1.ListingStatus_LISTING_STATUS_REJECTED,
	}, t0.Add(time.Hour)))
	if got := feed(t, r, "buyer"); len(got) != 1 || got[0] != "l3" {
		t.Fatalf("feed = %v, want [l3]", got)
	}
	// deleted (even though the payload still says published)
	handle(t, h, changed(t, "l3", "seller", published, deleted, t0.Add(2*time.Hour)))
	if got := feed(t, r, "buyer"); len(got) != 0 {
		t.Fatalf("feed = %v, want empty", got)
	}
	// deleting something we never saw is a no-op
	handle(t, h, changed(t, "ghost", "seller", published, deleted, t0))
}

func TestRedeliveryIsIdempotent(t *testing.T) {
	r, h := setup(t)
	ev := changed(t, "l1", "seller", published, created, t0)
	for i := 0; i < 3; i++ {
		handle(t, h, ev)
	}
	got, _, total, err := r.ListFollowedListings(context.Background(), "buyer", "", 50)
	if err != nil || len(got) != 1 || total != 1 {
		t.Fatalf("feed = %v total=%d err=%v, want one row", got, total, err)
	}
	del := changed(t, "l1", "seller", published, deleted, t0.Add(time.Hour))
	handle(t, h, del)
	handle(t, h, del)
	if got := feed(t, r, "buyer"); len(got) != 0 {
		t.Fatalf("feed = %v, want empty", got)
	}
}

func TestIgnoredAndMalformedEvents(t *testing.T) {
	_, h := setup(t)
	// other event types are ignored
	handle(t, h, envelope(t, "platform.listing.v1.ListingPricingChanged", &listingv1.ListingPricingChanged{ListingId: "l1"}, t0))
	// garbage and structurally invalid events are errors (retry, then DLQ)
	if err := h(context.Background(), nil, []byte("\xff\xff not proto")); err == nil {
		t.Error("garbage envelope must error")
	}
	if err := h(context.Background(), nil, changed(t, "", "seller", published, created, t0)); err == nil {
		t.Error("missing listing id must error")
	}
	if err := h(context.Background(), nil, changed(t, "l1", "", published, created, t0)); err == nil {
		t.Error("published listing without seller must error")
	}
}
