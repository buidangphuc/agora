package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	engagementv1 "github.com/buidangphuc/team-order/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

const (
	disputeResolvedType = "platform.engagement.v1.DisputeResolved"
	disputeConsumerName = "team-order.dispute"
	disputeRefundAction = "return.dispute_refund"
)

type DisputeReturnStore interface {
	GetReturn(ctx context.Context, id string) (store.OrderReturn, error)
	TransitionReturn(ctx context.Context, id string, from, to store.ReturnStatus) (store.OrderReturn, error)
	ReapproveReturn(ctx context.Context, id string, orderTotal int64) (store.OrderReturn, error)
}

type DisputeOrderStore interface {
	GetOrder(ctx context.Context, id string) (store.Order, error)
}

type DisputeHandler struct {
	orders  DisputeOrderStore
	returns DisputeReturnStore
	gate    *auditx.Gate
	db      DB
	logger  *slog.Logger
}

func NewDisputeHandler(orders DisputeOrderStore, returns DisputeReturnStore, gate *auditx.Gate, db DB, logger *slog.Logger) (*DisputeHandler, error) {
	if gate == nil {
		return nil, errors.New("dispute handler: audit gate is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &DisputeHandler{orders: orders, returns: returns, gate: gate, db: db, logger: logger}, nil
}

func (h *DisputeHandler) Handle(ctx context.Context, rec consume.Record) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		return permanent("unmarshal envelope: %v", err)
	}
	if env.GetType() != disputeResolvedType {
		return nil
	}
	if env.GetEventId() == "" {
		return permanent("envelope missing event_id")
	}
	already, err := dedupe.Processed(ctx, h.db, disputeConsumerName, env.GetEventId())
	if err != nil {
		return err
	}
	if already {
		return nil
	}

	var ev engagementv1.DisputeResolved
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		return permanent("unmarshal DisputeResolved: %v", err)
	}
	if ev.GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED ||
		ev.GetRuling() != engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR ||
		ev.GetReturnId() == "" {
		return nil
	}
	if ev.GetResolverId() == "" {
		return permanent("DisputeResolved missing resolver_id")
	}

	if err := h.apply(ctx, &ev); err != nil {
		return err
	}
	if _, err := dedupe.Seen(ctx, h.db, disputeConsumerName, env.GetEventId()); err != nil {
		return err
	}
	return nil
}

func (h *DisputeHandler) apply(ctx context.Context, ev *engagementv1.DisputeResolved) error {
	id := ev.GetReturnId()
	ret, err := h.returns.GetReturn(ctx, id)
	if errors.Is(err, store.ErrReturnNotFound) {
		return permanent("return %q not found", id)
	}
	if err != nil {
		return fmt.Errorf("get return %q: %w", id, err)
	}
	if ret.OrderID != ev.GetOrderId() {
		return permanent("return %q belongs to order %q, not %q", id, ret.OrderID, ev.GetOrderId())
	}
	if ret.Status == store.ReturnStatusRefunded {
		return nil
	}
	o, err := h.orders.GetOrder(ctx, ret.OrderID)
	if errors.Is(err, store.ErrOrderNotFound) {
		return permanent("order %q not found", ret.OrderID)
	}
	if err != nil {
		return fmt.Errorf("get order %q: %w", ret.OrderID, err)
	}
	online := o.PaidAt != nil
	if ret.Status == store.ReturnStatusApproved && !online {
		return nil
	}

	return h.gate.Do(ctx, auditx.Event{
		ActorID:    ev.GetResolverId(),
		Action:     disputeRefundAction,
		TargetType: "return_request",
		TargetID:   id,
		Metadata: map[string]string{
			"order_id":    ret.OrderID,
			"dispute_id":  ev.GetDisputeId(),
			"ruling":      ev.GetRuling().String(),
			"from_status": orderv1.ReturnStatus(ret.Status).String(),
		},
	}, func(ctx context.Context) error {
		return h.change(ctx, id, ret.Status, o.TotalAmount, online)
	})
}

func (h *DisputeHandler) change(ctx context.Context, id string, status store.ReturnStatus, orderTotal int64, online bool) error {
	switch status {
	case store.ReturnStatusPending:
		_, err := h.returns.TransitionReturn(ctx, id, store.ReturnStatusPending, store.ReturnStatusApproved)
		if err != nil && !errors.Is(err, store.ErrReturnStatusConflict) {
			return fmt.Errorf("approve return %q: %w", id, err)
		}
	case store.ReturnStatusRejected:
		_, err := h.returns.ReapproveReturn(ctx, id, orderTotal)
		switch {
		case err == nil, errors.Is(err, store.ErrReturnStatusConflict):
		case errors.Is(err, store.ErrReturnExceedsRemainder), errors.Is(err, store.ErrNoReturnableRemainder):
			return permanent("return %q does not fit the order remainder: %v", id, err)
		case errors.Is(err, store.ErrReturnNotFound):
			return permanent("return %q not found", id)
		default:
			return fmt.Errorf("reapprove return %q: %w", id, err)
		}
	}
	if !online {
		return nil
	}
	_, err := h.returns.TransitionReturn(ctx, id, store.ReturnStatusApproved, store.ReturnStatusRefunded)
	if err != nil && !errors.Is(err, store.ErrReturnStatusConflict) {
		return fmt.Errorf("refund return %q: %w", id, err)
	}
	return nil
}
