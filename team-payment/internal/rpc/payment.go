package rpc

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/store"
)

type Handler struct {
	paymentv1.UnimplementedPaymentServiceServer

	svc    *payment.Service
	gate   *auditx.Gate
	logger *slog.Logger
	errs   grpcx.ErrorTable
}

func NewHandler(svc *payment.Service, gate *auditx.Gate, logger *slog.Logger) (*Handler, error) {
	if svc == nil {
		return nil, errors.New("rpc: payment service is required")
	}
	if gate == nil {
		return nil, errors.New("rpc: audit gate is required")
	}
	if logger == nil {
		return nil, errors.New("rpc: logger is required")
	}
	errs, err := grpcx.NewErrorTable(
		grpcx.Rule{Target: payment.ErrOrderNotFound, Code: codes.NotFound, Message: "order not found"},
		grpcx.Rule{Target: payment.ErrTransactionNotFound, Code: codes.NotFound, Message: "transaction not found"},
		grpcx.Rule{Target: payment.ErrNotOrderBuyer, Code: codes.PermissionDenied, Message: "only the order's buyer can pay for it"},
		grpcx.Rule{Target: payment.ErrInvalidOrderState, Code: codes.FailedPrecondition},
		grpcx.Rule{Target: payment.ErrPaymentRefunded, Code: codes.FailedPrecondition},
		grpcx.Rule{Target: payment.ErrPaymentCancelled, Code: codes.FailedPrecondition},
		grpcx.Rule{Target: payment.ErrOrderServiceUnavailable, Code: codes.Unavailable},
		grpcx.Rule{Target: payment.ErrInsufficientBalance, Code: codes.FailedPrecondition, Message: "insufficient wallet balance"},
		grpcx.Rule{Target: payment.ErrInvalidAmount, Code: codes.InvalidArgument},
		grpcx.Rule{Target: payment.ErrExceedsRemainder, Code: codes.FailedPrecondition, Message: "refund amount exceeds the refundable remainder"},
		grpcx.Rule{Target: payment.ErrInvalidRefund, Code: codes.FailedPrecondition},
		grpcx.Rule{Target: payment.ErrRefundIDConflict, Code: codes.AlreadyExists, Message: "refund_id was already used for another payment or amount"},
		grpcx.Rule{Target: payment.ErrInvalidRefundID, Code: codes.InvalidArgument},
		grpcx.Rule{Target: payment.ErrInvalidPageToken, Code: codes.InvalidArgument, Message: "invalid page cursor"},
	)
	if err != nil {
		return nil, err
	}
	return &Handler{svc: svc, gate: gate, logger: logger, errs: errs}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	paymentv1.RegisterPaymentServiceServer(srv, h)
}

func (h *Handler) fail(ctx context.Context, err error) error {
	if held := (*store.FundsOnHoldError)(nil); errors.As(err, &held) {
		return status.Error(codes.FailedPrecondition, held.Error())
	}
	return grpcx.Status(ctx, h.logger, err, h.errs)
}

func (h *Handler) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.CreatePaymentResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	tx, url, err := h.svc.CreatePayment(ctx, req.GetOrderId(), principal.ID, store.PaymentMethod(req.GetMethod()))
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &paymentv1.CreatePaymentResponse{Transaction: toWireTransaction(tx), PaymentUrl: url}, nil
}

func requireBuyerOrAdmin(p auth.Principal, buyerID string) error {
	if p.HasScope("admin") {
		return nil
	}
	if p.Type == auth.TypeUser && buyerID != "" && p.ID == buyerID {
		return nil
	}
	return status.Error(codes.PermissionDenied, "not allowed to access this payment")
}

func (h *Handler) requirePaymentReader(ctx context.Context, p auth.Principal, tx store.PaymentTransaction) error {
	if requireBuyerOrAdmin(p, tx.BuyerID) == nil {
		return nil
	}
	if p.Type != auth.TypeUser || p.ID == "" {
		return status.Error(codes.PermissionDenied, "not allowed to access this payment")
	}
	sellerID, err := h.svc.OrderSellerID(ctx, tx.OrderID)
	if err != nil {
		return h.fail(ctx, err)
	}
	if sellerID == "" || sellerID != p.ID {
		return status.Error(codes.PermissionDenied, "not allowed to access this payment")
	}
	return nil
}

