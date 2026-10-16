package handler_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/handler"
	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

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
