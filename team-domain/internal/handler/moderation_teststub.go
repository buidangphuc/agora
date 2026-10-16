package handler

// TEST-STUB (task 8): compile-only placeholders; the implementer deletes this
// file and writes internal/handler/moderation.go.

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
)

func (h *ListingHandler) ModerateListing(context.Context, *listingv1.ModerateListingRequest) (*listingv1.ModerateListingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (h *ListingHandler) ListModerationQueue(context.Context, *listingv1.ListModerationQueueRequest) (*listingv1.ListModerationQueueResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
