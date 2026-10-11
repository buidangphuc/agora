package service

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	ErrOrderNotFound = errors.New("order not found for payment")
	// ErrOrderServiceUnavailable: team-order could not be reached to resolve the order.
	ErrOrderServiceUnavailable = errors.New("order service unavailable")
	ErrInvalidOrderState       = errors.New("order is not in pending state")
	ErrNotOrderBuyer           = errors.New("caller is not the buyer of this order")
	ErrTransactionNotFound     = repository.ErrTransactionNotFound
	ErrPayoutNotFound          = repository.ErrPayoutNotFound
	ErrInsufficientBalance     = repository.ErrInsufficientBalance
	ErrInvalidAmount           = repository.ErrInvalidAmount
	ErrInvalidRefund           = errors.New("cannot refund unpaid or already refunded transaction")
	// ErrPaymentRefunded: a (partially) refunded payment can never be paid again;
	// re-settling it would reopen it to further refunds of money already returned.
	ErrPaymentRefunded = errors.New("payment has been refunded; it cannot be paid again")
	// ErrExceedsRemainder: a refund above the payment's refundable remainder.
	ErrExceedsRemainder = repository.ErrExceedsRemainder
	// ErrRefundIDConflict: the refund id was already used for another payment or amount.
	ErrRefundIDConflict = repository.ErrRefundIDConflict
	// ErrInvalidRefundID: the refund id is missing or not 1-64 of [A-Za-z0-9._:-].
	ErrInvalidRefundID = errors.New("refund_id is required: 1-64 characters of [A-Za-z0-9._:-]")
	// ErrNotSettled: the order has no PAID/REFUNDED payment to credit or refund.
	ErrNotSettled = repository.ErrNotSettled
	// ErrFundsOnHold: the payout is within the balance but the proceeds are still in
	// the refund hold window (errors.As gives *repository.FundsOnHoldError).
	ErrFundsOnHold = repository.ErrFundsOnHold
	// ErrSettlementNotConfigured: no SettlementLedger was wired.
	ErrSettlementNotConfigured = errors.New("settlement ledger not configured")
)

