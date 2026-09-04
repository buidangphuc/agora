package handler

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/interceptor"
	"github.com/buidangphuc/team-identity/internal/repository"
)

type AddressHandler struct {
	identityv1.UnimplementedAddressServiceServer

	repo   repository.AddressRepository
	logger *slog.Logger
}

func NewAddressHandler(repo repository.AddressRepository, logger *slog.Logger) *AddressHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &AddressHandler{
		repo:   repo,
		logger: logger,
	}
}

func (h *AddressHandler) ListAddresses(
	ctx context.Context,
	_ *identityv1.ListAddressesRequest,
) (*identityv1.ListAddressesResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	items, err := h.repo.List(ctx, principal.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list addresses: %v", err)
	}
	wire := make([]*identityv1.Address, 0, len(items))
	for _, a := range items {
		wire = append(wire, toWireAddress(a))
	}
	return &identityv1.ListAddressesResponse{Addresses: wire}, nil
}

func (h *AddressHandler) CreateAddress(
	ctx context.Context,
	req *identityv1.CreateAddressRequest,
) (*identityv1.CreateAddressResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetRecipientName() == "" || req.GetPhone() == "" || req.GetStreet() == "" || req.GetCity() == "" {
		return nil, status.Error(codes.InvalidArgument, "recipient_name, phone, street, and city are required")
	}

	created, err := h.repo.Create(ctx, repository.Address{
		UserID:        principal.GetId(),
		RecipientName: req.GetRecipientName(),
		Phone:         req.GetPhone(),
		Street:        req.GetStreet(),
		Ward:          req.GetWard(),
		District:      req.GetDistrict(),
		City:          req.GetCity(),
		IsDefault:     req.GetIsDefault(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create address: %v", err)
	}
	return &identityv1.CreateAddressResponse{Address: toWireAddress(created)}, nil
}

func (h *AddressHandler) UpdateAddress(
	ctx context.Context,
	req *identityv1.UpdateAddressRequest,
) (*identityv1.UpdateAddressResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}
	if req.GetRecipientName() == "" || req.GetPhone() == "" || req.GetStreet() == "" || req.GetCity() == "" {
		return nil, status.Error(codes.InvalidArgument, "recipient_name, phone, street, and city are required")
	}

	updated, err := h.repo.Update(ctx, repository.Address{
		ID:            req.GetId(),
		UserID:        principal.GetId(),
		RecipientName: req.GetRecipientName(),
		Phone:         req.GetPhone(),
		Street:        req.GetStreet(),
		Ward:          req.GetWard(),
		District:      req.GetDistrict(),
		City:          req.GetCity(),
		IsDefault:     req.GetIsDefault(),
	})
	if err != nil {
		if errors.Is(err, repository.ErrAddressNotFound) {
			return nil, status.Error(codes.NotFound, "address not found")
		}
		return nil, status.Errorf(codes.Internal, "update address: %v", err)
	}
	return &identityv1.UpdateAddressResponse{Address: toWireAddress(updated)}, nil
}

func (h *AddressHandler) DeleteAddress(
	ctx context.Context,
	req *identityv1.DeleteAddressRequest,
) (*identityv1.DeleteAddressResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}
	if err := h.repo.Delete(ctx, req.GetId(), principal.GetId()); err != nil {
		if errors.Is(err, repository.ErrAddressNotFound) {
			return nil, status.Error(codes.NotFound, "address not found")
		}
		return nil, status.Errorf(codes.Internal, "delete address: %v", err)
	}
	return &identityv1.DeleteAddressResponse{}, nil
}

func (h *AddressHandler) SetDefaultAddress(
	ctx context.Context,
	req *identityv1.SetDefaultAddressRequest,
) (*identityv1.SetDefaultAddressResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}
	updated, err := h.repo.SetDefault(ctx, req.GetId(), principal.GetId())
	if err != nil {
		if errors.Is(err, repository.ErrAddressNotFound) {
			return nil, status.Error(codes.NotFound, "address not found")
		}
		return nil, status.Errorf(codes.Internal, "set default address: %v", err)
	}
	return &identityv1.SetDefaultAddressResponse{Address: toWireAddress(updated)}, nil
}

func toWireAddress(a repository.Address) *identityv1.Address {
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
