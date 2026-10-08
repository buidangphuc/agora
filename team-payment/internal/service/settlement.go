package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/buidangphuc/team-payment/internal/repository"
)

// CreditSettlement credits sellerID for the order's payment, driven by team-order's
// OrderPaidEvent (design D1/D2): one ORDER_SETTLEMENT of the payment transaction's
// amount, referencing it, idempotent across redelivery. The transaction amount wins over
// the event's total (a mismatch is logged). ErrNotSettled when the order has no
// PAID/REFUNDED transaction; repository.ErrInvalidAmount for a non-positive amount.
func (s *PaymentService) CreditSettlement(ctx context.Context, orderID, sellerID string, eventTotal int64) error {
	if s.settle == nil {
		return ErrSettlementNotConfigured
	}
	res, err := s.settle.CreditSettlement(ctx, orderID, sellerID)
	if err != nil {
		return err
	}
	if eventTotal != res.Transaction.Amount {
		s.logger.WarnContext(ctx, "settlement amount differs from the order total; crediting the payment amount",
			slog.String("order_id", orderID), slog.Int64("payment_amount", res.Transaction.Amount), slog.Int64("order_total", eventTotal))
	}
	s.logger.InfoContext(ctx, "settlement credit applied",
		slog.String("order_id", orderID), slog.String("payment_id", res.Transaction.ID), slog.String("seller_id", sellerID),
		slog.Bool("credited", res.Credited), slog.Bool("deducted", res.Deducted))
	return nil
}

// RefundCancelledOrder applies team-order's OrderCancelled for an order cancelled from
// Paid (design D12): the order's PAID payment is refunded in full as the system (reason
// order_cancelled) through the same refund transaction as RefundPayment, so a credited
// seller gets exactly one deduction whichever of the cancel and the credit is applied
// first. An already REFUNDED payment (seller/admin refund, or a redelivered cancel) is
// left alone. ErrNotSettled when the order has no PAID/REFUNDED transaction.
func (s *PaymentService) RefundCancelledOrder(ctx context.Context, orderID string) error {
	if s.settle == nil {
		return ErrSettlementNotConfigured
	}
	tx, err := s.settle.SettledTransaction(ctx, orderID)
	if err != nil {
		return err
	}
	if tx.Status == repository.PaymentStatusRefunded {
		s.logger.InfoContext(ctx, "cancelled order already refunded; nothing to do",
			slog.String("order_id", orderID), slog.String("payment_id", tx.ID))
		return nil
	}
	if _, err := s.refund(ctx, tx.ID, tx.Amount, "order_cancelled"); err != nil {
		if errors.Is(err, ErrInvalidRefund) {
			// Lost the compare-and-set to a concurrent refund: already refunded.
			return nil
		}
		return err
	}
	return nil
}
