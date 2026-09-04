package repository_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-order/internal/repository"
)

func TestInMemoryCartRepository(t *testing.T) {
	repo := repository.NewInMemoryCartRepository()
	ctx := context.Background()

	t.Run("AddItem and GetCart", func(t *testing.T) {
		items, err := repo.AddItem(ctx, repository.CartItem{
			UserID:    "user_1",
			ListingID: "list_1",
			VariantID: "var_1",
			Quantity:  2,
			UnitPrice: 50000,
			Title:     "Item 1",
		})
		if err != nil {
			t.Fatalf("unexpected error adding item: %v", err)
		}
		if len(items) != 1 {
			t.Errorf("expected 1 item, got %d", len(items))
		}

		// Update quantity
		updated, err := repo.UpdateItem(ctx, "user_1", items[0].ID, 5)
		if err != nil {
			t.Fatalf("unexpected error updating item: %v", err)
		}
		if updated[0].Quantity != 5 {
			t.Errorf("expected quantity 5, got %d", updated[0].Quantity)
		}

		// Remove item
		removed, err := repo.RemoveItem(ctx, "user_1", items[0].ID)
		if err != nil {
			t.Fatalf("unexpected error removing item: %v", err)
		}
		if len(removed) != 0 {
			t.Errorf("expected 0 items after removal, got %d", len(removed))
		}
	})
}
