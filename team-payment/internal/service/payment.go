package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/events"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/upstream"
)

var (
	ErrOrderNotFound       = errors.New("order not found for payment")
	ErrInvalidOrderState   = errors.New("order is not in pending state")
	ErrTransactionNotFound = repository.ErrTransactionNotFound
	ErrWalletNotFound      = repository.ErrWalletNotFound
	ErrPayoutNotFound      = repository.ErrPayoutNotFound
	ErrInsufficientBalance = repository.ErrInsufficientBalance
	ErrInvalidAmount       = repository.ErrInvalidAmount
	ErrInvalidRefund       = errors.New("cannot refund unpaid or already refunded transaction")
)

type PaymentService struct {
	paymentRepo repository.PaymentRepository
	walletRepo  repository.WalletRepository
	ledgerRepo  repository.LedgerRepository
	orderClient upstream.OrderClient
	txWriter    repository.PaymentTxWriter
	logger      *slog.Logger
}

// Option configures optional PaymentService collaborators without breaking the
// core constructor signature.
type Option func(*PaymentService)

// WithTxWriter injects the transactional outbox writer used on settle so that
// payment=SETTLED and the PaymentSettled outbox row commit atomically (AD4).
// When unset, settle falls back to a status-only update (no event emitted).
func WithTxWriter(tw repository.PaymentTxWriter) Option {
	return func(s *PaymentService) { s.txWriter = tw }
}

// WithLedgerRepo injects the append-only seller wallet ledger store backing the
// GetWalletBalance / ListLedgerEntries / RequestWalletPayout RPCs. When unset those
// methods return a "not configured" error rather than panicking.
func WithLedgerRepo(lr repository.LedgerRepository) Option {
	return func(s *PaymentService) { s.ledgerRepo = lr }
}

