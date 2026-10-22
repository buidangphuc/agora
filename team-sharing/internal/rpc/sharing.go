package rpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	sharingv1 "github.com/buidangphuc/team-sharing/generated/platform/sharing/v1"
	"github.com/buidangphuc/team-sharing/internal/sharelink"
)

type Handler struct {
	sharingv1.UnimplementedSharingServiceServer
	svc    *sharelink.Service
	logger *slog.Logger
}

func NewHandler(svc *sharelink.Service, logger *slog.Logger) (*Handler, error) {
	if svc == nil {
		return nil, errors.New("rpc: service is required")
	}
	if logger == nil {
		return nil, errors.New("rpc: logger is required")
	}
	return &Handler{svc: svc, logger: logger}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	sharingv1.RegisterSharingServiceServer(srv, h)
}

func callerID(ctx context.Context) string {
	p, ok := auth.FromContext(ctx)
	if !ok || p.Anonymous() {
		return ""
	}
	return p.ID
}

func mapErr(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, sharelink.ErrEmptyTargetType),
		errors.Is(err, sharelink.ErrEmptyTargetID),
		errors.Is(err, sharelink.ErrEmptyShortCode):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, sharelink.ErrShareLinkNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		slog.ErrorContext(ctx, "internal error", "error", err)
		return status.Error(codes.Internal, "internal error")
	}
}

func (h *Handler) CreateShareLink(ctx context.Context, req *sharingv1.CreateShareLinkRequest) (*sharingv1.CreateShareLinkResponse, error) {
	link, err := h.svc.CreateShareLink(ctx, callerID(ctx), req.GetTargetType(), req.GetTargetId(), req.GetUtm())
	if err != nil {
		return nil, mapErr(ctx, err)
	}
	return &sharingv1.CreateShareLinkResponse{ShortCode: link.ShortCode}, nil
}

func (h *Handler) ResolveShareLink(ctx context.Context, req *sharingv1.ResolveShareLinkRequest) (*sharingv1.ResolveShareLinkResponse, error) {
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
