package handler

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-payment/generated/platform/common/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/interceptor"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
)

type PaymentHandler struct {
	paymentv1.UnimplementedPaymentServiceServer

	svc    *service.PaymentService
	logger *slog.Logger
}

func NewPaymentHandler(svc *service.PaymentService, logger *slog.Logger) *PaymentHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &PaymentHandler{svc: svc, logger: logger}
}

func (h *PaymentHandler) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.CreatePaymentResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}

	tx, url, err := h.svc.CreatePayment(ctx, req.GetOrderId(), principal.GetId(), repository.PaymentMethod(req.GetMethod()))
	if err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		if errors.Is(err, service.ErrNotOrderBuyer) {
			return nil, status.Error(codes.PermissionDenied, "only the order's buyer can pay for it")
		}
		if errors.Is(err, service.ErrInvalidOrderState) {
			return nil, status.Error(codes.FailedPrecondition, "order is not in pending state")
		}
		return nil, status.Errorf(codes.Internal, "create payment: %v", err)
	}

	return &paymentv1.CreatePaymentResponse{
		Transaction: toWireTransaction(tx),
		PaymentUrl:  url,
	}, nil
}

// requireBuyerOrAdmin gates the transaction RPCs. A transaction's buyer is the
// order's buyer (CreatePayment records the buyer team-order reports), so only
// that user, or a principal holding the admin scope, may read or settle it.
// Service principals are not buyers.
func requireBuyerOrAdmin(principal *commonv1.Principal, buyerID string) error {
	if slices.Contains(principal.GetScopes(), "admin") {
		return nil
	}
	if principal.GetType() == commonv1.PrincipalType_PRINCIPAL_TYPE_USER &&
		buyerID != "" && principal.GetId() == buyerID {
		return nil
	}
	return status.Error(codes.PermissionDenied, "not allowed to access this payment")
}

func (h *PaymentHandler) GetPayment(ctx context.Context, req *paymentv1.GetPaymentRequest) (*paymentv1.GetPaymentResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" && req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "transaction id or order id required")
	}

	tx, err := h.svc.GetPayment(ctx, req.GetId(), req.GetOrderId())
	if err != nil {
		if errors.Is(err, repository.ErrTransactionNotFound) {
			return nil, status.Error(codes.NotFound, "transaction not found")
		}
		return nil, status.Errorf(codes.Internal, "get payment: %v", err)
	}
	if err := requireBuyerOrAdmin(principal, tx.BuyerID); err != nil {
		return nil, err
	}

	return &paymentv1.GetPaymentResponse{
		Transaction: toWireTransaction(tx),
	}, nil
}

func (h *PaymentHandler) ProcessMockPayment(ctx context.Context, req *paymentv1.ProcessMockPaymentRequest) (*paymentv1.ProcessMockPaymentResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetTransactionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "transaction_id is required")
	}

	existing, err := h.svc.GetPayment(ctx, req.GetTransactionId(), "")
	if err != nil {
		if errors.Is(err, repository.ErrTransactionNotFound) {
			return nil, status.Error(codes.NotFound, "transaction not found")
		}
		return nil, status.Errorf(codes.Internal, "get payment: %v", err)
	}
	if err := requireBuyerOrAdmin(principal, existing.BuyerID); err != nil {
		return nil, err
	}

	tx, success, msg, err := h.svc.ProcessMockPayment(ctx, req.GetTransactionId(), req.GetSimulateSuccess())
	if err != nil {
		if errors.Is(err, repository.ErrTransactionNotFound) {
			return nil, status.Error(codes.NotFound, "transaction not found")
		}
		return nil, status.Errorf(codes.Internal, "process mock payment: %v", err)
	}

	return &paymentv1.ProcessMockPaymentResponse{
		Transaction: toWireTransaction(tx),
		Success:     success,
		Message:     msg,
	}, nil
}

// ── Seller Wallet RPCs ────────────────────────────────────────────────

func (h *PaymentHandler) GetSellerWallet(ctx context.Context, req *paymentv1.GetSellerWalletRequest) (*paymentv1.GetSellerWalletResponse, error) {
	sellerID, err := sellerAccess(ctx, req.GetSellerId(), true)
	if err != nil {
		return nil, err
	}

	wallet, err := h.svc.GetSellerWallet(ctx, sellerID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get seller wallet: %v", err)
	}

	return &paymentv1.GetSellerWalletResponse{
		Wallet: toWireWallet(wallet),
	}, nil
}