func (h *Handler) GetPayment(ctx context.Context, req *paymentv1.GetPaymentRequest) (*paymentv1.GetPaymentResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" && req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "transaction id or order id required")
	}
	tx, err := h.svc.GetPayment(ctx, req.GetId(), req.GetOrderId())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if err := h.requirePaymentReader(ctx, principal, tx); err != nil {
		return nil, err
	}
	return &paymentv1.GetPaymentResponse{Transaction: toWireTransaction(tx)}, nil
}

func (h *Handler) ProcessMockPayment(ctx context.Context, req *paymentv1.ProcessMockPaymentRequest) (*paymentv1.ProcessMockPaymentResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if !h.svc.MockSettlementEnabled() {
		return nil, status.Error(codes.FailedPrecondition, "mock payments are disabled")
	}
	if req.GetTransactionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "transaction_id is required")
	}
	existing, err := h.svc.GetPayment(ctx, req.GetTransactionId(), "")
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if err := requireBuyerOrAdmin(principal, existing.BuyerID); err != nil {
		return nil, err
	}
	tx, success, msg, err := h.svc.ProcessMockPayment(ctx, req.GetTransactionId(), req.GetSimulateSuccess())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &paymentv1.ProcessMockPaymentResponse{Transaction: toWireTransaction(tx), Success: success, Message: msg}, nil
}

func (h *Handler) RefundPayment(ctx context.Context, req *paymentv1.RefundPaymentRequest) (*paymentv1.RefundPaymentResponse, error) {
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

	target, err := h.svc.FindTransaction(ctx, req.GetPaymentId())
	if err != nil {
		return nil, h.fail(ctx, err)
	}

	var (
		tx      store.PaymentTransaction
		success bool
		msg     string
	)
	run := func(ctx context.Context) error {
		var err error
		tx, success, msg, err = h.svc.RefundPayment(ctx, target.ID, req.GetRefundId(), req.GetAmount(), req.GetReason())
		return err
	}
	if principal.HasScope("admin") {
		ev := auditx.Event{
			ActorID:    principal.ID,
			Action:     "payment.admin_refund",
			TargetType: "payment",
			TargetID:   target.ID,
			Metadata: map[string]string{
				"order_id":  target.OrderID,
				"refund_id": req.GetRefundId(),
				"amount":    strconv.FormatInt(req.GetAmount(), 10),
			},
		}
		err = h.gate.Do(ctx, ev, run)
	} else {
		sellerID, serr := h.svc.OrderSellerID(ctx, target.OrderID)
		if serr != nil {
			return nil, h.fail(ctx, serr)
		}
		if principal.Type != auth.TypeUser || sellerID == "" || principal.ID != sellerID {
			return nil, status.Error(codes.PermissionDenied, "only the order's seller or an admin can refund a payment")
		}
		err = run(ctx)
	}
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &paymentv1.RefundPaymentResponse{Transaction: toWireTransaction(tx), Success: success, Message: msg}, nil
}

func (h *Handler) GetSellerWallet(ctx context.Context, req *paymentv1.GetSellerWalletRequest) (*paymentv1.GetSellerWalletResponse, error) {
	sellerID, err := sellerAccess(ctx, req.GetSellerId(), true)
	if err != nil {
		return nil, err
	}
	wallet, err := h.svc.GetSellerWallet(ctx, sellerID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &paymentv1.GetSellerWalletResponse{Wallet: toWireWallet(wallet)}, nil
}

func (h *Handler) RequestPayout(ctx context.Context, req *paymentv1.RequestPayoutRequest) (*paymentv1.RequestPayoutResponse, error) {
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
		return nil, h.fail(ctx, err)
	}
	return &paymentv1.RequestPayoutResponse{Payout: toWirePayout(payout)}, nil
}

func (h *Handler) ListPayoutHistory(ctx context.Context, req *paymentv1.ListPayoutHistoryRequest) (*paymentv1.ListPayoutHistoryResponse, error) {
	sellerID, err := sellerAccess(ctx, req.GetSellerId(), true)
	if err != nil {
		return nil, err
	}
	payouts, err := h.svc.ListPayoutHistory(ctx, sellerID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	wire := make([]*paymentv1.PayoutRequest, 0, len(payouts))
	for _, p := range payouts {
		wire = append(wire, toWirePayout(p))
	}
	return &paymentv1.ListPayoutHistoryResponse{Payouts: wire}, nil
}

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
