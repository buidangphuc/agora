package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-engagement/internal/repository"
)

func TestInMemoryCollectionRepository(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryCollectionRepository()

	// 1. Create a collection
	col, err := repo.CreateCollection(ctx, "user-1", "Nhà đẹp")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if col.ID == "" {
		t.Fatal("expected generated collection id")
	}
	if col.Name != "Nhà đẹp" || col.UserID != "user-1" {
		t.Fatalf("unexpected collection: %+v", col)
	}

	// 2. Add items (idempotent)
	added, err := repo.AddToCollection(ctx, "user-1", col.ID, "listing-a")
	if err != nil || !added {
		t.Fatalf("AddToCollection first: added=%v err=%v", added, err)
	}
	added, err = repo.AddToCollection(ctx, "user-1", col.ID, "listing-a")
	if err != nil {
		t.Fatalf("AddToCollection idempotent: %v", err)
	}
	if added {
		t.Fatal("expected idempotent add to report added=false")
	}
	if _, err := repo.AddToCollection(ctx, "user-1", col.ID, "listing-b"); err != nil {
		t.Fatalf("AddToCollection b: %v", err)
	}

	// 3. ListCollections shows item_count
	cols, err := repo.ListCollections(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(cols) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(cols))
	}
	if cols[0].ItemCount != 2 {
		t.Fatalf("expected item_count 2, got %d", cols[0].ItemCount)
	}

	// 4. ListCollectionItems
	ids, _, total, err := repo.ListCollectionItems(ctx, "user-1", col.ID, "", 50)
	if err != nil {
		t.Fatalf("ListCollectionItems: %v", err)
	}
	if total != 2 || len(ids) != 2 {
		t.Fatalf("expected 2 items, got total=%d len=%d", total, len(ids))
	}
	if ids[0] != "listing-a" || ids[1] != "listing-b" {
		t.Fatalf("expected sorted [listing-a listing-b], got %v", ids)
	}

	// 5. Remove item (idempotent)
	removed, err := repo.RemoveFromCollection(ctx, "user-1", col.ID, "listing-a")
	if err != nil || !removed {
		t.Fatalf("RemoveFromCollection: removed=%v err=%v", removed, err)
	}
	removed, err = repo.RemoveFromCollection(ctx, "user-1", col.ID, "listing-a")
	if err != nil {
		t.Fatalf("RemoveFromCollection idempotent: %v", err)
	}
	if removed {
		t.Fatal("expected idempotent remove to report removed=false")
	}

	// 6. Ownership: another user cannot touch the collection
	if _, err := repo.AddToCollection(ctx, "user-2", col.ID, "listing-x"); !errors.Is(err, repository.ErrCollectionNotFound) {
		t.Fatalf("expected ErrCollectionNotFound for foreign add, got %v", err)
	}
	if _, _, _, err := repo.ListCollectionItems(ctx, "user-2", col.ID, "", 50); !errors.Is(err, repository.ErrCollectionNotFound) {
		t.Fatalf("expected ErrCollectionNotFound for foreign list, got %v", err)
	}
	// user-2 sees none of user-1's collections
	if cols2, _ := repo.ListCollections(ctx, "user-2"); len(cols2) != 0 {
		t.Fatalf("expected user-2 to have 0 collections, got %d", len(cols2))
	}
}

func TestInMemoryCollectionItemsPagination(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryCollectionRepository()
	col, _ := repo.CreateCollection(ctx, "user-1", "Big")
	for _, id := range []string{"l1", "l2", "l3", "l4"} {
		if _, err := repo.AddToCollection(ctx, "user-1", col.ID, id); err != nil {
			t.Fatalf("add %s: %v", id, err)
		}
	}

	ids, next, total, err := repo.ListCollectionItems(ctx, "user-1", col.ID, "", 2)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if total != 4 {
		t.Fatalf("expected total 4, got %d", total)
	}
	if len(ids) != 2 || next == "" {
		t.Fatalf("expected 2 ids and a cursor, got ids=%v next=%q", ids, next)
	}

	ids2, _, _, err := repo.ListCollectionItems(ctx, "user-1", col.ID, next, 2)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(ids2) != 2 {
		t.Fatalf("expected 2 ids on page 2, got %v", ids2)
	}
	if ids2[0] <= ids[len(ids)-1] {
		t.Fatalf("expected page 2 to continue after cursor, got %v then %v", ids, ids2)
	}
}
