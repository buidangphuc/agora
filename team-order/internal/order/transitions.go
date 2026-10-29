package order

import (
	"errors"

	"github.com/buidangphuc/team-order/internal/store"
)

// ErrActorForbidden: the caller's class may never request this target status.
// -> PERMISSION_DENIED (checked before the order's status, so it reveals nothing).
var ErrActorForbidden = errors.New("caller may not request this order status")

// Actor is the class of caller asking for a status change.
type Actor int

const (
	// ActorNone is a caller who is neither the order's buyer, its seller nor an admin.
	ActorNone Actor = iota
	// ActorBuyer is the order's buyer (cancels through CancelOrder).
	ActorBuyer
	// ActorSeller is the order's seller, or an admin on UpdateOrderStatus.
	ActorSeller
	// ActorSystem is an internal caller: the PaymentSettled consumer.
	ActorSystem
)

func (a Actor) String() string {
	switch a {
	case ActorBuyer:
		return "buyer"
	case ActorSeller:
		return "seller"
	case ActorSystem:
		return "system"
	}
	return "none"
}

type transition struct {
	from, to store.OrderStatus
	actors   []Actor
}

// orderTransitions is THE order status table (spec order-lifecycle-guards).
// Completed and Cancelled are terminal: no row leaves them.
//
//	Pending -> Paid       system (payment settlement only)
//	Pending -> Shipped    seller (cash-on-delivery hand-over)
//	Paid    -> Shipped    seller
//	Shipped -> Completed  seller, buyer (receipt), system
//	Pending, Paid -> Cancelled  buyer (CancelOrder; ForceFailSaga for admin)
//	Pending -> Cancelled        system (payment-window expiry)
var orderTransitions = []transition{
	{store.OrderStatusPending, store.OrderStatusPaid, []Actor{ActorSystem}},
	{store.OrderStatusPending, store.OrderStatusShipped, []Actor{ActorSeller}},
	{store.OrderStatusPaid, store.OrderStatusShipped, []Actor{ActorSeller}},
	{store.OrderStatusShipped, store.OrderStatusCompleted, []Actor{ActorSeller, ActorBuyer, ActorSystem}},
	{store.OrderStatusPending, store.OrderStatusCancelled, []Actor{ActorBuyer}},
	{store.OrderStatusPaid, store.OrderStatusCancelled, []Actor{ActorBuyer}},
	{store.OrderStatusPending, store.OrderStatusCancelled, []Actor{ActorSystem}},
}

// AllowedFrom returns the statuses from which actor may move an order to `to`:
// the allowedFrom set of the compare-and-set write, so the database enforces the
// table. Empty means the actor may never request `to`.
func AllowedFrom(to store.OrderStatus, actor Actor) []store.OrderStatus {
	var out []store.OrderStatus
	for _, t := range orderTransitions {
		if t.to != to {
			continue
		}
		for _, a := range t.actors {
			if a == actor {
				out = append(out, t.from)
				break
			}
		}
	}
	return out
}

// cancelFrom is where a cancel may start (Pending or Paid).
var cancelFrom = AllowedFrom(store.OrderStatusCancelled, ActorBuyer)

// systemCancelFrom is where the system may cancel (Pending only).
var systemCancelFrom = AllowedFrom(store.OrderStatusCancelled, ActorSystem)
