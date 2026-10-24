package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

func contextWithPrincipal(ctx context.Context, p *commonv1.Principal) context.Context {
	return auth.WithPrincipal(ctx, auth.Principal{ID: p.GetId(), Type: auth.TypeUser, Scopes: p.GetScopes()})
}

func TestListingHandler(t *testing.T) {
	repo := storetest.NewInMemoryListingRepository()
	svc := listing.NewListingService(repo)
	h := rpc.NewListingHandler(svc, nil, nil)

	principal := &commonv1.Principal{
		Id:     "seller_test",
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"listing.write", "listing.read"},
	}
	ctx := contextWithPrincipal(context.Background(), principal)

	t.Run("Create and Get Listing via Handler", func(t *testing.T) {
		createRes, err := h.CreateListing(ctx, &listingv1.CreateListingRequest{
			Listing: &listingv1.Listing{
				Title:       "MacBook Pro M3",
				Description: "Laptop",
				Price:       45000000,
				Currency:    "VND",
				Status:      listingv1.ListingStatus_LISTING_STATUS_PUBLISHED,
				Stock:       5,
			},
		})
		if err != nil {
			t.Fatalf("unexpected error creating listing: %v", err)
		}
		if createRes.Listing.Title != "MacBook Pro M3" {
			t.Errorf("expected MacBook Pro M3, got %s", createRes.Listing.Title)
		}

		getRes, err := h.GetListing(ctx, &listingv1.GetListingRequest{
			Id: createRes.Listing.Id,
		})
		if err != nil {
			t.Fatalf("unexpected error getting listing: %v", err)
		}
		if getRes.Listing.Id != createRes.Listing.Id {
			t.Errorf("expected ID %s, got %s", createRes.Listing.Id, getRes.Listing.Id)
		}

		listRes, err := h.ListListings(ctx, &listingv1.ListListingsRequest{
			Page: &commonv1.PageRequest{PageSize: 10},
		})
		if err != nil {
			t.Fatalf("unexpected error listing: %v", err)
		}
		if len(listRes.Listings) == 0 {
			t.Errorf("expected non-empty listings")
		}
	})
}

func ownershipFixture(t *testing.T, stored string) (*rpc.ListingHandler, *storetest.InMemoryListingRepository, *storetest.InMemoryTxWriter) {
	t.Helper()
	repo := storetest.NewInMemoryListingRepository(store.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Price: 100, Status: stored, SellerID: "seller-a", Stock: 1,
	})
	txw := storetest.NewInMemoryTxWriter(repo)
	svc := listing.NewListingServiceWithOutbox(repo, txw)
	return rpc.NewListingHandler(svc, nil, nil), repo, txw
}

// Scenario: Status ownership: Status omitted keeps a published listing published.
func TestUpdateListingOmittedStatusKeepsPublished(t *testing.T) {
	h, repo, txw := ownershipFixture(t, "published")
	_, err := h.UpdateListing(ownerCtx(), &listingv1.UpdateListingRequest{Listing: &listingv1.Listing{
		Id: "L-1", Title: "orig", Currency: "VND", Price: 250, Stock: 1,
		Status: listingv1.ListingStatus_LISTING_STATUS_UNSPECIFIED,
	}})
	if err != nil {
		t.Fatalf("UpdateListing: %v", err)
	}
	got, _ := repo.Get(context.Background(), "L-1")
	if got.Status != "published" || got.Price != 250 {
		t.Fatalf("want published with price 250, got status=%q price=%v", got.Status, got.Price)
	}
	rows := txw.Rows()
	if len(rows) != 1 {
		t.Fatalf("want 1 outbox row, got %d", len(rows))
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rows[0].Payload, &env); err != nil {
		t.Fatal(err)
	}
	var ev listingv1.ListingChanged
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.GetChangeType() != listingv1.ChangeType_CHANGE_TYPE_UPDATED ||
		ev.GetListing().GetStatus() != listingv1.ListingStatus_LISTING_STATUS_PUBLISHED {
		t.Fatalf("outbox event = %v, want UPDATED with PUBLISHED", &ev)
	}
}

