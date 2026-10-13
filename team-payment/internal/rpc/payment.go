// Package rpc is the gRPC transport adapter of PaymentService: it authorizes each call, maps wire and domain types and turns errors into statuses.
package rpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/store"
)

// PaymentHandler is the PaymentService gRPC adapter.
type PaymentHandler struct {
	paymentv1.UnimplementedPaymentServiceServer

	svc          *payment.Service
	logger       *slog.Logger
	mockPayments bool
}

// Option configures optional PaymentHandler behaviour.
type Option func(*PaymentHandler)

// WithMockPayments enables the ProcessMockPayment RPC (MOCK_PAYMENTS). Off by default:
// the RPC then answers FAILED_PRECONDITION without reading the transaction.
func WithMockPayments(enabled bool) Option {
	return func(h *PaymentHandler) { h.mockPayments = enabled }
}

// NewPaymentHandler builds the payment handler.
func NewPaymentHandler(svc *payment.Service, logger *slog.Logger, opts ...Option) *PaymentHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &PaymentHandler{svc: svc, logger: logger}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// CreatePayment is the create payment operation of PaymentHandler.
func (h *PaymentHandler) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.CreatePaymentResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}

	tx, url, err := h.svc.CreatePayment(ctx, req.GetOrderId(), principal.ID, store.PaymentMethod(req.GetMethod()))
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, createErrs)
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
func requireBuyerOrAdmin(principal auth.Principal, buyerID string) error {
	if principal.HasScope("admin") {
		return nil
	}
	if principal.Type == auth.TypeUser &&
		buyerID != "" && principal.ID == buyerID {
		return nil
	}
	return status.Error(codes.PermissionDenied, "not allowed to access this payment")
}

// requirePaymentReader gates GetPayment (payment-refund-model D8): the order's buyer or
// an admin directly; otherwise the order's seller as reported by team-order. The extra
// hop happens only for callers who are neither. A failed lookup is never success.
func (h *PaymentHandler) requirePaymentReader(ctx context.Context, principal auth.Principal, tx store.PaymentTransaction) error {
	if requireBuyerOrAdmin(principal, tx.BuyerID) == nil {
		return nil
	}
	if principal.Type != auth.TypeUser || principal.ID == "" {
		return status.Error(codes.PermissionDenied, "not allowed to access this payment")
	}
	sellerID, err := h.svc.OrderSellerID(ctx, tx.OrderID)
	if err != nil {
		return grpcx.Status(ctx, h.logger, err, sellerLookupErrs)
	}
	if sellerID == "" || sellerID != principal.ID {
		return status.Error(codes.PermissionDenied, "not allowed to access this payment")
	}
	return nil
}

// GetPayment is the get payment operation of PaymentHandler.
func (h *PaymentHandler) GetPayment(ctx context.Context, req *paymentv1.GetPaymentRequest) (*paymentv1.GetPaymentResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" && req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "transaction id or order id required")
	}

	tx, err := h.svc.GetPayment(ctx, req.GetId(), req.GetOrderId())
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, getErrs)
	}
	if err := h.requirePaymentReader(ctx, principal, tx); err != nil {
		return nil, err
	}

	return &paymentv1.GetPaymentResponse{
		Transaction: toWireTransaction(tx),
	}, nil
}

// ProcessMockPayment is the process mock payment operation of PaymentHandler.
func (h *PaymentHandler) ProcessMockPayment(ctx context.Context, req *paymentv1.ProcessMockPaymentRequest) (*paymentv1.ProcessMockPaymentResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if !h.mockPayments {
		return nil, status.Error(codes.FailedPrecondition, "mock payments are disabled")
	}
	if req.GetTransactionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "transaction_id is required")
	}

	existing, err := h.svc.GetPayment(ctx, req.GetTransactionId(), "")
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, getErrs)
	}
	if err := requireBuyerOrAdmin(principal, existing.BuyerID); err != nil {
		return nil, err
	}

	tx, success, msg, err := h.svc.ProcessMockPayment(ctx, req.GetTransactionId(), req.GetSimulateSuccess())
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, mockErrs)
	}

	return &paymentv1.ProcessMockPaymentResponse{
		Transaction: toWireTransaction(tx),
		Success:     success,
		Message:     msg,
	}, nil
}

// ── Seller Wallet RPCs ────────────────────────────────────────────────

// GetSellerWallet is the get seller wallet operation of PaymentHandler.
func (h *PaymentHandler) GetSellerWallet(ctx context.Context, req *paymentv1.GetSellerWalletRequest) (*paymentv1.GetSellerWalletResponse, error) {
	sellerID, err := sellerAccess(ctx, req.GetSellerId(), true)
	if err != nil {
		return nil, err
	}

	wallet, err := h.svc.GetSellerWallet(ctx, sellerID)
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, grpcx.ErrorTable{})
	}

	return &paymentv1.GetSellerWalletResponse{
		Wallet: toWireWallet(wallet),
	}, nil
}

