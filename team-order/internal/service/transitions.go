package service

import (
	"errors"

	"github.com/buidangphuc/team-order/internal/repository"
)

// ErrActorForbidden: the caller's class may never request this target status.
// -> PERMISSION_DENIED (checked before the order's status, so it reveals nothing).
var ErrActorForbidden = errors.New("caller may not request this order status")

// Actor is the class of caller asking for a status change.
type Actor int

const (
	// ActorNone is a caller who is neither the order's buyer, its seller nor an admin.
	ActorNone Actor = iota
	// ActorBuyer is the order's buyer (cancels through CancelOrder / ForceFailSaga).
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
	from, to repository.OrderStatus
	actors   []Actor
}

// orderTransitions is THE order status table (spec order-lifecycle-guards).
// Completed and Cancelled are terminal: no row leaves them.
//
//	Pending -> Paid       system (payment settlement only)
//	Pending -> Shipped    seller (cash-on-delivery hand-over)
//	Paid    -> Shipped    seller
//	Shipped -> Completed  seller
//	Pending, Paid -> Cancelled  buyer (CancelOrder; ForceFailSaga for buyer or admin)
var orderTransitions = []transition{
	{repository.OrderStatusPending, repository.OrderStatusPaid, []Actor{ActorSystem}},
	{repository.OrderStatusPending, repository.OrderStatusShipped, []Actor{ActorSeller}},
	{repository.OrderStatusPaid, repository.OrderStatusShipped, []Actor{ActorSeller}},
	{repository.OrderStatusShipped, repository.OrderStatusCompleted, []Actor{ActorSeller}},
	{repository.OrderStatusPending, repository.OrderStatusCancelled, []Actor{ActorBuyer}},
	{repository.OrderStatusPaid, repository.OrderStatusCancelled, []Actor{ActorBuyer}},
}

// AllowedFrom returns the statuses from which actor may move an order to `to`:
// the allowedFrom set of the compare-and-set write, so the database enforces the
// table. Empty means the actor may never request `to`.
func AllowedFrom(to repository.OrderStatus, actor Actor) []repository.OrderStatus {
	var out []repository.OrderStatus
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
var cancelFrom = AllowedFrom(repository.OrderStatusCancelled, ActorBuyer)