// Scenario: Status ownership: Server-only statuses sent on create are ignored.
func TestCreateListingServerOnlyStatusesStoredAsDraft(t *testing.T) {
	for name, st := range map[string]listingv1.ListingStatus{
		"rejected":       listingv1.ListingStatus_LISTING_STATUS_REJECTED,
		"banned":         listingv1.ListingStatus_LISTING_STATUS_BANNED,
		"pending_review": listingv1.ListingStatus_LISTING_STATUS_PENDING_REVIEW,
		"held":           listingv1.ListingStatus_LISTING_STATUS_HELD,
		"unspecified":    listingv1.ListingStatus_LISTING_STATUS_UNSPECIFIED,
	} {
		t.Run(name, func(t *testing.T) {
			h, repo, _ := ownershipFixture(t, "draft")
			res, err := h.CreateListing(ownerCtx(), &listingv1.CreateListingRequest{Listing: &listingv1.Listing{
				Title: "new", Currency: "VND", Price: 1, Stock: 1, Status: st,
			}})
			if err != nil {
				t.Fatalf("CreateListing: %v", err)
			}
			if res.GetListing().GetStatus() != listingv1.ListingStatus_LISTING_STATUS_DRAFT {
				t.Fatalf("response status = %v, want DRAFT", res.GetListing().GetStatus())
			}
			stored, err := repo.Get(context.Background(), res.GetListing().GetId())
			if err != nil || stored.Status != "draft" {
				t.Fatalf("stored status = %q (%v), want draft", stored.Status, err)
			}
		})
	}
}

// Scenario: Status ownership: Seller cannot un-reject by sending PUBLISHED (handler path).
func TestUpdateListingRejectedStaysUnpublishedViaHandler(t *testing.T) {
	h, repo, _ := ownershipFixture(t, "rejected")
	_, _ = h.UpdateListing(ownerCtx(), &listingv1.UpdateListingRequest{Listing: &listingv1.Listing{
		Id: "L-1", Title: "orig", Currency: "VND", Price: 100, Stock: 1,
		Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED,
	}})
	got, _ := repo.Get(context.Background(), "L-1")
	if got.Status == "published" {
		t.Fatalf("rejected listing must never become published by a seller write")
	}
}

// Scenario: Status ownership: Another seller still cannot edit the listing.
func TestUpdateListingNonOwnerPublishDeniedStaysPending(t *testing.T) {
	h, repo, txw := ownershipFixture(t, "pending_review")
	_, err := h.UpdateListing(otherCtx(), &listingv1.UpdateListingRequest{Listing: &listingv1.Listing{
		Id: "L-1", Title: "orig", Currency: "VND", Stock: 1,
		Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED,
	}})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
	got, _ := repo.Get(context.Background(), "L-1")
	if got.Status != "pending_review" || len(txw.Rows()) != 0 {
		t.Fatalf("status=%q rows=%d, want pending_review and no rows", got.Status, len(txw.Rows()))
	}
}

// ---- domain-listing-moderation task 6: FAILED_PRECONDITION mapping ----

func lockedUpdateReq() *listingv1.UpdateListingRequest {
	return &listingv1.UpdateListingRequest{Listing: &listingv1.Listing{
		Id: "L-1", Title: "changed", Currency: "VND", Price: 100, Stock: 1,
		Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED,
	}}
}

// Scenario: Status ownership: Banned listing refuses seller writes.
func TestUpdateListingBannedFailedPrecondition(t *testing.T) {
	h, repo, txw := ownershipFixture(t, "banned")
	_, err := h.UpdateListing(ownerCtx(), lockedUpdateReq())
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition (%v)", status.Code(err), err)
	}
	got, _ := repo.Get(context.Background(), "L-1")
	if got.Status != "banned" || got.Title != "orig" || len(txw.Rows()) != 0 {
		t.Fatalf("row changed: %+v rows=%d", got, len(txw.Rows()))
	}
}

