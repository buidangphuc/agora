package rpc_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

type failingListingRepo struct {
	*storetest.InMemoryListingRepository
	err error
}

func (r failingListingRepo) Get(context.Context, string) (store.Listing, error) {
	return store.Listing{}, r.err
}

type failingStorefrontRepo struct {
	*storetest.InMemoryStorefrontRepository
	err error
}

func (r failingStorefrontRepo) Upsert(context.Context, store.Storefront) (store.Storefront, error) {
	return store.Storefront{}, r.err
}

type badCursorRepo struct {
	*storetest.InMemoryListingRepository
}

func (badCursorRepo) ListModerationQueue(context.Context, string, int32) (store.Page, error) {
	return store.Page{}, fmt.Errorf("decode cursor: %w", store.ErrInvalidCursor)
}

func getListingErr(t *testing.T, repoErr error) error {
	t.Helper()
	repo := failingListingRepo{InMemoryListingRepository: storetest.NewInMemoryListingRepository(), err: repoErr}
	h := rpc.NewListingHandler(listing.NewListingService(repo), nil, nil)
	_, err := h.GetListing(typedCtx("seller-a", "user", "listing.read", "listing.write"), &listingv1.GetListingRequest{Id: "L-1"})
	return err
}

func upsertStorefrontErr(t *testing.T, repoErr error) error {
	t.Helper()
	repo := failingStorefrontRepo{InMemoryStorefrontRepository: storetest.NewInMemoryStorefrontRepository(), err: repoErr}
	h := rpc.NewListingHandler(listing.NewListingService(storetest.NewInMemoryListingRepository()), nil, nil).
		WithStorefront(listing.NewStorefrontService(repo))
	_, err := h.UpsertStorefront(typedCtx("seller-a", "user", "listing.read", "listing.write"), &listingv1.UpsertStorefrontRequest{
		Storefront: &listingv1.Storefront{Slug: "shop-a"},
	})
	return err
}

// Scenario: Domain SDK: A unique violation is AlreadyExists.
func TestUniqueViolationMapsAlreadyExists(t *testing.T) {
	driver := fmt.Errorf("insert storefront: %w", &pgconn.PgError{
		Code: "23505", ConstraintName: "storefronts_seller_id_key", Message: "duplicate key value violates unique constraint",
	})
	err := upsertStorefrontErr(t, driver)
	st := status.Convert(err)
	if st.Code() != codes.AlreadyExists {
		t.Fatalf("code = %v (%q), want AlreadyExists", st.Code(), st.Message())
	}
	if strings.Contains(st.Message(), "duplicate key") || strings.Contains(st.Message(), "storefronts_seller_id_key") {
		t.Errorf("message leaks driver text: %q", st.Message())
	}
}

// Scenario: Domain SDK: A duplicate storefront slug is still AlreadyExists.
func TestUpsertStorefrontSlugTakenAlreadyExists(t *testing.T) {
	repo := storetest.NewInMemoryStorefrontRepository()
	h := rpc.NewListingHandler(listing.NewListingService(storetest.NewInMemoryListingRepository()), nil, nil).
		WithStorefront(listing.NewStorefrontService(repo))
	req := &listingv1.UpsertStorefrontRequest{Storefront: &listingv1.Storefront{Slug: "shop-a"}}
	if _, err := h.UpsertStorefront(typedCtx("seller-a", "user", "listing.read", "listing.write"), req); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	_, err := h.UpsertStorefront(typedCtx("seller-b", "user", "listing.read", "listing.write"), req)
	st := status.Convert(err)
	if st.Code() != codes.AlreadyExists || st.Message() != "slug already taken" {
		t.Fatalf("got %v %q, want AlreadyExists %q", st.Code(), st.Message(), "slug already taken")
	}

	wrapped := status.Convert(upsertStorefrontErr(t, fmt.Errorf("upsert: %w", store.ErrSlugTaken)))
	if wrapped.Code() != codes.AlreadyExists || wrapped.Message() != "slug already taken" {
		t.Fatalf("wrapped sentinel got %v %q, want AlreadyExists %q", wrapped.Code(), wrapped.Message(), "slug already taken")
	}
}

