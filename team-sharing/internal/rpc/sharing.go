// Package rpc is the transport layer: it decodes requests, reads the caller,
// calls the share-link domain and maps its errors to gRPC status. It holds no
// business logic.
package rpc

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	sharingv1 "github.com/buidangphuc/team-sharing/generated/platform/sharing/v1"
	"github.com/buidangphuc/team-sharing/internal/sharelink"
)

// Handler implements platform.sharing.v1.SharingService.
type Handler struct {
	sharingv1.UnimplementedSharingServiceServer
	svc    *sharelink.Service
	logger *slog.Logger
	errs   grpcx.ErrorTable
}

// NewHandler returns a Handler that maps domain errors with its error table.
// Each rule keeps the message clients saw before the table (the error text).
func NewHandler(svc *sharelink.Service, logger *slog.Logger) (*Handler, error) {
	errs, err := grpcx.NewErrorTable(
		grpcx.Rule{Target: sharelink.ErrEmptyTargetType, Code: codes.InvalidArgument},
		grpcx.Rule{Target: sharelink.ErrEmptyTargetID, Code: codes.InvalidArgument},
		grpcx.Rule{Target: sharelink.ErrEmptyShortCode, Code: codes.InvalidArgument},
		grpcx.Rule{Target: sharelink.ErrShareLinkNotFound, Code: codes.NotFound},
	)
	if err != nil {
		return nil, fmt.Errorf("error table: %w", err)
	}
	return &Handler{svc: svc, logger: logger, errs: errs}, nil
}

// Register adds the sharing service to s.
func Register(s grpc.ServiceRegistrar, h *Handler) {
	sharingv1.RegisterSharingServiceServer(s, h)
}

// mapErr turns a domain or store error into the right gRPC status; unknown
// errors are logged and become Internal with a fixed message.
func (h *Handler) mapErr(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, h.errs)
}

// callerID returns the authenticated caller id, or "" for anonymous callers.
// Creating a link does not require authentication, so this does not use
// auth.RequirePrincipal.
func callerID(ctx context.Context) string {
	p, ok := auth.FromContext(ctx)
	if !ok || p.Anonymous() {
		return ""
	}
	return p.ID
}

// CreateShareLink mints a short code. Anonymous callers are allowed; an
// authenticated caller is stored as the creator.
func (h *Handler) CreateShareLink(ctx context.Context, req *sharingv1.CreateShareLinkRequest) (*sharingv1.CreateShareLinkResponse, error) {
	link, err := h.svc.CreateShareLink(ctx, callerID(ctx), req.GetTargetType(), req.GetTargetId(), req.GetUtm())
	if err != nil {
		return nil, h.mapErr(ctx, err)
	}
	return &sharingv1.CreateShareLinkResponse{ShortCode: link.ShortCode}, nil
}

// ResolveShareLink returns the link for a short code and counts the click.
// It is anonymous by design: anyone with the code can unfurl it.
func (h *Handler) ResolveShareLink(ctx context.Context, req *sharingv1.ResolveShareLinkRequest) (*sharingv1.ResolveShareLinkResponse, error) {
	link, err := h.svc.ResolveShareLink(ctx, req.GetShortCode())
	if err != nil {
		return nil, h.mapErr(ctx, err)
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
