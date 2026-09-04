package handler_test

import (
	"context"
	"testing"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/handler"
	"github.com/buidangphuc/team-identity/internal/interceptor"
	"github.com/buidangphuc/team-identity/internal/repository"
)

func TestAddressHandler(t *testing.T) {
	repo := repository.NewInMemoryAddressRepository()
	h := handler.NewAddressHandler(repo, nil)

	principal := &commonv1.Principal{
		Id:     "user_addr_1",
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"identity:read", "identity:write"},
	}
	ctx := interceptor.ContextWithPrincipal(context.Background(), principal)

	t.Run("Create and List Addresses", func(t *testing.T) {
		createRes, err := h.CreateAddress(ctx, &identityv1.CreateAddressRequest{
			RecipientName: "Nguyen Van A",
			Phone:         "0912345678",
			City:          "Ha Noi",
			District:      "Hoan Kiem",
			Ward:          "Trang Tien",
			Street:        "18 Trang Tien",
			IsDefault:     true,
		})
		if err != nil {
			t.Fatalf("unexpected error creating address: %v", err)
		}
		if createRes.Address.RecipientName != "Nguyen Van A" {
			t.Errorf("expected Nguyen Van A, got %s", createRes.Address.RecipientName)
		}

		listRes, err := h.ListAddresses(ctx, &identityv1.ListAddressesRequest{})
		if err != nil {
			t.Fatalf("unexpected error listing addresses: %v", err)
		}
		if len(listRes.Addresses) != 1 {
			t.Fatalf("expected 1 address, got %d", len(listRes.Addresses))
		}

		// Update address
		updateRes, err := h.UpdateAddress(ctx, &identityv1.UpdateAddressRequest{
			Id:            createRes.Address.Id,
			RecipientName: "Nguyen Van B",
			Phone:         "0987654321",
			City:          "Ha Noi",
			District:      "Ba Dinh",
			Ward:          "Kim Ma",
			Street:        "100 Kim Ma",
			IsDefault:     true,
		})
		if err != nil {
			t.Fatalf("unexpected error updating address: %v", err)
		}
		if updateRes.Address.RecipientName != "Nguyen Van B" {
			t.Errorf("expected Nguyen Van B")
		}

		// Set default
		setDefaultRes, err := h.SetDefaultAddress(ctx, &identityv1.SetDefaultAddressRequest{
			Id: createRes.Address.Id,
		})
		if err != nil {
			t.Fatalf("unexpected error setting default address: %v", err)
		}
		if !setDefaultRes.Address.IsDefault {
			t.Errorf("expected isDefault true")
		}

		// Delete address
		_, err = h.DeleteAddress(ctx, &identityv1.DeleteAddressRequest{
			Id: createRes.Address.Id,
		})
		if err != nil {
			t.Fatalf("unexpected error deleting address: %v", err)
		}
	})
}