// Scenario: Domain SDK: Cancelled and expired calls keep their codes.
func TestContextErrorsKeepTheirCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"canceled", context.Canceled, codes.Canceled},
		{"wrapped canceled", fmt.Errorf("query: %w", context.Canceled), codes.Canceled},
		{"deadline exceeded", context.DeadlineExceeded, codes.DeadlineExceeded},
		{"wrapped deadline exceeded", fmt.Errorf("query: %w", context.DeadlineExceeded), codes.DeadlineExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := status.Code(getListingErr(t, tc.err)); got != tc.want {
				t.Fatalf("code = %v, want %v", got, tc.want)
			}
		})
	}
}

// Scenario: Domain SDK: An unknown error hides driver text.
func TestInternalErrorHidesDriverText(t *testing.T) {
	err := getListingErr(t, errors.New("pq: password authentication failed for user listing_svc"))
	st := status.Convert(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v %q, want Internal %q", st.Code(), st.Message(), "internal error")
	}
	if strings.Contains(err.Error(), "pq:") || strings.Contains(err.Error(), "listing_svc") {
		t.Fatalf("error text leaks driver detail: %q", err.Error())
	}
}

// Scenario: Domain SDK: Domain sentinels keep their codes.
func TestDomainSentinelsKeepCodes(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		want    codes.Code
		message string
	}{
		{"listing not found", store.ErrNotFound, codes.NotFound, ""},
		{"variant not found", store.ErrVariantNotFound, codes.NotFound, ""},
		{"reservation not found", store.ErrReservationNotFound, codes.NotFound, ""},
		{"category not found", store.ErrCategoryNotFound, codes.NotFound, ""},
		{"bundle not found", store.ErrBundleNotFound, codes.NotFound, ""},
		{"storefront not found", store.ErrStorefrontNotFound, codes.NotFound, ""},
		{"slug taken", store.ErrSlugTaken, codes.AlreadyExists, "slug already taken"},
		{"status locked", listing.ErrStatusLocked, codes.FailedPrecondition, "listing status does not allow this change"},
		{"status changed", store.ErrStatusChanged, codes.FailedPrecondition, "listing status does not allow this change"},
		{"reservation released", store.ErrReservationReleased, codes.FailedPrecondition, ""},
		{"listing not reservable", store.ErrListingNotReservable, codes.FailedPrecondition, ""},
		{"transition not allowed", listing.ErrTransitionNotAllowed, codes.FailedPrecondition, ""},
		{"not the owner", listing.ErrForbidden, codes.PermissionDenied, "not the listing owner"},
		{"invalid cursor", store.ErrInvalidCursor, codes.InvalidArgument, ""},
	}
	for _, tc := range cases {
		for _, wrap := range []bool{false, true} {
			name := tc.name
			err := tc.err
			if wrap {
				name += " wrapped"
				err = fmt.Errorf("load listing: %w", tc.err)
			}
			t.Run(name, func(t *testing.T) {
				got := getListingErr(t, err)
				st := status.Convert(got)
				if st.Code() != tc.want {
					t.Fatalf("code = %v (%q), want %v", st.Code(), st.Message(), tc.want)
				}
				if tc.message != "" && st.Message() != tc.message {
					t.Errorf("message = %q, want %q", st.Message(), tc.message)
				}
				if wrap && strings.Contains(st.Message(), "load listing") {
					t.Errorf("message leaks the wrapping chain: %q", st.Message())
				}
			})
		}
	}
}

// Scenario: Domain SDK: A malformed queue cursor is rejected.
func TestListModerationQueueInvalidCursor(t *testing.T) {
	repo := storetest.NewInMemoryListingRepository()
	svc := listing.NewListingServiceWithOutbox(badCursorRepo{repo}, storetest.NewInMemoryTxWriter(repo))
	h := rpc.NewListingHandler(svc, nil, nil)
	_, err := h.ListModerationQueue(adminCtx(), &listingv1.ListModerationQueueRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v (%v), want InvalidArgument", status.Code(err), err)
	}
}
