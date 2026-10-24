package engagement_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

func TestCollectionService(t *testing.T) {
	ctx := context.Background()
	svc := engagement.NewCollectionService(storetest.NewFakeCollectionRepository(), nil)

	// Validation
	if _, err := svc.CreateCollection(ctx, "u1", ""); !errors.Is(err, engagement.ErrEmptyCollectionName) {
		t.Fatalf("expected ErrEmptyCollectionName, got %v", err)
	}
	if _, err := svc.AddToCollection(ctx, "u1", "", "l1"); !errors.Is(err, engagement.ErrEmptyCollectionID) {
		t.Fatalf("expected ErrEmptyCollectionID, got %v", err)
	}
	if _, err := svc.AddToCollection(ctx, "u1", "c1", ""); !errors.Is(err, engagement.ErrEmptyListing) {
		t.Fatalf("expected ErrEmptyListing, got %v", err)
	}

	// Happy path
	col, err := svc.CreateCollection(ctx, "u1", "Wishlist")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if _, err := svc.AddToCollection(ctx, "u1", col.ID, "l1"); err != nil {
		t.Fatalf("AddToCollection: %v", err)
	}
	cols, err := svc.ListCollections(ctx, "u1")
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(cols) != 1 || cols[0].ItemCount != 1 {
		t.Fatalf("unexpected collections: %+v", cols)
	}
	ids, _, total, err := svc.ListCollectionItems(ctx, "u1", col.ID, "", 20)
	if err != nil {
		t.Fatalf("ListCollectionItems: %v", err)
	}
	if total != 1 || len(ids) != 1 || ids[0] != "l1" {
		t.Fatalf("unexpected items: ids=%v total=%d", ids, total)
	}
}
