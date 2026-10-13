package rpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/store"
)

// AddressHandler implements identityv1.AddressServiceServer; every call is scoped to the caller.
type AddressHandler struct {
	identityv1.UnimplementedAddressServiceServer

	repo   store.AddressRepository
	logger *slog.Logger
}

// NewAddressHandler builds the handler over an address repository; a nil logger means slog.Default().
func NewAddressHandler(repo store.AddressRepository, logger *slog.Logger) *AddressHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &AddressHandler{
		repo:   repo,
		logger: logger,
	}
}

// ListAddresses returns the caller's addresses.
func (h *AddressHandler) ListAddresses(
	ctx context.Context,
	_ *identityv1.ListAddressesRequest,
) (*identityv1.ListAddressesResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	items, err := h.repo.List(ctx, principal.ID)
	if err != nil {
		return nil, internalErr(ctx, h.logger, "list addresses", err)
	}
	wire := make([]*identityv1.Address, 0, len(items))
	for _, a := range items {
		wire = append(wire, toWireAddress(a))
	}
	return &identityv1.ListAddressesResponse{Addresses: wire}, nil
}

// CreateAddress stores a new address for the caller.
func (h *AddressHandler) CreateAddress(
	ctx context.Context,
	req *identityv1.CreateAddressRequest,
) (*identityv1.CreateAddressResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetRecipientName() == "" || req.GetPhone() == "" || req.GetStreet() == "" || req.GetCity() == "" {
		return nil, status.Error(codes.InvalidArgument, "recipient_name, phone, street, and city are required")
	}

	created, err := h.repo.Create(ctx, store.Address{
		UserID:        principal.ID,
		RecipientName: req.GetRecipientName(),
		Phone:         req.GetPhone(),
		Street:        req.GetStreet(),
		Ward:          req.GetWard(),
		District:      req.GetDistrict(),
		City:          req.GetCity(),
		IsDefault:     req.GetIsDefault(),
	})
	if err != nil {
		return nil, internalErr(ctx, h.logger, "create address", err)
	}
	return &identityv1.CreateAddressResponse{Address: toWireAddress(created)}, nil
}

// UpdateAddress changes one of the caller's addresses.
func (h *AddressHandler) UpdateAddress(
	ctx context.Context,
	req *identityv1.UpdateAddressRequest,
) (*identityv1.UpdateAddressResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}
	if req.GetRecipientName() == "" || req.GetPhone() == "" || req.GetStreet() == "" || req.GetCity() == "" {
		return nil, status.Error(codes.InvalidArgument, "recipient_name, phone, street, and city are required")
	}

	updated, err := h.repo.Update(ctx, store.Address{
		ID:            req.GetId(),
		UserID:        principal.ID,
		RecipientName: req.GetRecipientName(),
		Phone:         req.GetPhone(),
		Street:        req.GetStreet(),
		Ward:          req.GetWard(),
		District:      req.GetDistrict(),
		City:          req.GetCity(),
		IsDefault:     req.GetIsDefault(),
	})
	if err != nil {
		if errors.Is(err, store.ErrAddressNotFound) {
			return nil, status.Error(codes.NotFound, "address not found")
		}
		return nil, internalErr(ctx, h.logger, "update address", err)
	}
	return &identityv1.UpdateAddressResponse{Address: toWireAddress(updated)}, nil
}

// DeleteAddress removes one of the caller's addresses.
func (h *AddressHandler) DeleteAddress(
	ctx context.Context,
	req *identityv1.DeleteAddressRequest,
) (*identityv1.DeleteAddressResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}
	if err := h.repo.Delete(ctx, req.GetId(), principal.ID); err != nil {
		if errors.Is(err, store.ErrAddressNotFound) {
			return nil, status.Error(codes.NotFound, "address not found")
		}
		return nil, internalErr(ctx, h.logger, "delete address", err)
	}
	return &identityv1.DeleteAddressResponse{}, nil
}

// SetDefaultAddress makes one of the caller's addresses the default.
func (h *AddressHandler) SetDefaultAddress(
	ctx context.Context,
	req *identityv1.SetDefaultAddressRequest,
) (*identityv1.SetDefaultAddressResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}
	updated, err := h.repo.SetDefault(ctx, req.GetId(), principal.ID)
	if err != nil {
		if errors.Is(err, store.ErrAddressNotFound) {
			return nil, status.Error(codes.NotFound, "address not found")
		}
		return nil, internalErr(ctx, h.logger, "set default address", err)
	}
	return &identityv1.SetDefaultAddressResponse{Address: toWireAddress(updated)}, nil
}

func toWireAddress(a store.Address) *identityv1.Address {
	return &identityv1.Address{
		Id:            a.ID,
		UserId:        a.UserID,
		RecipientName: a.RecipientName,
		Phone:         a.Phone,
		Street:        a.Street,
		Ward:          a.Ward,
		District:      a.District,
		City:          a.City,
		IsDefault:     a.IsDefault,
	}
}
