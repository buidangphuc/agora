package events_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	listingv1 "github.com/buidangphuc/team-engagement/generated/platform/listing/v1"
	"github.com/buidangphuc/team-engagement/internal/events"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

func listingSetup(t *testing.T) (*storetest.FakeRepository, consume.Handler) {
	t.Helper()
	r := storetest.NewFakeRepository()
	if _, err := r.Follow(context.Background(), "buyer", "seller"); err != nil {
		t.Fatal(err)
	}
	return r, events.ListingEventHandler(r)
}

func feed(t *testing.T, r *storetest.FakeRepository, user string) []string {
	t.Helper()
	ids, _, _, err := r.ListFollowedListings(context.Background(), user, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func handle(t *testing.T, h consume.Handler, ev []byte) {
	t.Helper()
	if err := h(context.Background(), consume.Record{Topic: "listing.events", Value: ev}); err != nil {
		t.Fatalf("handler: %v", err)
	}
}

// Scenario: Engagement SDK: Published listing appears in the follower's feed
func TestPublishedListingAppearsInFeed(t *testing.T) {
	r, h := listingSetup(t)
	handle(t, h, changed(t, "l1", "seller", published, created, t0))
	if got := feed(t, r, "buyer"); len(got) != 1 || got[0] != "l1" {
		t.Fatalf("feed = %v, want [l1]", got)
	}
	handle(t, h, changed(t, "lx", "other", published, created, t0))
	if got := feed(t, r, "buyer"); len(got) != 1 {
		t.Fatalf("feed = %v, want only l1", got)
	}
}

// Scenario: Engagement SDK: Draft listing is not in the feed
func TestDraftListingIsNotInFeed(t *testing.T) {
	r, h := listingSetup(t)
	handle(t, h, changed(t, "l1", "seller", draft, created, t0))
	if got := feed(t, r, "buyer"); len(got) != 0 {
		t.Fatalf("draft must not be in feed, got %v", got)
	}
}

// Scenario: Engagement SDK: An update keeps the creation order
func TestUpdateKeepsCreatedAtOrdering(t *testing.T) {
	r, h := listingSetup(t)
	handle(t, h, changed(t, "l1", "seller", published, created, t0))
	handle(t, h, changed(t, "l2", "seller", published, created, t0.Add(time.Hour)))
	handle(t, h, changed(t, "l1", "seller", published, updated, t0.Add(2*time.Hour)))
	got := feed(t, r, "buyer")
	if len(got) != 2 || got[0] != "l2" || got[1] != "l1" {
		t.Fatalf("feed = %v, want [l2 l1]", got)
	}
}

// Scenario: Engagement SDK: Unpublish and delete remove the listing from the feed
func TestUnpublishAndDeleteRemoveFromFeed(t *testing.T) {
	r, h := listingSetup(t)
	handle(t, h, changed(t, "l1", "seller", published, created, t0))
	handle(t, h, changed(t, "l2", "seller", published, created, t0.Add(time.Minute)))
	handle(t, h, changed(t, "l3", "seller", published, created, t0.Add(2*time.Minute)))

	handle(t, h, changed(t, "l1", "seller", draft, updated, t0.Add(time.Hour)))
	handle(t, h, envelopeOf(t, "evt-st", statusType, &listingv1.ListingStatusChanged{
		ListingId: "l2", Status: listingv1.ListingStatus_LISTING_STATUS_REJECTED,
	}, t0.Add(time.Hour)))
	if got := feed(t, r, "buyer"); len(got) != 1 || got[0] != "l3" {
		t.Fatalf("feed = %v, want [l3]", got)
	}
	handle(t, h, changed(t, "l3", "seller", published, deleted, t0.Add(2*time.Hour)))
	if got := feed(t, r, "buyer"); len(got) != 0 {
		t.Fatalf("feed = %v, want empty", got)
	}
	handle(t, h, changed(t, "ghost", "seller", published, deleted, t0))
}

// Scenario: Engagement SDK: Redelivery is idempotent
func TestRedeliveryIsIdempotent(t *testing.T) {
	r, h := listingSetup(t)
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

// Scenario: Engagement SDK: Other event types are ignored and invalid ones fail
func TestIgnoredAndMalformedEvents(t *testing.T) {
	_, h := listingSetup(t)
	ctx := context.Background()
	handle(t, h, envelopeOf(t, "evt-px", "platform.listing.v1.ListingPricingChanged", &listingv1.ListingPricingChanged{ListingId: "l1"}, t0))

	permanentType := reflect.TypeOf(consume.Permanent(errors.New("x")))
	garbage := h(ctx, consume.Record{Value: []byte("\xff\xff not proto")})
	if garbage == nil {
		t.Fatal("garbage envelope must error")
	}
	if reflect.TypeOf(garbage) != permanentType {
		t.Errorf("garbage envelope error = %T, want consume.Permanent", garbage)
	}
	for name, ev := range map[string][]byte{
		"missing listing id":          changed(t, "", "seller", published, created, t0),
		"published without seller id": changed(t, "l1", "", published, created, t0),
	} {
		err := h(ctx, consume.Record{Value: ev})
		if err == nil {
			t.Errorf("%s must error", name)
			continue
		}
		if reflect.TypeOf(err) == permanentType {
			t.Errorf("%s error is Permanent; it must take the retry path", name)
		}
	}
}
