package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/store"
)

type AddressHandler struct {
	identityv1.UnimplementedAddressServiceServer

	repo   store.AddressRepository
	logger *slog.Logger
}

func NewAddressHandler(repo store.AddressRepository, logger *slog.Logger) *AddressHandler {
	return &AddressHandler{repo: repo, logger: orDefault(logger)}
}

func (h *AddressHandler) ListAddresses(ctx context.Context, _ *identityv1.ListAddressesRequest) (*identityv1.ListAddressesResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	items, err := h.repo.List(ctx, p.ID)
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	wire := make([]*identityv1.Address, 0, len(items))
	for _, a := range items {
		wire = append(wire, toWireAddress(a))
	}
	return &identityv1.ListAddressesResponse{Addresses: wire}, nil
}

func (h *AddressHandler) CreateAddress(ctx context.Context, req *identityv1.CreateAddressRequest) (*identityv1.CreateAddressResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetRecipientName() == "" || req.GetPhone() == "" || req.GetStreet() == "" || req.GetCity() == "" {
		return nil, status.Error(codes.InvalidArgument, "recipient_name, phone, street, and city are required")
	}
	created, err := h.repo.Create(ctx, store.Address{
		UserID:        p.ID,
		RecipientName: req.GetRecipientName(),
		Phone:         req.GetPhone(),
		Street:        req.GetStreet(),
		Ward:          req.GetWard(),
		District:      req.GetDistrict(),
		City:          req.GetCity(),
		IsDefault:     req.GetIsDefault(),
	})
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.CreateAddressResponse{Address: toWireAddress(created)}, nil
}

func (h *AddressHandler) UpdateAddress(ctx context.Context, req *identityv1.UpdateAddressRequest) (*identityv1.UpdateAddressResponse, error) {
	p, err := auth.RequireUser(ctx)
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
		UserID:        p.ID,
		RecipientName: req.GetRecipientName(),
		Phone:         req.GetPhone(),
		Street:        req.GetStreet(),
		Ward:          req.GetWard(),
		District:      req.GetDistrict(),
		City:          req.GetCity(),
		IsDefault:     req.GetIsDefault(),
	})
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.UpdateAddressResponse{Address: toWireAddress(updated)}, nil
}

func (h *AddressHandler) DeleteAddress(ctx context.Context, req *identityv1.DeleteAddressRequest) (*identityv1.DeleteAddressResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}
	if err := h.repo.Delete(ctx, req.GetId(), p.ID); err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.DeleteAddressResponse{}, nil
}

func (h *AddressHandler) SetDefaultAddress(ctx context.Context, req *identityv1.SetDefaultAddressRequest) (*identityv1.SetDefaultAddressResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}
	updated, err := h.repo.SetDefault(ctx, req.GetId(), p.ID)
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
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
