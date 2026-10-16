package service_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

func ownershipSvc(stored string) (*service.ListingService, *repository.InMemoryListingRepository) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Price: 100, Status: stored, SellerID: "seller-a", Stock: 1,
	})
	return service.NewListingServiceWithOutbox(repo, repository.NewInMemoryTxWriter(repo)), repo
}

func noEvent(l repository.Listing) (repository.OutboxRow, error) {
	return repository.OutboxRow{EventID: "e-" + l.ID, AggregateID: l.ID, EventType: "test"}, nil
}

// Scenario: Status ownership: Seller cannot un-reject by sending PUBLISHED.
func TestUpdateWithEventRejectedStaysUnpublished(t *testing.T) {
	svc, repo := ownershipSvc("rejected")
	_, _ = svc.UpdateWithEvent(context.Background(),
		repository.Listing{ID: "L-1", Title: "orig", Currency: "VND", Price: 100, Stock: 1, Status: "published"},
		"seller-a", false, noEvent)
	got, _ := repo.Get(context.Background(), "L-1")
	if got.Status != "rejected" {
		t.Fatalf("stored status = %q, want rejected", got.Status)
	}
}

// Scenario: Status ownership: Seller cannot move a published listing into a server-only status.
func TestUpdateWithEventClientBannedIgnored(t *testing.T) {
	for _, st := range []string{"banned", "held", "pending_review", "rejected"} {
		t.Run(st, func(t *testing.T) {
			svc, repo := ownershipSvc("published")
			_, err := svc.UpdateWithEvent(context.Background(),
				repository.Listing{ID: "L-1", Title: "orig", Currency: "VND", Price: 100, Stock: 1, Status: st},
				"seller-a", false, noEvent)
			if err != nil {
				t.Fatalf("UpdateWithEvent: %v", err)
			}
			got, _ := repo.Get(context.Background(), "L-1")
			if got.Status != "published" {
				t.Fatalf("stored status = %q, want published", got.Status)
			}
		})
	}
}

// Scenario: Status ownership: Status omitted keeps a published listing published.
func TestUpdateWithEventOmittedStatusKeepsStored(t *testing.T) {
	for _, stored := range []string{"published", "draft"} {
		t.Run(stored, func(t *testing.T) {
			svc, repo := ownershipSvc(stored)
			_, err := svc.UpdateWithEvent(context.Background(),
				repository.Listing{ID: "L-1", Title: "orig", Currency: "VND", Price: 999, Stock: 1, Status: ""},
				"seller-a", false, noEvent)
			if err != nil {
				t.Fatalf("UpdateWithEvent: %v", err)
			}
			got, _ := repo.Get(context.Background(), "L-1")
			if got.Status != stored {
				t.Fatalf("stored status = %q, want %q", got.Status, stored)
			}
		})
	}
}

// Scenario: Status ownership: Server-only statuses sent on create are ignored.
func TestCreateWithEventServerOnlyStatusIgnored(t *testing.T) {
	for _, st := range []string{"rejected", "banned", "pending_review", "held", ""} {
		t.Run("status="+st, func(t *testing.T) {
			svc, _ := ownershipSvc("draft")
			got, err := svc.CreateWithEvent(context.Background(),
				repository.Listing{Title: "n", Currency: "VND", Stock: 1, Status: st}, "seller-a", noEvent)
			if err != nil {
				t.Fatalf("CreateWithEvent: %v", err)
			}
			if got.Status != "draft" {
				t.Fatalf("status = %q, want draft", got.Status)
			}
		})
	}
}

// Scenario: Status ownership: Another seller still cannot edit the listing.
func TestUpdateWithEventNonOwnerForbidden(t *testing.T) {
	svc, repo := ownershipSvc("pending_review")
	_, err := svc.UpdateWithEvent(context.Background(),
		repository.Listing{ID: "L-1", Title: "x", Currency: "VND", Stock: 1, Status: "published"},
		"seller-b", false, noEvent)
	if err != service.ErrForbidden {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	got, _ := repo.Get(context.Background(), "L-1")
	if got.Status != "pending_review" {
		t.Fatalf("status = %q", got.Status)
	}
}
