package store_test

import (
	"sort"
	"testing"
)

// Scenario: Featured lookup uses one batched read
func TestGetByIDsReturnsExistingAndOmitsUnknown(t *testing.T) {
	ctx := t.Context()
	repo, _ := newPGRepo(ctx, t)
	for _, id := range []string{"b-1", "b-2", "b-3", "b-4"} {
		seedPGListing(ctx, t, repo, id, 1)
	}
	got, err := repo.GetByIDs(ctx, []string{"b-3", "missing", "b-1", "b-3"})
	if err != nil {
		t.Fatalf("GetByIDs: %v", err)
	}
	ids := make([]string, 0, len(got))
	for _, l := range got {
		ids = append(ids, l.ID)
	}
	sort.Strings(ids)
	if len(ids) != 2 || ids[0] != "b-1" || ids[1] != "b-3" {
		t.Fatalf("got ids %v, want [b-1 b-3]", ids)
	}
}

// Scenario: Featured lookup uses one batched read
func TestGetByIDsReturnsOwnerAndFullRow(t *testing.T) {
	ctx := t.Context()
	repo, pool := newPGRepo(ctx, t)
	seedPGListing(ctx, t, repo, "own-1", 1)
	if _, err := pool.Exec(ctx, `UPDATE listings SET seller_id = 'seller-x' WHERE id = 'own-1'`); err != nil {
		t.Fatalf("set owner: %v", err)
	}
	got, err := repo.GetByIDs(ctx, []string{"own-1"})
	if err != nil || len(got) != 1 {
		t.Fatalf("GetByIDs = %v, %v", got, err)
	}
	if got["own-1"].SellerID != "seller-x" || got["own-1"].Status != "published" {
		t.Fatalf("row not fully populated: %+v", got["own-1"])
	}
}

// Scenario: Featured lookup uses one batched read
func TestGetByIDsEmptyInput(t *testing.T) {
	ctx := t.Context()
	repo, _ := newPGRepo(ctx, t)
	got, err := repo.GetByIDs(ctx, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("GetByIDs(nil) = %v, %v; want empty, nil", got, err)
	}
}
