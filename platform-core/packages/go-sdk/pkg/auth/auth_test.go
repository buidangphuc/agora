package auth_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
)

func TestRequireScopes(t *testing.T) {
	ctx := context.Background()

	// 1. Missing principal -> Unauthenticated
	err := auth.RequireScopes(ctx, "listing.write")
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}

	// 2. Principal with insufficient scopes -> PermissionDenied
	md := metadata.Pairs(
		"x-principal-id", "user_123",
		"x-principal-type", "buyer",
		"x-principal-scopes", "listing.read",
	)
	incomingCtx := metadata.NewIncomingContext(ctx, md)
	err = auth.RequireScopes(incomingCtx, "listing.write")
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", err)
	}

	// 3. Principal with required scope -> OK
	mdSeller := metadata.Pairs(
		"x-principal-id", "seller_456",
		"x-principal-type", "seller",
		"x-principal-scopes", "listing.read,listing.write",
	)
	sellerCtx := metadata.NewIncomingContext(ctx, mdSeller)
	if err := auth.RequireScopes(sellerCtx, "listing.write"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	// 4. Admin scope bypasses any required scope -> OK
	mdAdmin := metadata.Pairs(
		"x-principal-id", "admin_001",
		"x-principal-type", "admin",
		"x-principal-scopes", "admin",
	)
	adminCtx := metadata.NewIncomingContext(ctx, mdAdmin)
	if err := auth.RequireScopes(adminCtx, "any.super.secret.scope"); err != nil {
		t.Fatalf("expected admin bypass, got %v", err)
	}
}
