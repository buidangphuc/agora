// Package handler is the gRPC transport adapter: it decodes requests,
// enforces authentication, calls the service and maps domain errors to gRPC
// status. It holds no business logic.
//
// The service is described by a hand-built grpc.ServiceDesc over well-known
// protobuf messages so the template needs no code generation. Replace it with
// the generated registration function once the service has its own proto.
package handler

import (
	"context"
	"fmt"
	"log/slog"

	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"

	"github.com/your-org/team-service/internal/repository"
	"github.com/your-org/team-service/internal/service"
)

// ListingHandler serves the listing methods.
type ListingHandler struct {
	logger *slog.Logger
	errs   grpcx.ErrorTable
	svc    *service.ListingService
}

// NewListingHandler builds the handler around the business service.
func NewListingHandler(logger *slog.Logger, svc *service.ListingService) (*ListingHandler, error) {
	errs, err := grpcx.NewErrorTable(grpcx.Rule{Target: repository.ErrNotFound, Code: codes.NotFound})
	if err != nil {
		return nil, fmt.Errorf("error table: %w", err)
	}
	return &ListingHandler{logger: logger, errs: errs, svc: svc}, nil
}

// GetListing returns the title of the listing with the given id.
func (h *ListingHandler) GetListing(ctx context.Context, in *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	if _, err := auth.RequirePrincipal(ctx); err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, h.errs)
	}
	if in.GetValue() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	l, err := h.svc.Get(ctx, in.GetValue())
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, h.errs)
	}
	return wrapperspb.String(l.Title), nil
}

// Register adds the listing service to s.
func Register(s grpc.ServiceRegistrar, h *ListingHandler) {
	s.RegisterService(&serviceDesc, h)
}

type listingServer interface {
	GetListing(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error)
}

var serviceDesc = grpc.ServiceDesc{
	ServiceName: "team.v1.Listing",
	HandlerType: (*listingServer)(nil),
	Methods:     []grpc.MethodDesc{{MethodName: "GetListing", Handler: getListingHandler}},
}

//nolint:revive // signature fixed by grpc.MethodHandler
func getListingHandler(srv any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
	in := new(wrapperspb.StringValue)
	if err := dec(in); err != nil {
		return nil, err
	}
	if ic == nil {
		return srv.(listingServer).GetListing(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/team.v1.Listing/GetListing"}
	return ic(ctx, in, info, func(ctx context.Context, req any) (any, error) {
		return srv.(listingServer).GetListing(ctx, req.(*wrapperspb.StringValue))
	})
}