// RequestPayout is the request payout operation of PaymentHandler.
func (h *PaymentHandler) RequestPayout(ctx context.Context, req *paymentv1.RequestPayoutRequest) (*paymentv1.RequestPayoutResponse, error) {
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
	if req.GetBankCode() == "" || req.GetAccountNumber() == "" || req.GetAccountName() == "" {
		return nil, status.Error(codes.InvalidArgument, "bank_code, account_number, and account_name are required")
	}

	payout, err := h.svc.RequestPayout(ctx, sellerID, req.GetAmount(), req.GetBankCode(), req.GetAccountNumber(), req.GetAccountName())
	if err != nil {
		if held := (*store.FundsOnHoldError)(nil); errors.As(err, &held) {
			// "amount is held until <RFC3339> (refund window)": the instant only, no amounts.
			return nil, status.Error(codes.FailedPrecondition, held.Error())
		}
		return nil, grpcx.Status(ctx, h.logger, err, payoutErrs)
	}

	return &paymentv1.RequestPayoutResponse{
		Payout: toWirePayout(payout),
	}, nil
}

// ListPayoutHistory is the list payout history operation of PaymentHandler.
func (h *PaymentHandler) ListPayoutHistory(ctx context.Context, req *paymentv1.ListPayoutHistoryRequest) (*paymentv1.ListPayoutHistoryResponse, error) {
	sellerID, err := sellerAccess(ctx, req.GetSellerId(), true)
	if err != nil {
		return nil, err
	}

	payouts, err := h.svc.ListPayoutHistory(ctx, sellerID)
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, grpcx.ErrorTable{})
	}

	wirePayouts := make([]*paymentv1.PayoutRequest, 0, len(payouts))
	for _, p := range payouts {
		wirePayouts = append(wirePayouts, toWirePayout(p))
	}

	return &paymentv1.ListPayoutHistoryResponse{
		Payouts: wirePayouts,
	}, nil
}

// RefundPayment is the refund payment operation of PaymentHandler.
func (h *PaymentHandler) RefundPayment(ctx context.Context, req *paymentv1.RefundPaymentRequest) (*paymentv1.RefundPaymentResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}

	if req.GetPaymentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "payment_id is required")
	}
	if !payment.ValidRefundID(req.GetRefundId()) {
		return nil, status.Error(codes.InvalidArgument, payment.ErrInvalidRefundID.Error())
	}
	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	// Refunds are a seller/admin action (the same rule team-order applies to
	// approving a return): the order's seller, or a principal with the admin scope.
	target, err := h.svc.FindTransaction(ctx, req.GetPaymentId())
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, refundLookupErrs)
	}
	if !principal.HasScope("admin") {
		sellerID, err := h.svc.OrderSellerID(ctx, target.OrderID)
		if err != nil {
			return nil, grpcx.Status(ctx, h.logger, err, sellerLookupErrs)
		}
		if principal.Type != auth.TypeUser ||
			sellerID == "" || principal.ID != sellerID {
			return nil, status.Error(codes.PermissionDenied, "only the order's seller or an admin can refund a payment")
		}
	}

	tx, success, msg, err := h.svc.RefundPayment(ctx, target.ID, req.GetRefundId(), req.GetAmount(), req.GetReason())
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, refundErrs)
	}

	return &paymentv1.RefundPaymentResponse{
		Transaction: toWireTransaction(tx),
		Success:     success,
		Message:     msg,
	}, nil
}

// ── Wire Mappings ────────────────────────────────────────────────────

func toWireTransaction(t store.PaymentTransaction) *paymentv1.PaymentTransaction {
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
		RefundedAmount:    t.RefundedAmount,
		Refunds:           toWireRefunds(t.Refunds),
	}
}

var refundSourceToWire = map[string]paymentv1.PaymentRefundSource{
	store.RefundSourceSellerOrAdmin: paymentv1.PaymentRefundSource_PAYMENT_REFUND_SOURCE_SELLER_OR_ADMIN,
	store.RefundSourceReturn:        paymentv1.PaymentRefundSource_PAYMENT_REFUND_SOURCE_RETURN,
	store.RefundSourceOrderCancel:   paymentv1.PaymentRefundSource_PAYMENT_REFUND_SOURCE_ORDER_CANCEL,
	store.RefundSourceLegacy:        paymentv1.PaymentRefundSource_PAYMENT_REFUND_SOURCE_LEGACY,
}

// toWireRefunds keeps the store's order (oldest first).
func toWireRefunds(rs []store.Refund) []*paymentv1.PaymentRefund {
	if len(rs) == 0 {
		return nil
	}
	out := make([]*paymentv1.PaymentRefund, 0, len(rs))
	for _, r := range rs {
		out = append(out, &paymentv1.PaymentRefund{
			Id:              r.ID,
			Source:          refundSourceToWire[r.Source],
			SourceId:        r.SourceID,
			RequestedAmount: r.RequestedAmount,
			Amount:          r.Amount,
			Reason:          r.Reason,
			CreatedAt:       timestamppb.New(r.CreatedAt),
		})
	}
	return out
}

func toWireWallet(w store.SellerWallet) *paymentv1.SellerWallet {
	return &paymentv1.SellerWallet{
		Id:        w.ID,
		SellerId:  w.SellerID,
		Balance:   w.Balance,
		Currency:  w.Currency,
		UpdatedAt: timestamppb.New(w.UpdatedAt),
	}
}

func toWirePayout(p store.PayoutRequest) *paymentv1.PayoutRequest {
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
