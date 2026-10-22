package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	sharingv1 "github.com/buidangphuc/team-sharing/generated/platform/sharing/v1"
)

var createReq = &sharingv1.CreateShareLinkRequest{TargetType: "listing", TargetId: "l1"}

// Scenario: The creator is the forwarded principal.
func TestHandlerCreateAttributesPrincipal(t *testing.T) {
	repo := newFakeRepo()
	h := newHandler(t, repo)
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "user-1", Type: auth.TypeUser})
	resp, err := h.CreateShareLink(ctx, createReq)
	if err != nil {
		t.Fatal(err)
	}
	if got := repo.creator(t, resp.GetShortCode()); got != "user-1" {
		t.Fatalf("created_by = %q", got)
	}
}

// Scenario: anonymous callers are accepted and store an empty creator.
func TestCreateAnonymousStoresEmptyCreator(t *testing.T) {
	repo := newFakeRepo()
	h := newHandler(t, repo)
	for name, ctx := range map[string]context.Context{
		"gateway anonymous": auth.WithPrincipal(context.Background(), auth.Principal{ID: "anonymous", Type: auth.TypeAnonymous}),
		"no principal":      context.Background(),
	} {
		resp, err := h.CreateShareLink(ctx, createReq)
		if err != nil {
			t.Fatalf("%s: anonymous create should be allowed: %v", name, err)
		}
		if got := repo.creator(t, resp.GetShortCode()); got != "" {
			t.Fatalf("%s: created_by = %q", name, got)
		}
	}
}

// Scenario: the creator is never taken from the legacy x-user-id header.
func TestLegacyUserIDHeaderIgnored(t *testing.T) {
	repo := newFakeRepo()
	client := dial(t, newHandler(t, repo))
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-user-id", "attacker")
	resp, err := client.CreateShareLink(ctx, createReq)
	if err != nil {
		t.Fatal(err)
	}
	if got := repo.creator(t, resp.GetShortCode()); got != "" {
		t.Fatalf("created_by = %q, want empty", got)
	}
}

// Scenario: the creator is the forwarded principal (metadata through the SDK interceptor).
func TestForwardedPrincipalIsTrusted(t *testing.T) {
	repo := newFakeRepo()
	client := dial(t, newHandler(t, repo))
	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"x-principal-id", "user-1", "x-principal-type", "user", "x-principal-scopes", "a, b")
	resp, err := client.CreateShareLink(ctx, createReq)
	if err != nil {
		t.Fatal(err)
	}
	if got := repo.creator(t, resp.GetShortCode()); got != "user-1" {
		t.Fatalf("created_by = %q", got)
	}
}

// Scenario: anonymous callers are accepted; resolve needs no principal.
func TestResolveRequiresNoPrincipal(t *testing.T) {
	repo := newFakeRepo()
	client := dial(t, newHandler(t, repo))
	created, err := client.CreateShareLink(context.Background(), createReq)
	if err != nil {
		t.Fatal(err)
	}
	anon := metadata.AppendToOutgoingContext(context.Background(),
		"x-principal-id", "anonymous", "x-principal-type", "anonymous")
	for name, ctx := range map[string]context.Context{"no metadata": context.Background(), "anonymous": anon} {
		resp, err := client.ResolveShareLink(ctx, &sharingv1.ResolveShareLinkRequest{ShortCode: created.GetShortCode()})
		if err != nil || resp.GetTargetId() != "l1" {
			t.Fatalf("%s: resp=%+v err=%v", name, resp, err)
		}
	}
}