type PaymentService struct {
	paymentRepo repository.PaymentRepository
	walletRepo  repository.WalletRepository
	ledgerRepo  repository.LedgerRepository
	orderClient upstream.OrderClient
	txWriter    repository.PaymentTxWriter
	settle      repository.SettlementLedger
	logger      *slog.Logger
	// holdWindow is the payout hold-back window (0 = off); now is the clock it is
	// measured against (injectable for tests).
	holdWindow time.Duration
	now        func() time.Time
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

// WithSettlementLedger injects the store that moves seller money for a payment: the
// settlement credit (consumer of OrderPaidEvent) and the refund compare-and-set with
// its deduction (design D4). RefundPayment requires it.
func WithSettlementLedger(sl repository.SettlementLedger) Option {
	return func(s *PaymentService) { s.settle = sl }
}

// WithPayoutHold sets the payout hold-back window (PAYOUT_HOLD_DAYS / PAYOUT_HOLD_WINDOW):
// settlement credits younger than it cannot be paid out. 0 disables the hold.
func WithPayoutHold(window time.Duration) Option {
	return func(s *PaymentService) { s.holdWindow = window }
}

// WithClock injects the clock the hold window is measured against (tests).
func WithClock(now func() time.Time) Option {
	return func(s *PaymentService) { s.now = now }
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
		now:         time.Now,
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
	callerID string,
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
	// Only the order's buyer may open a payment for it. The transaction's buyer is
	// the order's buyer as reported by team-order, never the request's claim.
	buyerID := order.GetBuyerId()
	if buyerID == "" || buyerID != callerID {
		return repository.PaymentTransaction{}, "", ErrNotOrderBuyer
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

// GetPayment returns the transaction by id or by order id, with its refunds.
func (s *PaymentService) GetPayment(ctx context.Context, id string, orderID string) (repository.PaymentTransaction, error) {
	var (
		tx  repository.PaymentTransaction
		err error
	)
	switch {
	case id != "":
		tx, err = s.paymentRepo.GetTransaction(ctx, id)
	case orderID != "":
		tx, err = s.paymentRepo.GetTransactionByOrderID(ctx, orderID)
	default:
		return repository.PaymentTransaction{}, errors.New("transaction id or order id required")
	}
	if err != nil {
		return repository.PaymentTransaction{}, err
	}
	return s.withRefunds(ctx, tx)
}

// withRefunds attaches the payment's refunds, oldest first (none without a settlement
// ledger).
func (s *PaymentService) withRefunds(ctx context.Context, tx repository.PaymentTransaction) (repository.PaymentTransaction, error) {
	if s.settle == nil || tx.ID == "" {
		return tx, nil
	}
	refunds, err := s.settle.ListRefunds(ctx, tx.ID)
	if err != nil {
		return repository.PaymentTransaction{}, fmt.Errorf("list refunds: %w", err)
	}
	tx.Refunds = refunds
	return tx, nil
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
	// Only PENDING or FAILED may be (re)settled. This early check gives the clear
	// error; the writes below are a compare-and-set from PENDING/FAILED, so a racing
	// call that settled (and maybe refunded) meanwhile is caught there too.
	if tx.Status == repository.PaymentStatusRefunded || tx.Status == repository.PaymentStatusPartiallyRefunded {
		return repository.PaymentTransaction{}, false, "", ErrPaymentRefunded
	}

	if simulateSuccess {
		providerRef := fmt.Sprintf("MOCK-REF-%d", time.Now().UnixMilli())

		// AD4 (SA-H3): settle is event-carried, not a synchronous RPC. Write
		// payment=PAID and a PaymentSettled outbox row in ONE transaction; the
		// relayer publishes to "payment.events" and team-order consumes it. The
		// old fire-and-forget order.UpdateOrderStatus call is intentionally gone.
		updated, err := s.settlePaid(ctx, tx, providerRef)
		if errors.Is(err, repository.ErrNotSettleable) {
			return s.lostSettleRace(ctx, tx.ID)
		}
		if err != nil {
			return repository.PaymentTransaction{}, false, "", fmt.Errorf("settle payment: %w", err)
		}
		// No ledger write here: the seller is credited from team-order's OrderPaidEvent
		// (internal/consumer), so a payment that loses to a cancel never credits.
		return updated, true, "Thanh toán giả lập thành công!", nil
	}

	// Simulate failure
	updated, err := s.paymentRepo.UpdateTransactionStatus(ctx, tx.ID, repository.PaymentStatusFailed, "MOCK-FAIL-REJECTED")
	if errors.Is(err, repository.ErrNotSettleable) {
		return s.lostSettleRace(ctx, tx.ID)
	}
	if err != nil {
		return repository.PaymentTransaction{}, false, "", fmt.Errorf("update status: %w", err)
	}
	return updated, false, "Giao dịch thanh toán bị từ chối.", nil
}

// lostSettleRace answers a mock payment whose compare-and-set found the payment no
// longer PENDING/FAILED: a concurrent call paid it (report it as already paid) or
// it was refunded meanwhile (refuse, never reopen it).
func (s *PaymentService) lostSettleRace(ctx context.Context, txID string) (repository.PaymentTransaction, bool, string, error) {
	cur, err := s.paymentRepo.GetTransaction(ctx, txID)
	if err != nil {
		return repository.PaymentTransaction{}, false, "", err
	}
	if cur.Status == repository.PaymentStatusPaid {
		return cur, true, "Đơn hàng đã được thanh toán trước đó", nil
	}
	return repository.PaymentTransaction{}, false, "", ErrPaymentRefunded
}

// settlePaid drives a payment to PAID. With a transactional outbox writer wired
// (production / AD4), the status write and the PaymentSettled event commit in
// one transaction. Without one, it degrades to a status-only update so the flow
// still completes (event emission then depends on the outbox writer being
// wired). It never calls order.UpdateOrderStatus — order transition is driven by
// the emitted event.
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

// FindTransaction resolves a payment reference that is either a transaction id or
// an order id (the form the RMA flow uses).
func (s *PaymentService) FindTransaction(ctx context.Context, paymentID string) (repository.PaymentTransaction, error) {
	tx, err := s.paymentRepo.GetTransaction(ctx, paymentID)
	if err != nil {
		tx, err = s.paymentRepo.GetTransactionByOrderID(ctx, paymentID)
		if err != nil {
			return repository.PaymentTransaction{}, ErrTransactionNotFound
		}
	}
	return tx, nil
}

// OrderSellerID returns the seller of an order as reported by team-order.
func (s *PaymentService) OrderSellerID(ctx context.Context, orderID string) (string, error) {
	resp, err := s.orderClient.GetOrder(ctx, &orderv1.GetOrderRequest{Id: orderID})
	if err != nil {
		// an unreachable team-order is not a missing order: say so (503), never 404
		switch status.Code(err) {
		case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
			return "", fmt.Errorf("%w: %v", ErrOrderServiceUnavailable, err)
		}
		return "", fmt.Errorf("%w: %v", ErrOrderNotFound, err)
	}
	if resp.GetOrder() == nil {
		return "", ErrOrderNotFound
	}
	return resp.GetOrder().GetSellerId(), nil
}

// ValidRefundID reports whether id is a caller-chosen refund id: 1-64 characters of
// [A-Za-z0-9._:-] (payment-refund-model D2).
func ValidRefundID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range []byte(id) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '.', c == '_', c == ':', c == '-':
		default:
			return false
		}
	}
	return true
}

