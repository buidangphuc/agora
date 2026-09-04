package handler

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

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

func (h *PaymentHandler) GetPayment(ctx context.Context, req *paymentv1.GetPaymentRequest) (*paymentv1.GetPaymentResponse, error) {
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

	return &paymentv1.GetPaymentResponse{
		Transaction: toWireTransaction(tx),
	}, nil
}

func (h *PaymentHandler) ProcessMockPayment(ctx context.Context, req *paymentv1.ProcessMockPaymentRequest) (*paymentv1.ProcessMockPaymentResponse, error) {
	if req.GetTransactionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "transaction_id is required")
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
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}

	sellerID := req.GetSellerId()
	if sellerID == "" {
		sellerID = principal.GetId()
	}
	if sellerID == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
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
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}

	sellerID := req.GetSellerId()
	if sellerID == "" {
		sellerID = principal.GetId()
	}
	if sellerID == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
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
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}

	sellerID := req.GetSellerId()
	if sellerID == "" {
		sellerID = principal.GetId()
	}
	if sellerID == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
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
	_, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}

	if req.GetPaymentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "payment_id is required")
	}
	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	tx, success, msg, err := h.svc.RefundPayment(ctx, req.GetPaymentId(), req.GetAmount(), req.GetReason())
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