func (h *PaymentHandler) RequestPayout(ctx context.Context, req *paymentv1.RequestPayoutRequest) (*paymentv1.RequestPayoutResponse, error) {
	sellerID, err := sellerAccess(ctx, req.GetSellerId(), false)
	if err != nil {
		return nil, err
	}
	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}
	if req.GetBankCode() == "" || req.GetAccountNumber() == "" || req.GetAccountName() == "" {
		return nil, status.Error(codes.InvalidArgument, "bank_code, account_number, and account_name are required")
	}

	payout, err := h.svc.RequestPayout(ctx, sellerID, req.GetAmount(), req.GetBankCode(), req.GetAccountNumber(), req.GetAccountName())
	if err != nil {
		if errors.Is(err, repository.ErrInsufficientBalance) {
			return nil, status.Error(codes.FailedPrecondition, "insufficient wallet balance")
		}
		if errors.Is(err, repository.ErrInvalidAmount) {
			return nil, status.Error(codes.InvalidArgument, "invalid payout amount")
		}
		return nil, status.Errorf(codes.Internal, "request payout: %v", err)
	}

	return &paymentv1.RequestPayoutResponse{
		Payout: toWirePayout(payout),
	}, nil
}

func (h *PaymentHandler) ListPayoutHistory(ctx context.Context, req *paymentv1.ListPayoutHistoryRequest) (*paymentv1.ListPayoutHistoryResponse, error) {
	sellerID, err := sellerAccess(ctx, req.GetSellerId(), true)
	if err != nil {
		return nil, err
	}

	payouts, err := h.svc.ListPayoutHistory(ctx, sellerID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list payout history: %v", err)
	}

	wirePayouts := make([]*paymentv1.PayoutRequest, 0, len(payouts))
	for _, p := range payouts {
		wirePayouts = append(wirePayouts, toWirePayout(p))
	}

	return &paymentv1.ListPayoutHistoryResponse{
		Payouts: wirePayouts,
	}, nil
}

func (h *PaymentHandler) RefundPayment(ctx context.Context, req *paymentv1.RefundPaymentRequest) (*paymentv1.RefundPaymentResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}

	if req.GetPaymentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "payment_id is required")
	}
	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	// Refunds are a seller/admin action (the same rule team-order applies to
	// approving a return): the order's seller, or a principal with the admin scope.
	target, err := h.svc.FindTransaction(ctx, req.GetPaymentId())
	if err != nil {
		if errors.Is(err, repository.ErrTransactionNotFound) {
			return nil, status.Error(codes.NotFound, "transaction not found")
		}
		return nil, status.Errorf(codes.Internal, "refund payment: %v", err)
	}
	if !slices.Contains(principal.GetScopes(), "admin") {
		sellerID, err := h.svc.OrderSellerID(ctx, target.OrderID)
		if err != nil {
			if errors.Is(err, service.ErrOrderNotFound) {
				return nil, status.Error(codes.NotFound, "order not found")
			}
			return nil, status.Errorf(codes.Internal, "resolve order seller: %v", err)
		}
		if principal.GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER ||
			sellerID == "" || principal.GetId() != sellerID {
			return nil, status.Error(codes.PermissionDenied, "only the order's seller or an admin can refund a payment")
		}
	}

	tx, success, msg, err := h.svc.RefundPayment(ctx, target.ID, req.GetAmount(), req.GetReason())
	if err != nil {
		if errors.Is(err, repository.ErrTransactionNotFound) {
			return nil, status.Error(codes.NotFound, "transaction not found")
		}
		if errors.Is(err, service.ErrInvalidRefund) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		if errors.Is(err, service.ErrInvalidAmount) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "refund payment: %v", err)
	}

	return &paymentv1.RefundPaymentResponse{
		Transaction: toWireTransaction(tx),
		Success:     success,
		Message:     msg,
	}, nil
}

// ── Wire Mappings ────────────────────────────────────────────────────

func toWireTransaction(t repository.PaymentTransaction) *paymentv1.PaymentTransaction {
	return &paymentv1.PaymentTransaction{
		Id:                t.ID,
		OrderId:           t.OrderID,
		BuyerId:           t.BuyerID,
		Amount:            t.Amount,
		Currency:          t.Currency,
		Method:            paymentv1.PaymentMethod(t.Method),
		Status:            paymentv1.PaymentStatus(t.Status),
		ProviderReference: t.ProviderReference,
		CreatedAt:         timestamppb.New(t.CreatedAt),
		UpdatedAt:         timestamppb.New(t.UpdatedAt),
	}
}

func toWireWallet(w repository.SellerWallet) *paymentv1.SellerWallet {
	return &paymentv1.SellerWallet{
		Id:        w.ID,
		SellerId:  w.SellerID,
		Balance:   w.Balance,
		Currency:  w.Currency,
		UpdatedAt: timestamppb.New(w.UpdatedAt),
	}
}

func toWirePayout(p repository.PayoutRequest) *paymentv1.PayoutRequest {
	return &paymentv1.PayoutRequest{
		Id:            p.ID,
		SellerId:      p.SellerID,
		Amount:        p.Amount,
		BankCode:      p.BankCode,
		AccountNumber: p.AccountNumber,
		AccountName:   p.AccountName,
		Status:        paymentv1.PayoutStatus(p.Status),
		CreatedAt:     timestamppb.New(p.CreatedAt),
	}
}