// Scenario: Status ownership: Held listing refuses seller writes.
func TestUpdateListingHeldFailedPrecondition(t *testing.T) {
	h, repo, txw := ownershipFixture(t, "held")
	_, err := h.UpdateListing(ownerCtx(), lockedUpdateReq())
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition (%v)", status.Code(err), err)
	}
	got, _ := repo.Get(context.Background(), "L-1")
	if got.Status != "held" || got.Title != "orig" || len(txw.Rows()) != 0 {
		t.Fatalf("row changed: %+v rows=%d", got, len(txw.Rows()))
	}
}

// Scenario: Status ownership: Banned or held listing refuses seller delete.
func TestDeleteListingBannedOrHeldFailedPrecondition(t *testing.T) {
	for _, st := range []string{"banned", "held"} {
		t.Run(st, func(t *testing.T) {
			h, repo, txw := ownershipFixture(t, st)
			_, err := h.DeleteListing(ownerCtx(), &listingv1.DeleteListingRequest{Id: "L-1"})
			if status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("code = %v, want FailedPrecondition (%v)", status.Code(err), err)
			}
			got, gerr := repo.Get(context.Background(), "L-1")
			if gerr != nil || got.Status != st || len(txw.Rows()) != 0 {
				t.Fatalf("row must remain unchanged with no outbox row: %+v err=%v rows=%d", got, gerr, len(txw.Rows()))
			}
		})
	}
}

// statusChangedTx simulates a moderation decision committing between the
// service's read and its transaction (the repository returns ErrStatusChanged).
type statusChangedTx struct{ *storetest.InMemoryTxWriter }

func (statusChangedTx) UpdateTx(context.Context, store.Listing, store.EnqueueFn) (store.Listing, error) {
	return store.Listing{}, store.ErrStatusChanged
}

// Scenario: Status ownership: Seller write racing a moderation decision is refused (handler mapping).
func TestUpdateListingStatusChangedFailedPrecondition(t *testing.T) {
	repo := storetest.NewInMemoryListingRepository(store.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Price: 100, Status: "pending_review", SellerID: "seller-a", Stock: 1,
	})
	txw := storetest.NewInMemoryTxWriter(repo)
	h := rpc.NewListingHandler(listing.NewListingServiceWithOutbox(repo, statusChangedTx{txw}), nil, nil)
	_, err := h.UpdateListing(ownerCtx(), lockedUpdateReq())
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition (%v)", status.Code(err), err)
	}
	if len(txw.Rows()) != 0 {
		t.Fatalf("outbox rows = %d, want 0", len(txw.Rows()))
	}
}

// Scenario: Status ownership: Admin UpdateListing cannot set status.
func TestUpdateListingAdminCannotSetStatus(t *testing.T) {
	repo := storetest.NewInMemoryListingRepository(store.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Status: "rejected", SellerID: "seller-a", Stock: 1,
	})
	txw := storetest.NewInMemoryTxWriter(repo)
	fa := &handlerAudit{repo: repo}
	svc := listing.NewListingServiceWithOutbox(repo, txw).WithAudit(auditx.NewGate(fa, nil))
	h := rpc.NewListingHandler(svc, nil, nil)
	if _, err := h.UpdateListing(adminCtx(), updReq(listingv1.ListingStatus_LISTING_STATUS_PUBLISHED)); err != nil {
		t.Fatalf("UpdateListing: %v", err)
	}
	l, _ := repo.Get(context.Background(), "L-1")
	if l.Status != "rejected" {
		t.Fatalf("stored status = %q, want rejected", l.Status)
	}
	if len(fa.calls) != 1 || fa.calls[0].status != "rejected" {
		t.Fatalf("audit calls = %+v, want 1 with status rejected", fa.calls)
	}
}

// ---- domain-seller-enforcement-hold task 4: HELD visibility and write lock ----

