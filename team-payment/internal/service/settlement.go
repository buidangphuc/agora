package service

import (
	"context"
	"log/slog"
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
