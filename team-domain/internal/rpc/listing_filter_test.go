package rpc_test

import (
	"bytes"
	"context"
	"log/slog"
	"sort"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"

	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

func filterFixture(t *testing.T) *rpc.ListingHandler {
	t.Helper()
	mk := func(id, title, st, seller string) store.Listing {
		return store.Listing{ID: id, Title: title, Price: 1, Currency: "VND", Status: st, Stock: 1, SellerID: seller}
	}
	repo := storetest.NewInMemoryListingRepository(
		mk("p1", "Red Shirt", "published", "seller-a"),
		mk("p2", "Blue Shoe", "published", "seller-a"),
		mk("d1", "Red Hat", "draft", "seller-a"),
		mk("r1", "Green Coat", "rejected", "seller-a"),
		mk("pr1", "Review Me", "pending_review", "seller-a"),
		mk("b1", "Banned Thing", "banned", "seller-a"),
		mk("h1", "Held Thing", "held", "seller-a"),
		mk("o1", "Red Shirt Other", "published", "seller-b"),
	)
	return rpc.NewListingHandler(listing.NewListingService(repo), nil, nil)
}

func sellerA() context.Context { return typedCtx("seller-a", "user", "listing.write") }

func idsOf(res *listingv1.ListMyListingsResponse) []string {
	var ids []string
	for _, l := range res.GetListings() {
		ids = append(ids, l.GetId())
	}
	sort.Strings(ids)
	return ids
}

func sameIDs(got []string, want ...string) bool {
	sort.Strings(want)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Scenario: A single status returns only that status (handler mapping)
func TestListMyListingsFilterStatusMapping(t *testing.T) {
	h := filterFixture(t)
	cases := map[listingv1.ListingStatus][]string{
		listingv1.ListingStatus_LISTING_STATUS_DRAFT:          {"d1"},
		listingv1.ListingStatus_LISTING_STATUS_PUBLISHED:      {"p1", "p2"},
		listingv1.ListingStatus_LISTING_STATUS_REJECTED:       {"r1"},
		listingv1.ListingStatus_LISTING_STATUS_PENDING_REVIEW: {"pr1"},
		listingv1.ListingStatus_LISTING_STATUS_BANNED:         {"b1"},
		listingv1.ListingStatus_LISTING_STATUS_HELD:           {"h1"},
		listingv1.ListingStatus_LISTING_STATUS_DELETED:        nil,
	}
	for st, want := range cases {
		res, err := h.ListMyListings(sellerA(), &listingv1.ListMyListingsRequest{Statuses: []listingv1.ListingStatus{st}})
		if err != nil {
			t.Fatalf("%v: %v", st, err)
		}
		if got := idsOf(res); !sameIDs(got, want...) {
			t.Errorf("%v: got %v want %v", st, got, want)
		}
	}
}

// Scenario: Several statuses are OR-ed
func TestListMyListingsFilterStatusesOred(t *testing.T) {
	h := filterFixture(t)
	res, err := h.ListMyListings(sellerA(), &listingv1.ListMyListingsRequest{Statuses: []listingv1.ListingStatus{
		listingv1.ListingStatus_LISTING_STATUS_DRAFT, listingv1.ListingStatus_LISTING_STATUS_REJECTED,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := idsOf(res); !sameIDs(got, "d1", "r1") {
		t.Fatalf("got %v", got)
	}
}

// Scenario: Query and status combine
func TestListMyListingsFilterQueryAndStatusCombine(t *testing.T) {
	h := filterFixture(t)
	res, err := h.ListMyListings(sellerA(), &listingv1.ListMyListingsRequest{
		Query:    "  rEd ",
		Statuses: []listingv1.ListingStatus{listingv1.ListingStatus_LISTING_STATUS_PUBLISHED},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := idsOf(res); !sameIDs(got, "p1") {
		t.Fatalf("got %v", got)
	}
}

// Scenario: An unspecified status is rejected
func TestListMyListingsFilterUnspecifiedStatusRejected(t *testing.T) {
	h := filterFixture(t)
	for name, sts := range map[string][]listingv1.ListingStatus{
		"alone": {listingv1.ListingStatus_LISTING_STATUS_UNSPECIFIED},
		"mixed": {listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, listingv1.ListingStatus_LISTING_STATUS_UNSPECIFIED},
	} {
		_, err := h.ListMyListings(sellerA(), &listingv1.ListMyListingsRequest{Statuses: sts})
		if code(err) != codes.InvalidArgument {
			t.Errorf("%s: code = %v, want InvalidArgument (%v)", name, code(err), err)
		}
	}
}

// Scenario: A query of exactly 100 runes is accepted, 101 is rejected
func TestListMyListingsFilterQueryLengthBoundary(t *testing.T) {
	h := filterFixture(t)
	ok100 := strings.Repeat("ê", 100)
	if _, err := h.ListMyListings(sellerA(), &listingv1.ListMyListingsRequest{Query: ok100}); err != nil {
		t.Errorf("100 runes: %v", err)
	}
	_, err := h.ListMyListings(sellerA(), &listingv1.ListMyListingsRequest{Query: strings.Repeat("ê", 101)})
	if code(err) != codes.InvalidArgument {
		t.Errorf("101 runes: code = %v, want InvalidArgument (%v)", code(err), err)
	}
	padded := "  " + ok100 + "  "
	if _, err := h.ListMyListings(sellerA(), &listingv1.ListMyListingsRequest{Query: padded}); err != nil {
		t.Errorf("100 runes plus padding must be measured after trim: %v", err)
	}
}

// Scenario: A blank query is ignored
func TestListMyListingsFilterBlankQueryIgnored(t *testing.T) {
	h := filterFixture(t)
	res, err := h.ListMyListings(sellerA(), &listingv1.ListMyListingsRequest{Query: " \t  "})
	if err != nil {
		t.Fatal(err)
	}
	if got := idsOf(res); !sameIDs(got, "p1", "p2", "d1", "r1", "pr1", "b1", "h1") {
		t.Fatalf("got %v", got)
	}
}

// Scenario: A caller without the listing.write scope is denied
func TestListMyListingsFilterPermissionDenied(t *testing.T) {
	h := filterFixture(t)
	req := &listingv1.ListMyListingsRequest{
		Query:    "red",
		Statuses: []listingv1.ListingStatus{listingv1.ListingStatus_LISTING_STATUS_PUBLISHED},
	}
	_, err := h.ListMyListings(typedCtx("buyer-1", "user", "listing.read"), req)
	if code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied (%v)", code(err), err)
	}
}

// Scenario: Another seller's listings are never returned
func TestListMyListingsFilterOtherSellerNeverReturned(t *testing.T) {
	h := filterFixture(t)
	res, err := h.ListMyListings(sellerA(), &listingv1.ListMyListingsRequest{
		Query:    "red shirt",
		Statuses: []listingv1.ListingStatus{listingv1.ListingStatus_LISTING_STATUS_PUBLISHED},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range res.GetListings() {
		if l.GetId() == "o1" {
			t.Fatalf("seller-b listing leaked: %v", idsOf(res))
		}
	}
	if got := idsOf(res); !sameIDs(got, "p1") {
		t.Fatalf("got %v", got)
	}
	resB, err := h.ListMyListings(typedCtx("seller-b", "user", "listing.write"), &listingv1.ListMyListingsRequest{Query: "red"})
	if err != nil || !sameIDs(idsOf(resB), "o1") {
		t.Fatalf("seller-b: err=%v got %v", err, idsOf(resB))
	}
}

// Scenario: The query text is not logged
func TestListMyListingsFilterQueryNotLogged(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := filterFixture(t)
	const secret = "zq-secret-needle-7431"
	_, _ = h.ListMyListings(sellerA(), &listingv1.ListMyListingsRequest{Query: secret})
	_, _ = h.ListMyListings(sellerA(), &listingv1.ListMyListingsRequest{Query: secret + strings.Repeat("x", 100)})
	_, _ = h.ListMyListings(sellerA(), &listingv1.ListMyListingsRequest{
		Query:    secret,
		Statuses: []listingv1.ListingStatus{listingv1.ListingStatus_LISTING_STATUS_UNSPECIFIED},
	})
	if strings.Contains(buf.String(), "needle") {
		t.Fatalf("query text found in log output: %s", buf.String())
	}
}
