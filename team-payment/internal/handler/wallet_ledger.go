package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-payment/generated/platform/common/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/interceptor"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
)

// ── Seller Wallet Ledger RPCs ─────────────────────────────────────────

// resolveSellerID returns the effective seller id: an explicit request seller_id,
// otherwise the authenticated principal's own id. Requires authentication.
func (h *PaymentHandler) resolveSellerID(ctx context.Context, requested string) (string, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return "", err
	}
	sellerID := requested
	if sellerID == "" {
		sellerID = principal.GetId()
	}
	if sellerID == "" {
		return "", status.Error(codes.InvalidArgument, "seller_id is required")
	}
	return sellerID, nil
}

func (h *PaymentHandler) GetWalletBalance(ctx context.Context, req *paymentv1.GetWalletBalanceRequest) (*paymentv1.GetWalletBalanceResponse, error) {
	sellerID, err := h.resolveSellerID(ctx, req.GetSellerId())
	if err != nil {
		return nil, err
	}

	balance, err := h.svc.GetWalletBalance(ctx, sellerID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get wallet balance: %v", err)
	}

	return &paymentv1.GetWalletBalanceResponse{Balance: balance}, nil
}

func (h *PaymentHandler) ListLedgerEntries(ctx context.Context, req *paymentv1.ListLedgerEntriesRequest) (*paymentv1.ListLedgerEntriesResponse, error) {
	sellerID, err := h.resolveSellerID(ctx, req.GetSellerId())
	if err != nil {
		return nil, err
	}

	entries, next, total, err := h.svc.ListLedgerEntries(ctx, sellerID, req.GetPage().GetCursor(), req.GetPage().GetPageSize())
	if err != nil {
		if errors.Is(err, service.ErrInvalidPageToken) {
			return nil, status.Error(codes.InvalidArgument, "invalid page cursor")
		}
		return nil, status.Errorf(codes.Internal, "list ledger entries: %v", err)
	}

	wireEntries := make([]*paymentv1.WalletEntry, 0, len(entries))
	for _, e := range entries {
		wireEntries = append(wireEntries, toWireLedgerEntry(e))
	}

	return &paymentv1.ListLedgerEntriesResponse{
		Entries: wireEntries,
		Page: &commonv1.PageResponse{
			NextCursor: next,
			Total:      total,
		},
	}, nil
}

func (h *PaymentHandler) RequestWalletPayout(ctx context.Context, req *paymentv1.RequestWalletPayoutRequest) (*paymentv1.RequestWalletPayoutResponse, error) {
	sellerID, err := h.resolveSellerID(ctx, req.GetSellerId())
	if err != nil {
		return nil, err
	}
	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	entry, err := h.svc.RequestWalletPayout(ctx, sellerID, req.GetAmount())
	if err != nil {
		if errors.Is(err, repository.ErrInvalidAmount) {
			return nil, status.Error(codes.InvalidArgument, "amount must be positive")
		}
		if errors.Is(err, repository.ErrInsufficientBalance) {
			return nil, status.Error(codes.FailedPrecondition, "insufficient wallet balance")
		}
		return nil, status.Errorf(codes.Internal, "request wallet payout: %v", err)
	}

	return &paymentv1.RequestWalletPayoutResponse{Entry: toWireLedgerEntry(entry)}, nil
}

func toWireLedgerEntry(e repository.LedgerEntry) *paymentv1.WalletEntry {
	return &paymentv1.WalletEntry{
		Id:        e.ID,
		SellerId:  e.SellerID,
		Type:      e.Type,
		Amount:    e.Amount,
		Status:    e.Status,
		CreatedAt: timestamppb.New(e.CreatedAt),
	}
}