func NewPaymentService(
	paymentRepo repository.PaymentRepository,
	walletRepo repository.WalletRepository,
	orderClient upstream.OrderClient,
	logger *slog.Logger,
	opts ...Option,
) *PaymentService {
	if logger == nil {
		logger = slog.Default()
	}
	s := &PaymentService{
		paymentRepo: paymentRepo,
		walletRepo:  walletRepo,
		orderClient: orderClient,
		logger:      logger,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ── Payment Processing ───────────────────────────────────────────────

func (s *PaymentService) CreatePayment(
	ctx context.Context,
	orderID string,
	buyerID string,
	method repository.PaymentMethod,
) (repository.PaymentTransaction, string, error) {
	if orderID == "" {
		return repository.PaymentTransaction{}, "", errors.New("order id is required")
	}

	// 1. Fetch order details from team-order
	orderResp, err := s.orderClient.GetOrder(ctx, &orderv1.GetOrderRequest{Id: orderID})
	if err != nil {
		return repository.PaymentTransaction{}, "", fmt.Errorf("%w: %v", ErrOrderNotFound, err)
	}
	order := orderResp.GetOrder()
	if order == nil {
		return repository.PaymentTransaction{}, "", ErrOrderNotFound
	}
	if order.GetStatus() != orderv1.OrderStatus_ORDER_STATUS_PENDING {
		return repository.PaymentTransaction{}, "", ErrInvalidOrderState
	}

	// 2. Check if a transaction already exists for this order
	existing, err := s.paymentRepo.GetTransactionByOrderID(ctx, orderID)
	if err == nil {
		paymentURL := fmt.Sprintf("/checkout/pay/%s", orderID)
		return existing, paymentURL, nil
	}

	// 3. Create a new pending transaction
	tx := repository.PaymentTransaction{
		OrderID:  orderID,
		BuyerID:  buyerID,
		Amount:   order.GetTotalAmount(),
		Currency: order.GetCurrency(),
		Method:   method,
		Status:   repository.PaymentStatusPending,
	}

	saved, err := s.paymentRepo.CreateTransaction(ctx, tx)
	if err != nil {
		return repository.PaymentTransaction{}, "", fmt.Errorf("create transaction: %w", err)
	}

	paymentURL := fmt.Sprintf("/checkout/pay/%s", orderID)
	return saved, paymentURL, nil
}

func (s *PaymentService) GetPayment(ctx context.Context, id string, orderID string) (repository.PaymentTransaction, error) {
	if id != "" {
		return s.paymentRepo.GetTransaction(ctx, id)
	}
	if orderID != "" {
		return s.paymentRepo.GetTransactionByOrderID(ctx, orderID)
	}
	return repository.PaymentTransaction{}, errors.New("transaction id or order id required")
}

func (s *PaymentService) ProcessMockPayment(
	ctx context.Context,
	transactionID string,
	simulateSuccess bool,
) (repository.PaymentTransaction, bool, string, error) {
	tx, err := s.paymentRepo.GetTransaction(ctx, transactionID)
	if err != nil {
		return repository.PaymentTransaction{}, false, "", ErrTransactionNotFound
	}

	if tx.Status == repository.PaymentStatusPaid {
		return tx, true, "Đơn hàng đã được thanh toán trước đó", nil
	}

	if simulateSuccess {
		providerRef := fmt.Sprintf("MOCK-REF-%d", time.Now().UnixMilli())

		// AD4 (SA-H3): settle is event-carried, not a synchronous RPC. Write
		// payment=PAID and a PaymentSettled outbox row in ONE transaction; the
		// relayer publishes to "payment.events" and team-order consumes it. The
		// old fire-and-forget order.UpdateOrderStatus call is intentionally gone.
		updated, err := s.settlePaid(ctx, tx, providerRef)
		if err != nil {
			return repository.PaymentTransaction{}, false, "", fmt.Errorf("settle payment: %w", err)
		}
		// Credit the seller's (mock) wallet with the settled amount so payouts
		// have a balance to draw from. Best-effort: never undo a paid order.
		s.creditSellerWallet(ctx, updated)
		return updated, true, "Thanh toán giả lập thành công!", nil
	}

	// Simulate failure
	updated, err := s.paymentRepo.UpdateTransactionStatus(ctx, tx.ID, repository.PaymentStatusFailed, "MOCK-FAIL-REJECTED")
	if err != nil {
		return repository.PaymentTransaction{}, false, "", fmt.Errorf("update status: %w", err)
	}
	return updated, false, "Giao dịch thanh toán bị từ chối.", nil
}

// settlePaid drives a payment to PAID. With a transactional outbox writer wired
// (production / AD4), the status write and the PaymentSettled event commit in
// one transaction. Without one, it degrades to a status-only update so the flow
// still completes (event emission then depends on the outbox writer being
// wired). It never calls order.UpdateOrderStatus — order transition is driven by
// the emitted event.
// creditSellerWallet resolves the order's seller and records a COMPLETED credit
// ledger entry for the settled amount. Best-effort: a failure here is logged and
// must not fail the (already successful) payment.
func (s *PaymentService) creditSellerWallet(ctx context.Context, tx repository.PaymentTransaction) {
	orderResp, err := s.orderClient.GetOrder(ctx, &orderv1.GetOrderRequest{Id: tx.OrderID})
	if err != nil || orderResp.GetOrder() == nil {
		s.logger.WarnContext(ctx, "wallet credit skipped: cannot resolve order seller",
			slog.String("order_id", tx.OrderID), slog.Any("err", err))
		return
	}
	sellerID := orderResp.GetOrder().GetSellerId()
	if sellerID == "" || tx.Amount <= 0 {
		return
	}
	if _, err := s.CreditWallet(ctx, sellerID, tx.Amount, ""); err != nil {
		s.logger.WarnContext(ctx, "wallet credit failed",
			slog.String("seller_id", sellerID), slog.Any("err", err))
	}
}

func (s *PaymentService) settlePaid(ctx context.Context, tx repository.PaymentTransaction, providerRef string) (repository.PaymentTransaction, error) {
	if s.txWriter == nil {
		s.logger.WarnContext(ctx, "settling without transactional outbox writer; no PaymentSettled event emitted",
			slog.String("order_id", tx.OrderID))
		return s.paymentRepo.UpdateTransactionStatus(ctx, tx.ID, repository.PaymentStatusPaid, providerRef)
	}

	eventID := uuid.NewString()
	occurredAt := time.Now().UTC()
	return s.txWriter.SettleTx(ctx, tx.ID, repository.PaymentStatusPaid, providerRef,
		func(settled repository.PaymentTransaction) (repository.OutboxRow, error) {
			payload, err := events.BuildPaymentSettledEnvelope(
				eventID,
				settled.ID,
				settled.OrderID,
				settled.BuyerID,
				paymentv1.PaymentStatus(settled.Status),
				occurredAt,
				"",
			)
			if err != nil {
				return repository.OutboxRow{}, err
			}
			return repository.OutboxRow{
				EventID:       eventID,
				AggregateType: "Payment",
				AggregateID:   settled.OrderID, // Kafka key = order_id
				EventType:     events.PaymentSettledEventType,
				Payload:       payload,
			}, nil
		})
}

// ── Refund Payment ───────────────────────────────────────────────────

func (s *PaymentService) RefundPayment(
	ctx context.Context,
	paymentID string,
	amount int64,
	reason string,
) (repository.PaymentTransaction, bool, string, error) {
	if paymentID == "" {
		return repository.PaymentTransaction{}, false, "", errors.New("payment id is required")
	}
	if amount <= 0 {
		return repository.PaymentTransaction{}, false, "", ErrInvalidAmount
	}

	tx, err := s.paymentRepo.GetTransaction(ctx, paymentID)
	if err != nil {
		tx, err = s.paymentRepo.GetTransactionByOrderID(ctx, paymentID)
		if err != nil {
			return repository.PaymentTransaction{}, false, "", ErrTransactionNotFound
		}
	}

	if tx.Status != repository.PaymentStatusPaid {
		return repository.PaymentTransaction{}, false, "", ErrInvalidRefund
	}

	if amount > tx.Amount {
		return repository.PaymentTransaction{}, false, "", errors.New("refund amount exceeds transaction amount")
	}

	ref := fmt.Sprintf("REFUND:%s", reason)
	updated, err := s.paymentRepo.UpdateTransactionStatus(ctx, tx.ID, repository.PaymentStatusRefunded, ref)
	if err != nil {
		return repository.PaymentTransaction{}, false, "", fmt.Errorf("update refund status: %w", err)
	}

	return updated, true, "Hoàn tiền thành công", nil
}

// ── Seller Wallet & Payout ───────────────────────────────────────────

func (s *PaymentService) GetSellerWallet(ctx context.Context, sellerID string) (repository.SellerWallet, error) {
	if sellerID == "" {
		return repository.SellerWallet{}, errors.New("seller id is required")
	}
	if s.walletRepo == nil {
		return repository.SellerWallet{}, errors.New("wallet repository not configured")
	}
	return s.walletRepo.GetOrCreateWallet(ctx, sellerID)
}

func (s *PaymentService) RequestPayout(
	ctx context.Context,
	sellerID string,
	amount int64,
	bankCode string,
	accountNumber string,
	accountName string,
) (repository.PayoutRequest, error) {
	if sellerID == "" {
		return repository.PayoutRequest{}, errors.New("seller id is required")
	}
	if amount <= 0 {
		return repository.PayoutRequest{}, ErrInvalidAmount
	}
	if bankCode == "" || accountNumber == "" || accountName == "" {
		return repository.PayoutRequest{}, errors.New("bank code, account number, and account name are required")
	}
	if s.walletRepo == nil {
		return repository.PayoutRequest{}, errors.New("wallet repository not configured")
	}

	// 1. Deduct balance from seller wallet
	wallet, err := s.walletRepo.UpdateWalletBalance(ctx, sellerID, -amount)
	if err != nil {
		return repository.PayoutRequest{}, err
	}

	// 2. Create payout request
	payout := repository.PayoutRequest{
		SellerID:      sellerID,
		Amount:        amount,
		BankCode:      bankCode,
		AccountNumber: accountNumber,
		AccountName:   accountName,
		Status:        repository.PayoutStatusPending,
	}

	savedPayout, err := s.walletRepo.CreatePayoutRequest(ctx, payout)
	if err != nil {
		// Rollback wallet balance if payout request fails
		_, _ = s.walletRepo.UpdateWalletBalance(ctx, sellerID, amount)
		return repository.PayoutRequest{}, fmt.Errorf("create payout request: %w", err)
	}

	// 3. Record wallet transaction
	_, _ = s.walletRepo.CreateWalletTransaction(ctx, repository.WalletTransaction{
		WalletID:    wallet.ID,
		Amount:      -amount,
		Type:        repository.WalletTxTypePayout,
		ReferenceID: savedPayout.ID,
	})

	return savedPayout, nil
}

func (s *PaymentService) ListPayoutHistory(ctx context.Context, sellerID string) ([]repository.PayoutRequest, error) {
	if sellerID == "" {
		return nil, errors.New("seller id is required")
	}
	if s.walletRepo == nil {
		return nil, errors.New("wallet repository not configured")
	}
	return s.walletRepo.ListPayoutRequestsBySellerID(ctx, sellerID)
}
