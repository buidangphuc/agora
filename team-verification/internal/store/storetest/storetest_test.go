package storetest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/buidangphuc/team-verification/internal/store"
	"github.com/buidangphuc/team-verification/internal/store/storetest"
)

func TestInMemoryFakeKeepsStoreContract(t *testing.T) {
	ctx := context.Background()
	var repo store.KycRepository = storetest.NewInMemoryKycRepo()

	if _, err := repo.GetLatestByUser(ctx, "u-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("latest before create = %v, want ErrNotFound", err)
	}
	if _, err := repo.Create(ctx, &store.Submission{ID: "kyc_1", UserID: "u-1", Status: store.StatusPending}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.UpdateStatus(ctx, "kyc_1", store.StatusVerified, time.Now()); err != nil {
		t.Fatalf("first review: %v", err)
	}
	if _, err := repo.UpdateStatus(ctx, "kyc_1", store.StatusRejected, time.Now()); !errors.Is(err, store.ErrAlreadyReviewed) {
		t.Fatalf("second review = %v, want ErrAlreadyReviewed", err)
	}
	if _, err := repo.UpdateStatus(ctx, "kyc_missing", store.StatusVerified, time.Now()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing review = %v, want ErrNotFound", err)
	}
}
