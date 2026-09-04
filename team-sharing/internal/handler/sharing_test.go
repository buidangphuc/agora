package handler_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sharingv1 "github.com/buidangphuc/team-sharing/generated/platform/sharing/v1"
	"github.com/buidangphuc/team-sharing/internal/handler"
	"github.com/buidangphuc/team-sharing/internal/repository"
	"github.com/buidangphuc/team-sharing/internal/service"
)

func newHandler() *handler.SharingHandler {
	return handler.NewSharingHandler(service.NewShareService(repository.NewInMemoryShareLinkRepo()))
}

// The full gRPC surface: CreateShareLink returns a short code, ResolveShareLink
// echoes the target, UTM and OG meta back through the proto messages.
func TestHandlerCreateThenResolve(t *testing.T) {
	h := newHandler()
	ctx := context.Background()

	createResp, err := h.CreateShareLink(ctx, &sharingv1.CreateShareLinkRequest{
		TargetType: "listing",
		TargetId:   "listing_42",
		Utm:        map[string]string{"utm_campaign": "tet"},
	})
	if err != nil {
		t.Fatalf("CreateShareLink: %v", err)
	}
	if createResp.GetShortCode() == "" {
		t.Fatalf("expected non-empty short code")
	}

	resolveResp, err := h.ResolveShareLink(ctx, &sharingv1.ResolveShareLinkRequest{
		ShortCode: createResp.GetShortCode(),
	})
	if err != nil {
		t.Fatalf("ResolveShareLink: %v", err)
	}
	if resolveResp.GetTargetType() != "listing" || resolveResp.GetTargetId() != "listing_42" {
		t.Fatalf("target mismatch: %+v", resolveResp)
	}
	if resolveResp.GetUtm()["utm_campaign"] != "tet" {
		t.Fatalf("utm mismatch: %+v", resolveResp.GetUtm())
	}
	if resolveResp.GetOgMeta().GetTitle() == "" {
		t.Fatalf("expected OG meta title in response")
	}
}

// An unknown short code maps to gRPC NotFound.
func TestHandlerResolveUnknownIsNotFound(t *testing.T) {
	h := newHandler()

	_, err := h.ResolveShareLink(context.Background(), &sharingv1.ResolveShareLinkRequest{ShortCode: "nope"})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("expected NotFound, got %v (err=%v)", got, err)
	}
}

// Missing target fields map to gRPC InvalidArgument.
func TestHandlerCreateInvalidArgument(t *testing.T) {
	h := newHandler()

	_, err := h.CreateShareLink(context.Background(), &sharingv1.CreateShareLinkRequest{TargetType: "", TargetId: "x"})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v (err=%v)", got, err)
	}
}