// RefundPayment applies a seller or admin refund of amount in strict mode, keyed
// rpc:<refundID> (design D1/D2). A replay of the same id and amount returns the current
// state; ErrRefundIDConflict for the id on another payment or amount; ErrInvalidRefund
// when the payment is not PAID / PARTIALLY_REFUNDED; ErrExceedsRemainder above the
// remainder; ErrInvalidRefundID / ErrInvalidAmount for bad input. paymentID may be a
// transaction id or an order id.
func (s *PaymentService) RefundPayment(
	ctx context.Context,
	paymentID string,
	refundID string,
	amount int64,
	reason string,
) (repository.PaymentTransaction, bool, string, error) {
	if paymentID == "" {
		return repository.PaymentTransaction{}, false, "", errors.New("payment id is required")
	}
	if !ValidRefundID(refundID) {
		return repository.PaymentTransaction{}, false, "", ErrInvalidRefundID
	}
	if amount <= 0 {
		return repository.PaymentTransaction{}, false, "", ErrInvalidAmount
	}

	tx, err := s.FindTransaction(ctx, paymentID)
	if err != nil {
		return repository.PaymentTransaction{}, false, "", err
	}

	updated, err := s.refund(ctx, repository.RefundRequest{
		PaymentID: tx.ID, Key: "rpc:" + refundID, Source: repository.RefundSourceSellerOrAdmin,
		SourceID: refundID, Requested: amount, Reason: reason, Mode: repository.RefundStrict,
	})
	if err != nil {
		return repository.PaymentTransaction{}, false, "", err
	}
	return updated, true, "Hoàn tiền thành công", nil
}

// refund runs one ApplyRefund (design D1), maps its refusals and returns the payment
// with its refunds. Never blocked by the seller's balance or hold.
func (s *PaymentService) refund(ctx context.Context, req repository.RefundRequest) (repository.PaymentTransaction, error) {
	if s.settle == nil {
		return repository.PaymentTransaction{}, ErrSettlementNotConfigured
	}
	res, err := s.settle.ApplyRefund(ctx, req)
	switch {
	case errors.Is(err, repository.ErrNotRefundable):
		return repository.PaymentTransaction{}, ErrInvalidRefund
	case errors.Is(err, repository.ErrExceedsRemainder), errors.Is(err, repository.ErrRefundIDConflict),
		errors.Is(err, repository.ErrTransactionNotFound), errors.Is(err, repository.ErrInvalidAmount):
		return repository.PaymentTransaction{}, err
	case err != nil:
		return repository.PaymentTransaction{}, fmt.Errorf("refund payment: %w", err)
	}
	s.logger.InfoContext(ctx, "payment refund applied",
		slog.String("payment_id", req.PaymentID), slog.String("order_id", res.Transaction.OrderID),
		slog.String("refund_id", req.Key), slog.Int64("requested", res.Refund.RequestedAmount),
		slog.Int64("applied", res.Refund.Amount), slog.Bool("created", res.Created),
		slog.Bool("seller_deducted", res.Deducted))
	return s.withRefunds(ctx, res.Transaction)
}

// ── Seller Wallet & Payout ───────────────────────────────────────────

// walletCurrency is the only currency the wallet ledger is kept in.
const walletCurrency = "VND"

// GetSellerWallet returns the seller's wallet view. The balance is the wallet
// ledger's SUM(amount): the ledger is the single source of truth for seller money.
func (s *PaymentService) GetSellerWallet(ctx context.Context, sellerID string) (repository.SellerWallet, error) {
	if sellerID == "" {
		return repository.SellerWallet{}, errors.New("seller id is required")
	}
	if s.ledgerRepo == nil {
		return repository.SellerWallet{}, ErrLedgerNotConfigured
	}
	balance, err := s.ledgerRepo.Balance(ctx, sellerID)
	if err != nil {
		return repository.SellerWallet{}, err
	}
	return repository.SellerWallet{
		ID:        sellerID,
		SellerID:  sellerID,
		Balance:   balance,
		Currency:  walletCurrency,
		UpdatedAt: time.Now(),
	}, nil
}

// RequestPayout debits the wallet ledger through the same atomic AppendDebit as
// RequestWalletPayout (balance check + debit under the per-seller lock), then records
// the bank details in payout_requests linked to the ledger entry. A payout can never
// exceed the ledger balance (ErrInsufficientBalance).
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

	// 1. Atomic balance check + debit on the ledger.
	entry, err := s.RequestWalletPayout(ctx, sellerID, amount)
	if err != nil {
		return repository.PayoutRequest{}, err
	}

	// 2. Record the bank details, linked to the ledger debit.
	savedPayout, err := s.walletRepo.CreatePayoutRequest(ctx, repository.PayoutRequest{
		SellerID:      sellerID,
		Amount:        amount,
		BankCode:      bankCode,
		AccountNumber: accountNumber,
		AccountName:   accountName,
		Status:        repository.PayoutStatusPending,
		LedgerEntryID: entry.ID,
	})
	if err != nil {
		// The ledger is append-only: undo the debit with a compensating credit.
		if _, rbErr := s.ledgerRepo.AppendEntry(ctx, repository.LedgerEntry{
			SellerID: sellerID,
			Type:     repository.LedgerTypePayout,
			Amount:   amount,
			Status:   repository.LedgerStatusRejected,
		}); rbErr != nil {
			s.logger.Error("payout compensation failed", "seller_id", sellerID, "ledger_entry_id", entry.ID, "err", rbErr)
		}
		return repository.PayoutRequest{}, fmt.Errorf("create payout request: %w", err)
	}

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