func heldHandler(t *testing.T) (*rpc.ListingHandler, *storetest.InMemoryListingRepository, *storetest.InMemoryTxWriter) {
	t.Helper()
	repo := storetest.NewInMemoryListingRepository(store.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Price: 100, Status: "held", SellerID: "seller-a", Stock: 1,
		ModerationReasonCode: "seller_enforcement",
	})
	txw := storetest.NewInMemoryTxWriter(repo)
	return rpc.NewListingHandler(listing.NewListingServiceWithOutbox(repo, txw), nil, nil), repo, txw
}

// Scenario: Seller hold: Non-owner cannot read a held listing
func TestGetListingHeldHiddenFromNonOwner(t *testing.T) {
	h, _, _ := heldHandler(t)
	for name, ctx := range map[string]context.Context{
		"buyer":        typedCtx("buyer-1", "user", "listing.read"),
		"anonymous":    typedCtx("anonymous", "anonymous", "listing.read"),
		"other seller": typedCtx("seller-b", "user", "listing.read", "listing.write"),
	} {
		_, err := h.GetListing(ctx, &listingv1.GetListingRequest{Id: "L-1"})
		if status.Code(err) != codes.NotFound {
			t.Errorf("%s: code = %v, want NotFound", name, status.Code(err))
		}
	}
}

// Scenario: Seller hold: Owner sees the held listing and the reason
func TestListMyListingsShowsHeldWithReason(t *testing.T) {
	h, _, _ := heldHandler(t)
	res, err := h.ListMyListings(typedCtx("seller-a", "user", "listing.read", "listing.write"), &listingv1.ListMyListingsRequest{})
	if err != nil || len(res.GetListings()) != 1 {
		t.Fatalf("ListMyListings: err=%v n=%d, want 1", err, len(res.GetListings()))
	}
	l := res.GetListings()[0]
	if l.GetStatus() != listingv1.ListingStatus_LISTING_STATUS_HELD {
		t.Errorf("status = %v, want HELD", l.GetStatus())
	}
	if got := l.GetModeration().GetReasonCode(); got != "seller_enforcement" {
		t.Errorf("reason = %q, want seller_enforcement", got)
	}
}

// Scenario: Seller hold: Owner cannot edit a held listing
func TestUpdateListingHeldRefusedFailedPrecondition(t *testing.T) {
	h, repo, txw := heldHandler(t)
	_, err := h.UpdateListing(ownerCtx(), lockedUpdateReq())
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition (%v)", status.Code(err), err)
	}
	got, _ := repo.Get(context.Background(), "L-1")
	if got.Status != "held" || got.Title != "orig" || len(txw.Rows()) != 0 {
		t.Fatalf("row changed: %+v rows=%d", got, len(txw.Rows()))
	}
}

// Scenario: Seller hold: Another seller cannot edit a held listing
func TestUpdateListingHeldNonOwnerDenied(t *testing.T) {
	h, repo, txw := heldHandler(t)
	_, err := h.UpdateListing(otherCtx(), lockedUpdateReq())
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied (%v)", status.Code(err), err)
	}
	got, _ := repo.Get(context.Background(), "L-1")
	if got.Status != "held" || got.Title != "orig" || len(txw.Rows()) != 0 {
		t.Fatalf("row changed: %+v rows=%d", got, len(txw.Rows()))
	}
}

// Scenario: Seller hold: Owner cannot delete a held listing
func TestDeleteListingHeldRefusedFailedPrecondition(t *testing.T) {
	h, repo, txw := heldHandler(t)
	_, err := h.DeleteListing(ownerCtx(), &listingv1.DeleteListingRequest{Id: "L-1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition (%v)", status.Code(err), err)
	}
	got, gerr := repo.Get(context.Background(), "L-1")
	if gerr != nil || got.Status != "held" || len(txw.Rows()) != 0 {
		t.Fatalf("listing must remain held with no outbox row: %+v err=%v rows=%d", got, gerr, len(txw.Rows()))
	}
}
