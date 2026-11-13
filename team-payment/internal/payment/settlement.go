package payment

import (
	"context"
	"errors"

	"github.com/buidangphuc/team-payment/internal/store"
)

// CreditSettlement credits sellerID for the order's payment, driven by team-order's
// OrderPaidEvent (design D1/D2): one ORDER_SETTLEMENT of the payment transaction's
// amount, referencing it, idempotent across redelivery. The transaction amount wins over
// the event's total (a mismatch is logged). ErrNotSettled when the order has no
// PAID/REFUNDED transaction; store.ErrInvalidAmount for a non-positive amount.
func (s *Service) CreditSettlement(ctx context.Context, orderID, sellerID string, eventTotal int64) error {
	return s.credit(ctx, orderID, sellerID, eventTotal, store.Commissionable{})
}

// Breakdown is the price breakdown of an OrderPaidEvent, in integer VND.
type Breakdown struct {
	ItemsSubtotal, ShippingFee, ShopDiscount, PlatformDiscount int64
}

// CreditSettlementWithBreakdown is CreditSettlement for an event that carries the price
// breakdown: the seller is also credited the platform voucher share and the commission is
// booked on items subtotal minus shop discount at the configured rate.
func (s *Service) CreditSettlementWithBreakdown(ctx context.Context, orderID, sellerID string, eventTotal int64, b Breakdown) error {
	c := store.Commissionable{
		OK:               true,
		PlatformDiscount: b.PlatformDiscount,
		Base:             max(b.ItemsSubtotal-b.ShopDiscount, 0),
		RateBps:          int(s.commission),
	}
	return s.credit(ctx, orderID, sellerID, eventTotal, c)
}

func (s *Service) credit(ctx context.Context, orderID, sellerID string, eventTotal int64, c store.Commissionable) error {
	if s.settle == nil {
		return ErrSettlementNotConfigured
	}
	res, err := s.settle.CreditSettlement(ctx, orderID, sellerID, c)
	if err != nil {
		return err
	}
	if eventTotal != res.Transaction.Amount {
		s.logger.WarnContext(ctx, "settlement amount differs from the order total; crediting the payment amount",
			"order_id", orderID, "payment_amount", res.Transaction.Amount, "order_total", eventTotal)
	}
	s.logger.InfoContext(ctx, "settlement credit applied",
		"order_id", orderID, "payment_id", res.Transaction.ID,
		"credited", res.Credited, "deducted", res.Deducted)
	return nil
}

// RefundCancelledOrder applies team-order's OrderCancelled for an order cancelled from
// Paid (design D4): it refunds whatever is still refundable on the order's payment as one
// refund keyed cancel:<order_id> (remainder mode, reason order_cancelled), so a credited
// seller gets exactly one deduction for it whichever of the cancel and the credit is
// applied first. A REFUNDED payment, a remainder of 0 and a redelivered cancel write
// nothing. ErrNotSettled when the order has no settled transaction.
func (s *Service) RefundCancelledOrder(ctx context.Context, orderID string) error {
	if s.settle == nil {
		return ErrSettlementNotConfigured
	}
	tx, err := s.settle.SettledTransaction(ctx, orderID)
	if err != nil {
		return err
	}
	_, err = s.refund(ctx, store.RefundRequest{
		PaymentID: tx.ID, Key: "cancel:" + orderID, Source: store.RefundSourceOrderCancel,
		SourceID: orderID, Reason: "order_cancelled", Mode: store.RefundRemainder,
	})
	return err
}

// RefundReturn applies team-order's ReturnRefunded (design D6/D7): one refund of the
// order's payment keyed return:<return_id>, source RETURN, reason return_refunded, in clamp
// mode. It refunds min(amount, remaining) and records the requested and applied amounts;
// with nothing left it records an applied 0 and no deduction. A redelivered fact is a
// no-op. ErrNotSettled when the order has no settled transaction.
func (s *Service) RefundReturn(ctx context.Context, orderID, returnID string, amount int64) error {
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
	_, err = s.refund(ctx, store.RefundRequest{
		PaymentID: tx.ID, Key: "return:" + returnID, Source: store.RefundSourceReturn,
		SourceID: returnID, Requested: amount, Reason: "return_refunded", Mode: store.RefundClamp,
	})
	return err
}

// unpaidCanceller is the compare-and-set void both repository implementations provide.
type unpaidCanceller interface {
	CancelUnpaid(ctx context.Context, orderID string) (store.CancelUnpaidOutcome, error)
}

// VoidCancelledOrder applies team-order's OrderCancelled for an order cancelled unpaid:
// a PENDING/FAILED transaction is voided (compare-and-set to CANCELLED, no money moves).
// Absent and already-CANCELLED are no-ops. When the compare-and-set finds the payment
// settled meanwhile, it falls back to the refund keyed cancel:<order_id>.
func (s *Service) VoidCancelledOrder(ctx context.Context, orderID string) error {
	canceller, ok := s.paymentRepo.(unpaidCanceller)
	if !ok {
		return errors.New("payment repository cannot void unpaid transactions")
	}
	outcome, err := canceller.CancelUnpaid(ctx, orderID)
	if err != nil {
		return err
	}
	if outcome != store.CancelOtherStatus {
		return nil
	}
	return s.RefundCancelledOrder(ctx, orderID)
}

// GetOrderSettlement reads the settlement breakdown of an order without writing.
// ErrNotSettled when the order has no settled payment or no credit yet.
func (s *Service) GetOrderSettlement(ctx context.Context, orderID string) (store.OrderSettlementView, error) {
	if s.settle == nil {
		return store.OrderSettlementView{}, ErrSettlementNotConfigured
	}
	return s.settle.OrderSettlement(ctx, orderID)
}
