package handler_test

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/handler"
	"github.com/buidangphuc/team-domain/internal/interceptor"
	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

// contextWithPrincipal injects a resolved Principal the way the gateway does:
// as x-principal-* metadata run through the service's unary interceptor. The
// interceptor's context setter is unexported, so tests reach it via this path.
func contextWithPrincipal(ctx context.Context, p *commonv1.Principal) context.Context {
	md := metadata.Pairs(
		"x-principal-id", p.GetId(),
		"x-principal-type", "user",
		"x-principal-scopes", strings.Join(p.GetScopes(), ","),
	)
	ctx = metadata.NewIncomingContext(ctx, md)
	var out context.Context
	_, _ = interceptor.UnaryServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{},
		func(c context.Context, _ any) (any, error) {
			out = c
			return nil, nil
		})
	return out
}

func TestListingHandler(t *testing.T) {
	repo := repository.NewInMemoryListingRepository()
	svc := service.NewListingService(repo)
	h := handler.NewListingHandler(svc, nil, nil)

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

func ownershipFixture(t *testing.T, stored string) (*handler.ListingHandler, *repository.InMemoryListingRepository, *repository.InMemoryTxWriter) {
	t.Helper()
	repo := repository.NewInMemoryListingRepository(repository.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Price: 100, Status: stored, SellerID: "seller-a", Stock: 1,
	})
	txw := repository.NewInMemoryTxWriter(repo)
	svc := service.NewListingServiceWithOutbox(repo, txw)
	return handler.NewListingHandler(svc, nil, nil), repo, txw
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
type statusChangedTx struct{ *repository.InMemoryTxWriter }

func (statusChangedTx) UpdateTx(context.Context, repository.Listing, repository.EnqueueFn) (repository.Listing, error) {
	return repository.Listing{}, repository.ErrStatusChanged
}

// Scenario: Status ownership: Seller write racing a moderation decision is refused (handler mapping).
func TestUpdateListingStatusChangedFailedPrecondition(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Price: 100, Status: "pending_review", SellerID: "seller-a", Stock: 1,
	})
	txw := repository.NewInMemoryTxWriter(repo)
	h := handler.NewListingHandler(service.NewListingServiceWithOutbox(repo, statusChangedTx{txw}), nil, nil)
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
	repo := repository.NewInMemoryListingRepository(repository.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Status: "rejected", SellerID: "seller-a", Stock: 1,
	})
	txw := repository.NewInMemoryTxWriter(repo)
	fa := &handlerAudit{repo: repo}
	svc := service.NewListingServiceWithOutbox(repo, txw).WithAudit(fa)
	h := handler.NewListingHandler(svc, nil, nil)
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
