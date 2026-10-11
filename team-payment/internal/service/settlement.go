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
// Paid (design D4): it refunds whatever is still refundable on the order's payment as one
// refund keyed cancel:<order_id> (remainder mode, reason order_cancelled), so a credited
// seller gets exactly one deduction for it whichever of the cancel and the credit is
// applied first. A REFUNDED payment, a remainder of 0 and a redelivered cancel write
// nothing. ErrNotSettled when the order has no settled transaction.
func (s *PaymentService) RefundCancelledOrder(ctx context.Context, orderID string) error {
	if s.settle == nil {
		return ErrSettlementNotConfigured
	}
	tx, err := s.settle.SettledTransaction(ctx, orderID)
	if err != nil {
		return err
	}
	_, err = s.refund(ctx, repository.RefundRequest{
		PaymentID: tx.ID, Key: "cancel:" + orderID, Source: repository.RefundSourceOrderCancel,
		SourceID: orderID, Reason: "order_cancelled", Mode: repository.RefundRemainder,
	})
	return err
}

// RefundReturn applies team-order's ReturnRefunded (design D6/D7): one refund of the
// order's payment keyed return:<return_id>, source RETURN, reason return_refunded, in clamp
// mode. It refunds min(amount, remaining) and records the requested and applied amounts;
// with nothing left it records an applied 0 and no deduction. A redelivered fact is a
// no-op. ErrNotSettled when the order has no settled transaction.
func (s *PaymentService) RefundReturn(ctx context.Context, orderID, returnID string, amount int64) error {
	if s.settle == nil {
		return ErrSettlementNotConfigured
	}
	if orderID == "" || returnID == "" {
		return errors.New("order id and return id are required")
	}
	if amount <= 0 {
		return ErrInvalidAmount
	}
	tx, err := s.settle.SettledTransaction(ctx, orderID)
	if err != nil {
		return err
	}
	_, err = s.refund(ctx, repository.RefundRequest{
		PaymentID: tx.ID, Key: "return:" + returnID, Source: repository.RefundSourceReturn,
		SourceID: returnID, Requested: amount, Reason: "return_refunded", Mode: repository.RefundClamp,
	})
	return err
}
