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

	svc          *service.PaymentService
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

func NewPaymentHandler(svc *service.PaymentService, logger *slog.Logger, opts ...Option) *PaymentHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &PaymentHandler{svc: svc, logger: logger}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// internalError logs the cause and returns a fixed INTERNAL status, so storage,
// SQL or upstream error text never reaches the caller.
func (h *PaymentHandler) internalError(ctx context.Context, op string, err error) error {
	reqID, _ := interceptor.RequestIDFromContext(ctx)
	h.logger.ErrorContext(ctx, op+" failed",
		slog.String("request_id", reqID), slog.Any("err", err))
	return status.Error(codes.Internal, "internal error")
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
		return nil, h.internalError(ctx, "create payment", err)
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

// requirePaymentReader gates GetPayment (payment-refund-model D8): the order's buyer or
// an admin directly; otherwise the order's seller as reported by team-order. The extra
// hop happens only for callers who are neither. A failed lookup is never success.
func (h *PaymentHandler) requirePaymentReader(ctx context.Context, principal *commonv1.Principal, tx repository.PaymentTransaction) error {
	if requireBuyerOrAdmin(principal, tx.BuyerID) == nil {
		return nil
	}
	if principal.GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER || principal.GetId() == "" {
		return status.Error(codes.PermissionDenied, "not allowed to access this payment")
	}
	sellerID, err := h.svc.OrderSellerID(ctx, tx.OrderID)
	if err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			return status.Error(codes.NotFound, "order not found")
		}
		return h.internalError(ctx, "resolve order seller", err)
	}
	if sellerID == "" || sellerID != principal.GetId() {
		return status.Error(codes.PermissionDenied, "not allowed to access this payment")
	}
	return nil
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
		return nil, h.internalError(ctx, "get payment", err)
	}
	if err := h.requirePaymentReader(ctx, principal, tx); err != nil {
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
	if !h.mockPayments {
		return nil, status.Error(codes.FailedPrecondition, "mock payments are disabled")
	}
	if req.GetTransactionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "transaction_id is required")
	}

	existing, err := h.svc.GetPayment(ctx, req.GetTransactionId(), "")
	if err != nil {
		if errors.Is(err, repository.ErrTransactionNotFound) {
			return nil, status.Error(codes.NotFound, "transaction not found")
		}
		return nil, h.internalError(ctx, "get payment", err)
	}
	if err := requireBuyerOrAdmin(principal, existing.BuyerID); err != nil {
		return nil, err
	}

	tx, success, msg, err := h.svc.ProcessMockPayment(ctx, req.GetTransactionId(), req.GetSimulateSuccess())
	if err != nil {
		if errors.Is(err, repository.ErrTransactionNotFound) {
			return nil, status.Error(codes.NotFound, "transaction not found")
		}
		return nil, h.internalError(ctx, "process mock payment", err)
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
		return nil, h.internalError(ctx, "get seller wallet", err)
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
		if held := (*repository.FundsOnHoldError)(nil); errors.As(err, &held) {
			// "amount is held until <RFC3339> (refund window)": the instant only, no amounts.
			return nil, status.Error(codes.FailedPrecondition, held.Error())
		}
		return nil, h.internalError(ctx, "request payout", err)
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
		return nil, h.internalError(ctx, "list payout history", err)
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
	if !service.ValidRefundID(req.GetRefundId()) {
		return nil, status.Error(codes.InvalidArgument, service.ErrInvalidRefundID.Error())
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
		return nil, h.internalError(ctx, "refund payment", err)
	}
	if !slices.Contains(principal.GetScopes(), "admin") {
		sellerID, err := h.svc.OrderSellerID(ctx, target.OrderID)
		if err != nil {
			if errors.Is(err, service.ErrOrderNotFound) {
				return nil, status.Error(codes.NotFound, "order not found")
			}
			return nil, h.internalError(ctx, "resolve order seller", err)
		}
		if principal.GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER ||
			sellerID == "" || principal.GetId() != sellerID {
			return nil, status.Error(codes.PermissionDenied, "only the order's seller or an admin can refund a payment")
		}
	}

	tx, success, msg, err := h.svc.RefundPayment(ctx, target.ID, req.GetRefundId(), req.GetAmount(), req.GetReason())
	if err != nil {
		if errors.Is(err, repository.ErrTransactionNotFound) {
			return nil, status.Error(codes.NotFound, "transaction not found")
		}
		if errors.Is(err, service.ErrExceedsRemainder) {
			return nil, status.Error(codes.FailedPrecondition, "refund amount exceeds the refundable remainder")
		}
		if errors.Is(err, service.ErrInvalidRefund) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		if errors.Is(err, service.ErrRefundIDConflict) {
			return nil, status.Error(codes.AlreadyExists, "refund_id was already used for another payment or amount")
		}
		if errors.Is(err, service.ErrInvalidRefundID) || errors.Is(err, service.ErrInvalidAmount) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, h.internalError(ctx, "refund payment", err)
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
		RefundedAmount:    t.RefundedAmount,
		Refunds:           toWireRefunds(t.Refunds),
	}
}

var refundSourceToWire = map[string]paymentv1.PaymentRefundSource{
	repository.RefundSourceSellerOrAdmin: paymentv1.PaymentRefundSource_PAYMENT_REFUND_SOURCE_SELLER_OR_ADMIN,
	repository.RefundSourceReturn:        paymentv1.PaymentRefundSource_PAYMENT_REFUND_SOURCE_RETURN,
	repository.RefundSourceOrderCancel:   paymentv1.PaymentRefundSource_PAYMENT_REFUND_SOURCE_ORDER_CANCEL,
	repository.RefundSourceLegacy:        paymentv1.PaymentRefundSource_PAYMENT_REFUND_SOURCE_LEGACY,
}

// toWireRefunds keeps the store's order (oldest first).
func toWireRefunds(rs []repository.Refund) []*paymentv1.PaymentRefund {
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
