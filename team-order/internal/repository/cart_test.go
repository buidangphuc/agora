package repository_test

import (
	"context"
	"errors"
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

// Scenario (order-advertised-price): Another buyer's cart is never re-priced.
//
// Interface fixed by these tests: CartRepository gains
//   UpdateItemPrice(ctx context.Context, userID, itemID string, unitPrice int64) error
// on both InMemoryCartRepository and PostgresCartRepository. It changes only
// unit_price of the row matching BOTH userID and itemID; no match (unknown id or
// another user's id) returns ErrCartItemNotFound and changes nothing.
// Postgres variants skip unless TEST_DATABASE_URL/DATABASE_URL is set.

func repriceCases(t *testing.T, repo repository.CartRepository) {
	t.Helper()
	ctx := context.Background()
	b1, b2 := "b1-"+t.Name(), "b2-"+t.Name()
	l := "lst-" + t.Name()
	c1, err := repo.AddItem(ctx, repository.CartItem{UserID: b1, ListingID: l, Quantity: 2, UnitPrice: 500000, Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	c2, err := repo.AddItem(ctx, repository.CartItem{UserID: b2, ListingID: l, Quantity: 1, UnitPrice: 500000, Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	b1Item, b2Item := c1[0], c2[0]

	t.Run("owner re-price changes only unit_price", func(t *testing.T) {
		if err := repo.UpdateItemPrice(ctx, b1, b1Item.ID, 350000); err != nil {
			t.Fatalf("UpdateItemPrice: %v", err)
		}
		got, _ := repo.GetCart(ctx, b1)
		if len(got) != 1 || got[0].UnitPrice != 350000 || got[0].Quantity != 2 || got[0].ListingID != l {
			t.Fatalf("b1 cart = %+v, want unit_price 350000, qty 2", got)
		}
	})
	t.Run("another buyer's line is untouched", func(t *testing.T) {
		got, _ := repo.GetCart(ctx, b2)
		if len(got) != 1 || got[0].UnitPrice != 500000 {
			t.Fatalf("b2 cart = %+v, want unit_price 500000", got)
		}
	})
	t.Run("foreign item id is not found and unchanged", func(t *testing.T) {
		err := repo.UpdateItemPrice(ctx, b1, b2Item.ID, 1)
		if !errors.Is(err, repository.ErrCartItemNotFound) {
			t.Fatalf("err = %v, want ErrCartItemNotFound", err)
		}
		got, _ := repo.GetCart(ctx, b2)
		if got[0].UnitPrice != 500000 {
			t.Fatalf("b2 line re-priced by b1: %+v", got)
		}
	})
	t.Run("unknown item id is not found", func(t *testing.T) {
		if err := repo.UpdateItemPrice(ctx, b1, "nope", 1); !errors.Is(err, repository.ErrCartItemNotFound) {
			t.Fatalf("err = %v, want ErrCartItemNotFound", err)
		}
	})
}

func TestInMemoryCartRepositoryUpdateItemPriceUserScoped(t *testing.T) {
	repriceCases(t, repository.NewInMemoryCartRepository())
}

func TestPostgresCartRepositoryUpdateItemPriceUserScoped(t *testing.T) {
	pool := pgPool(t)
	repriceCases(t, repository.NewPostgresCartRepository(pool))
}
