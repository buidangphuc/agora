package handler_test

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/handler"
	"github.com/buidangphuc/team-identity/internal/interceptor"
	"github.com/buidangphuc/team-identity/internal/repository"
)

func ctxWith(t commonv1.PrincipalType, scopes ...string) context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{
		Id: "p-1", Type: t, Scopes: scopes,
	})
}

func newProfileHandler(t *testing.T) *handler.ProfileHandler {
	t.Helper()
	repo := repository.NewInMemoryUserRepository()
	for _, u := range []repository.User{
		{ID: "u1", Username: "an", PasswordHash: "secret-hash"},
		{ID: "u2", Username: "binh", PasswordHash: "secret-hash"},
	} {
		if _, err := repo.Create(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
	return handler.NewProfileHandler(repo, nil)
}

func TestGetPublicProfiles_ServicePrincipal(t *testing.T) {
	h := newProfileHandler(t)
	ctx := ctxWith(commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE, "identity.read")
	res, err := h.GetPublicProfiles(ctx, &identityv1.GetPublicProfilesRequest{UserIds: []string{"u1", "u2", "u1", "missing", ""}})
	if err != nil {
		t.Fatalf("GetPublicProfiles: %v", err)
	}
	got := map[string]string{}
	for _, p := range res.GetProfiles() {
		got[p.GetUserId()] = p.GetDisplayName()
	}
	if len(got) != 2 || got["u1"] != "an" || got["u2"] != "binh" {
		t.Fatalf("profiles = %v, want u1=an u2=binh only", got)
	}
}

func TestGetPublicProfiles_UserPrincipalDenied(t *testing.T) {
	h := newProfileHandler(t)
	// A user token that claims the scope is still rejected: type must be service.
	for _, ctx := range []context.Context{
		ctxWith(commonv1.PrincipalType_PRINCIPAL_TYPE_USER, "identity.read", "admin"),
		ctxWith(commonv1.PrincipalType_PRINCIPAL_TYPE_USER),
	} {
		_, err := h.GetPublicProfiles(ctx, &identityv1.GetPublicProfilesRequest{UserIds: []string{"u1"}})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("user principal: code = %v, want PermissionDenied", status.Code(err))
		}
	}
}

func TestGetPublicProfiles_ServiceMissingScopeDenied(t *testing.T) {
	h := newProfileHandler(t)
	ctx := ctxWith(commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE, "listing.read")
	_, err := h.GetPublicProfiles(ctx, &identityv1.GetPublicProfilesRequest{UserIds: []string{"u1"}})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestGetPublicProfiles_NoPrincipalUnauthenticated(t *testing.T) {
	h := newProfileHandler(t)
	_, err := h.GetPublicProfiles(context.Background(), &identityv1.GetPublicProfilesRequest{UserIds: []string{"u1"}})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestGetPublicProfiles_MaxIDs(t *testing.T) {
	h := newProfileHandler(t)
	ctx := ctxWith(commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE, "identity.read")
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = fmt.Sprintf("u-%d", i)
	}
	_, err := h.GetPublicProfiles(ctx, &identityv1.GetPublicProfilesRequest{UserIds: ids})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("101 ids: code = %v, want InvalidArgument", status.Code(err))
	}
	if _, err := h.GetPublicProfiles(ctx, &identityv1.GetPublicProfilesRequest{UserIds: ids[:100]}); err != nil {
		t.Fatalf("100 ids should pass: %v", err)
	}
}
