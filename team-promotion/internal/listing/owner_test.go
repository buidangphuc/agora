package listing

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
)

type fakeListings struct {
	owners map[string]string
	err    error
	calls  int
	ids    []string
}

func (f *fakeListings) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	f.calls++
	f.ids = append(f.ids, req.GetId())
	if f.err != nil {
		return nil, f.err
	}
	seller, ok := f.owners[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "not_found")
	}
	return &listingv1.GetListingResponse{Listing: &listingv1.Listing{Id: req.GetId(), SellerId: seller}}, nil
}

// Scenario: Listing lookup runs as the caller with listing.read (ownership outcomes as sentinels)
func TestOwnerSentinels(t *testing.T) {
	owners := map[string]string{"mine": "seller-1", "theirs": "seller-2", "orphan": ""}
	cases := []struct {
		name     string
		listings *fakeListings
		listing  string
		caller   string
		want     error
	}{
		{"own listing ok", &fakeListings{owners: owners}, "mine", "seller-1", nil},
		{"foreign listing denied", &fakeListings{owners: owners}, "theirs", "seller-1", ErrNotOwner},
		{"listing not found", &fakeListings{owners: owners}, "ghost", "seller-1", ErrNotFound},
		{"upstream error", &fakeListings{err: status.Error(codes.Internal, "boom")}, "mine", "seller-1", ErrLookupFailed},
		{"upstream down", &fakeListings{err: errors.New("dial tcp: refused")}, "mine", "seller-1", ErrLookupFailed},
		{"deadline exceeded", &fakeListings{err: status.Error(codes.DeadlineExceeded, "slow")}, "mine", "seller-1", ErrLookupFailed},
		{"listing without seller denied", &fakeListings{owners: owners}, "orphan", "seller-1", ErrNotOwner},
		{"empty caller denied", &fakeListings{owners: owners}, "mine", "", ErrNotOwner},
		{"empty caller and empty seller denied", &fakeListings{owners: owners}, "orphan", "", ErrNotOwner},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Owner(t.Context(), tc.listings, slog.New(slog.DiscardHandler), tc.listing, tc.caller)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.want == nil && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if tc.listings.calls != 1 || tc.listings.ids[0] != tc.listing {
				t.Fatalf("lookup calls = %d ids = %v, want one lookup of %q", tc.listings.calls, tc.listings.ids, tc.listing)
			}
		})
	}
}

// Scenario: A slow listing lookup is unavailable (the cause never reaches the caller)
func TestOwnerLookupFailureHidesCause(t *testing.T) {
	f := &fakeListings{err: status.Error(codes.Internal, "db password leaked")}
	err := Owner(t.Context(), f, slog.New(slog.DiscardHandler), "mine", "seller-1")
	if !errors.Is(err, ErrLookupFailed) {
		t.Fatalf("err = %v, want ErrLookupFailed", err)
	}
	if got := err.Error(); got != ErrLookupFailed.Error() {
		t.Fatalf("error text %q must not carry the upstream cause", got)
	}
}

func TestOwnerSentinelsAreDistinct(t *testing.T) {
	all := []error{ErrNotFound, ErrNotOwner, ErrLookupFailed}
	for i, a := range all {
		for j, b := range all {
			if (i == j) != errors.Is(a, b) {
				t.Fatalf("sentinel %d and %d: errors.Is = %v", i, j, errors.Is(a, b))
			}
		}
	}
}
