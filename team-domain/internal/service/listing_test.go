package service_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

func TestListingService(t *testing.T) {
	repo := repository.NewInMemoryListingRepository()
	svc := service.NewListingService(repo)
	ctx := context.Background()

	t.Run("Create and Get Listing", func(t *testing.T) {
		l := repository.Listing{
			Title:       "iPhone 15 Pro",
			Description: "Apple smartphone",
			Price:       25000000,
			Currency:    "VND",
			Status:      "published",
			Stock:       10,
		}

		created, err := svc.Create(ctx, l, "seller_1")
		if err != nil {
			t.Fatalf("unexpected error creating listing: %v", err)
		}
		if created.ID == "" {
			t.Fatalf("expected non-empty ID")
		}
		if created.SellerID != "seller_1" {
			t.Errorf("expected seller_1, got %s", created.SellerID)
		}

		got, err := svc.Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("unexpected error getting listing: %v", err)
		}
		if got.Title != "iPhone 15 Pro" {
			t.Errorf("expected iPhone 15 Pro, got %s", got.Title)
		}

		// Update as owner
		got.Title = "iPhone 15 Pro Max"
		updated, err := svc.Update(ctx, got, "seller_1", false)
		if err != nil {
			t.Fatalf("unexpected error updating listing: %v", err)
		}
		if updated.Title != "iPhone 15 Pro Max" {
			t.Errorf("expected iPhone 15 Pro Max, got %s", updated.Title)
		}

		// Update as non-owner without admin fails
		_, err = svc.Update(ctx, got, "another_seller", false)
		if err != service.ErrForbidden {
			t.Errorf("expected ErrForbidden when non-owner updates, got %v", err)
		}

		// Update as admin succeeds
		got.Title = "iPhone 15 Pro Max (Admin Edited)"
		adminUpdated, err := svc.Update(ctx, got, "another_seller", true)
		if err != nil {
			t.Fatalf("unexpected error admin updating: %v", err)
		}
		if adminUpdated.Title != "iPhone 15 Pro Max (Admin Edited)" {
			t.Errorf("expected admin edited title")
		}

		// Delete non-owner fails
		_, err = svc.Delete(ctx, created.ID, "another_seller", false)
		if err != service.ErrForbidden {
			t.Errorf("expected ErrForbidden when non-owner deletes")
		}

		// Delete as owner succeeds
		deleted, err := svc.Delete(ctx, created.ID, "seller_1", false)
		if err != nil {
			t.Fatalf("unexpected error deleting: %v", err)
		}
		if deleted.ID != created.ID {
			t.Errorf("expected deleted ID %s, got %s", created.ID, deleted.ID)
		}
	})

	t.Run("CreateWithEvent enqueues one row on success", func(t *testing.T) {
		repo := repository.NewInMemoryListingRepository()
		txw := repository.NewInMemoryTxWriter(repo)
		svc := service.NewListingServiceWithOutbox(repo, txw)

		created, err := svc.CreateWithEvent(ctx, repository.Listing{Title: "Tx OK", Currency: "VND"}, "seller_1",
			func(l repository.Listing) (repository.OutboxRow, error) {
				return repository.OutboxRow{EventID: "evt-ok", AggregateID: l.ID, EventType: "test"}, nil
			})
		if err != nil {
			t.Fatalf("CreateWithEvent: %v", err)
		}
		if _, err := repo.Get(ctx, created.ID); err != nil {
			t.Fatalf("listing should be persisted: %v", err)
		}
		rows := txw.Rows()
		if len(rows) != 1 || rows[0].AggregateID != created.ID {
			t.Fatalf("want 1 outbox row for %s, got %+v", created.ID, rows)
		}
	})

	t.Run("CreateWithEvent rolls back both rows on write failure", func(t *testing.T) {
		repo := repository.NewInMemoryListingRepository()
		txw := repository.NewInMemoryTxWriter(repo)
		txw.FailWrite = true // simulate the business write failing inside the tx
		svc := service.NewListingServiceWithOutbox(repo, txw)

		created, err := svc.CreateWithEvent(ctx, repository.Listing{ID: "rollback-1", Title: "boom", Currency: "VND"}, "seller_1",
			func(l repository.Listing) (repository.OutboxRow, error) {
				return repository.OutboxRow{EventID: "evt-x", AggregateID: l.ID}, nil
			})
		if err == nil {
			t.Fatal("expected error when the write fails")
		}
		// Neither the listing nor an outbox row may survive a rolled-back write.
		if _, err := repo.Get(ctx, "rollback-1"); err != repository.ErrNotFound {
			t.Fatalf("listing must not be persisted on rollback, got %v", err)
		}
		if got := len(txw.Rows()); got != 0 {
			t.Fatalf("no outbox row may survive rollback, got %d", got)
		}
		_ = created
	})

	t.Run("List and ListMine", func(t *testing.T) {
		_, _ = svc.Create(ctx, repository.Listing{Title: "Item 1", Price: 100, Status: "published"}, "seller_a")
		_, _ = svc.Create(ctx, repository.Listing{Title: "Item 2", Price: 200, Status: "published"}, "seller_a")
		_, _ = svc.Create(ctx, repository.Listing{Title: "Item 3", Price: 300, Status: "published"}, "seller_b")

		page, err := svc.List(ctx, "", 10, "published")
		if err != nil {
			t.Fatalf("unexpected error listing: %v", err)
		}
		if len(page.Items) < 3 {
			t.Errorf("expected at least 3 items, got %d", len(page.Items))
		}

		minePage, err := svc.ListMine(ctx, "seller_a", "", 10)
		if err != nil {
			t.Fatalf("unexpected error listing mine: %v", err)
		}
		if len(minePage.Items) != 2 {
			t.Errorf("expected 2 items for seller_a, got %d", len(minePage.Items))
		}
	})
}
