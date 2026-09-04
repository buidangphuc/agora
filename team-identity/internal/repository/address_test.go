package repository_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-identity/internal/repository"
)

func TestInMemoryAddressRepository(t *testing.T) {
	repo := repository.NewInMemoryAddressRepository()
	ctx := context.Background()

	addr, err := repo.Create(ctx, repository.Address{
		UserID:        "user_1",
		RecipientName: "Test User",
		Phone:         "0901234567",
		City:          "TP.HCM",
		District:      "Quan 1",
		Ward:          "Ben Nghe",
		Street:        "1 Le Duan",
		IsDefault:     true,
	})
	if err != nil {
		t.Fatalf("unexpected error creating address: %v", err)
	}

	got, err := repo.Get(ctx, addr.ID, "user_1")
	if err != nil {
		t.Fatalf("unexpected error getting address: %v", err)
	}
	if got.RecipientName != "Test User" {
		t.Errorf("expected Test User, got %s", got.RecipientName)
	}

	// Update
	got.RecipientName = "Updated User"
	updated, err := repo.Update(ctx, got)
	if err != nil {
		t.Fatalf("unexpected error updating address: %v", err)
	}
	if updated.RecipientName != "Updated User" {
		t.Errorf("expected Updated User")
	}

	// List
	list, err := repo.List(ctx, "user_1")
	if err != nil || len(list) != 1 {
		t.Errorf("expected 1 address in list")
	}

	// SetDefault
	def, err := repo.SetDefault(ctx, addr.ID, "user_1")
	if err != nil || !def.IsDefault {
		t.Errorf("expected default address")
	}

	// Delete
	err = repo.Delete(ctx, addr.ID, "user_1")
	if err != nil {
		t.Errorf("unexpected error deleting address")
	}
}
