package rpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/pagination"
	commonv1 "github.com/buidangphuc/team-payment/generated/platform/common/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/store"
)

func sellerAccess(ctx context.Context, requested string, adminRead bool) (string, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return "", err
	}
	isUser := p.Type == auth.TypeUser
	if requested == "" || requested == p.ID {
		if isUser {
			return p.ID, nil
		}
		if requested == "" {
			return "", status.Error(codes.InvalidArgument, "seller_id is required")
		}
	}
	if adminRead && requested != "" && p.HasScope("admin") {
		return requested, nil
	}
	return "", status.Error(codes.PermissionDenied, "not allowed to access this seller's wallet")
}

func requirePayoutScope(ctx context.Context) error {
	if _, err := auth.RequireScopes(ctx, "listing.write"); err != nil {
		if status.Code(err) == codes.PermissionDenied {
			return status.Error(codes.PermissionDenied, "listing.write scope is required to request a payout")
		}
		return err
	}
	return nil
}

func (h *Handler) GetWalletBalance(ctx context.Context, req *paymentv1.GetWalletBalanceRequest) (*paymentv1.GetWalletBalanceResponse, error) {
	sellerID, err := sellerAccess(ctx, req.GetSellerId(), true)
	if err != nil {
		return nil, err
	}
	balance, err := h.svc.GetWalletBalance(ctx, sellerID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &paymentv1.GetWalletBalanceResponse{Balance: balance}, nil
}

func (h *Handler) ListLedgerEntries(ctx context.Context, req *paymentv1.ListLedgerEntriesRequest) (*paymentv1.ListLedgerEntriesResponse, error) {
	sellerID, err := sellerAccess(ctx, req.GetSellerId(), true)
	if err != nil {
		return nil, err
	}
	entries, next, total, err := h.svc.ListLedgerEntries(ctx, sellerID, req.GetPage().GetCursor(), int32(pagination.Clamp(int(req.GetPage().GetPageSize()), 20, 100)))
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	wire := make([]*paymentv1.WalletEntry, 0, len(entries))
	for _, e := range entries {
		wire = append(wire, toWireLedgerEntry(e))
	}
	return &paymentv1.ListLedgerEntriesResponse{
		Entries: wire,
		Page:    &commonv1.PageResponse{NextCursor: next, Total: total},
	}, nil
}

func (h *Handler) RequestWalletPayout(ctx context.Context, req *paymentv1.RequestWalletPayoutRequest) (*paymentv1.RequestWalletPayoutResponse, error) {
	sellerID, err := sellerAccess(ctx, req.GetSellerId(), false)
	if err != nil {
		return nil, err
	}
	if err := requirePayoutScope(ctx); err != nil {
		return nil, err
	}
	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}
	entry, err := h.svc.RequestWalletPayout(ctx, sellerID, req.GetAmount())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &paymentv1.RequestWalletPayoutResponse{Entry: toWireLedgerEntry(entry)}, nil
}

func toWireLedgerEntry(e store.LedgerEntry) *paymentv1.WalletEntry {
	return &paymentv1.WalletEntry{
		Id:          e.ID,
		SellerId:    e.SellerID,
		Type:        e.Type,
		Amount:      e.Amount,
		Status:      e.Status,
		CreatedAt:   timestamppb.New(e.CreatedAt),
		ReferenceId: e.ReferenceID,
	}
}
