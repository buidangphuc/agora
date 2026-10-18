package service_test

import (
	"context"
	"errors"
	"testing"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

// Scenario "Order expiry: System may cancel only from Pending".
func TestAllowedFromCancelledSystemIsPendingOnly(t *testing.T) {
	sys := service.AllowedFrom(repository.OrderStatusCancelled, service.ActorSystem)
	if len(sys) != 1 || sys[0] != repository.OrderStatusPending {
		t.Fatalf("system may cancel from %v, want exactly [Pending]", sys)
	}
	buyer := service.AllowedFrom(repository.OrderStatusCancelled, service.ActorBuyer)
	got := map[repository.OrderStatus]bool{}
	for _, s := range buyer {
		got[s] = true
	}
	if len(buyer) != 2 || !got[repository.OrderStatusPending] || !got[repository.OrderStatusPaid] {
		t.Fatalf("buyer may cancel from %v, want exactly Pending and Paid", buyer)
	}
	// The system class still pays and nothing else changed for the seller.
	if seller := service.AllowedFrom(repository.OrderStatusCancelled, service.ActorSeller); len(seller) != 0 {
		t.Fatalf("seller must never reach Cancelled, got %v", seller)
	}
}

// Scenario "Order expiry: A seller or admin cannot request Cancelled through UpdateOrderStatus".
func TestUpdateOrderStatusCancelledStillForbidden(t *testing.T) {
	svc, orders, outbox, o := newCancelOutboxRig(t)
	_, err := svc.UpdateOrderStatus(context.Background(), o.ID, service.ActorSeller, repository.OrderStatusCancelled, "")
	if !errors.Is(err, service.ErrActorForbidden) {
		t.Fatalf("want ErrActorForbidden, got %v", err)
	}
	got, gerr := orders.GetOrder(context.Background(), o.ID)
	if gerr != nil || got.Status != repository.OrderStatusPending || got.CancelReason != 0 {
		t.Fatalf("order must be unchanged: %v %+v", gerr, got)
	}
	if n := len(cancelFacts(t, outbox, o.ID)); n != 0 {
		t.Fatalf("a refused change emits nothing, got %d", n)
	}
}

// Scenario "Order expiry: Buyer cancel stores the chosen reason" (service half:
// the claimed row stores the reason and the event carries the same one).
func TestCancelOrderWithReasonStoresItOnRowAndEvent(t *testing.T) {
	for _, reason := range []orderv1.CancelReason{
		orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND,
		orderv1.CancelReason_CANCEL_REASON_OTHER,
		orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED,
		orderv1.CancelReason_CANCEL_REASON_ADMIN_FORCE_FAIL,
	} {
		t.Run(reason.String(), func(t *testing.T) {
			svc, orders, outbox, o := newCancelOutboxRig(t)
			res, err := svc.CancelOrderWithReason(context.Background(), o.ID, reason)
			if err != nil {
				t.Fatal(err)
			}
			if res.CancelReason != int32(reason) {
				t.Fatalf("returned order cancel_reason %d, want %d", res.CancelReason, reason)
			}
			stored, _ := orders.GetOrder(context.Background(), o.ID)
			if stored.Status != repository.OrderStatusCancelled || stored.CancelReason != int32(reason) {
				t.Fatalf("stored order: %+v", stored)
			}
			facts := cancelFacts(t, outbox, o.ID)
			if len(facts) != 1 || facts[0].GetReasonCode() != reason {
				t.Fatalf("OrderCancelled facts: %+v", facts)
			}
		})
	}
}

// Scenario "Order expiry: Cancel without a reason code still succeeds": the
// unchanged CancelOrder entry point stores UNSPECIFIED.
func TestCancelOrderWithoutReasonStoresUnspecified(t *testing.T) {
	svc, orders, outbox, o := newCancelOutboxRig(t)
	if _, err := svc.CancelOrder(context.Background(), o.ID); err != nil {
		t.Fatal(err)
	}
	stored, _ := orders.GetOrder(context.Background(), o.ID)
	if stored.Status != repository.OrderStatusCancelled || stored.CancelReason != 0 {
		t.Fatalf("stored order: %+v", stored)
	}
	facts := cancelFacts(t, outbox, o.ID)
	if len(facts) != 1 || facts[0].GetReasonCode() != orderv1.CancelReason_CANCEL_REASON_UNSPECIFIED {
		t.Fatalf("facts: %+v", facts)
	}
}

// Scenario "Order expiry: Cancel of a shipped order is refused".
func TestCancelShippedOrderKeepsReasonUnset(t *testing.T) {
	svc, orders, outbox, o := newCancelOutboxRig(t)
	ctx := context.Background()
	if _, err := orders.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusShipped,
		[]repository.OrderStatus{repository.OrderStatusPending}, "TRK"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CancelOrderWithReason(ctx, o.ID, orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND)
	if !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("want ErrInvalidStatus, got %v", err)
	}
	stored, _ := orders.GetOrder(ctx, o.ID)
	if stored.Status != repository.OrderStatusShipped || stored.CancelReason != 0 {
		t.Fatalf("shipped order must be untouched: %+v", stored)
	}
	if n := len(cancelFacts(t, outbox, o.ID)); n != 0 {
		t.Fatalf("no OrderCancelled for a refused cancel, got %d", n)
	}
}

// Scenario "Order expiry: Concurrent buyer cancel and expiry keep one reason"
// (service half): the second cancel with a different reason loses and the first
// reason stands.
func TestSecondCancelWithOtherReasonKeepsFirstReason(t *testing.T) {
	svc, orders, outbox, o := newCancelOutboxRig(t)
	ctx := context.Background()
	if _, err := svc.CancelOrderWithReason(ctx, o.ID, orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelOrderWithReason(ctx, o.ID, orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND); !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("second cancel: want ErrInvalidStatus, got %v", err)
	}
	stored, _ := orders.GetOrder(ctx, o.ID)
	if stored.CancelReason != int32(orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED) {
		t.Fatalf("reason overwritten: %d", stored.CancelReason)
	}
	facts := cancelFacts(t, outbox, o.ID)
	if len(facts) != 1 || facts[0].GetReasonCode() != orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED {
		t.Fatalf("facts: %+v", facts)
	}
}
