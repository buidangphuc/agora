package handler

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sharingv1 "github.com/buidangphuc/team-sharing/generated/platform/sharing/v1"
	"github.com/buidangphuc/team-sharing/internal/interceptor"
	"github.com/buidangphuc/team-sharing/internal/service"
)

type SharingHandler struct {
	sharingv1.UnimplementedSharingServiceServer
	svc *service.ShareService
}

func NewSharingHandler(svc *service.ShareService) *SharingHandler {
	return &SharingHandler{svc: svc}
}

// mapErr turns a service/repository error into the right gRPC status.
func mapErr(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrEmptyTargetType),
		errors.Is(err, service.ErrEmptyTargetID),
		errors.Is(err, service.ErrEmptyShortCode):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrShareLinkNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		slog.ErrorContext(ctx, "internal error", "error", err)
		return status.Error(codes.Internal, "internal error")
	}
}

func (h *SharingHandler) CreateShareLink(ctx context.Context, req *sharingv1.CreateShareLinkRequest) (*sharingv1.CreateShareLinkResponse, error) {
	link, err := h.svc.CreateShareLink(ctx, interceptor.CallerID(ctx), req.GetTargetType(), req.GetTargetId(), req.GetUtm())
	if err != nil {
		return nil, mapErr(ctx, err)
	}
	return &sharingv1.CreateShareLinkResponse{ShortCode: link.ShortCode}, nil
}

func (h *SharingHandler) ResolveShareLink(ctx context.Context, req *sharingv1.ResolveShareLinkRequest) (*sharingv1.ResolveShareLinkResponse, error) {
	link, err := h.svc.ResolveShareLink(ctx, req.GetShortCode())
	if err != nil {
		return nil, mapErr(ctx, err)
	}
	return &sharingv1.ResolveShareLinkResponse{
		TargetType: link.TargetType,
		TargetId:   link.TargetID,
		Utm:        link.UTM,
		OgMeta: &sharingv1.OgMeta{
			Title:       link.OgTitle,
			Description: link.OgDescription,
			ImageUrl:    link.OgImageURL,
		},
		ClickCount: link.ClickCount,
	}, nil
}
