package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"

	engagementv1 "github.com/buidangphuc/team-order/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/repository"
)

const (
	disputeResolvedType = "platform.engagement.v1.DisputeResolved"
	disputeConsumerName = "team-order.dispute"
	disputeRefundAction = "return.dispute_refund"
)

type DisputeReturnStore interface {
	GetReturn(ctx context.Context, id string) (repository.OrderReturn, error)
	TransitionReturn(ctx context.Context, id string, from, to repository.ReturnStatus) (repository.OrderReturn, error)
	ReapproveReturn(ctx context.Context, id string, orderTotal int64) (repository.OrderReturn, error)
}

type DisputeOrderStore interface {
	GetOrder(ctx context.Context, id string) (repository.Order, error)
}

type AdminAuditWriter interface {
	WriteAdminEvent(ctx context.Context, actorID, action, targetType, targetID string, metadata map[string]string) error
}

type DisputeConsumer struct {
	orders  DisputeOrderStore
	returns DisputeReturnStore
	audit   AdminAuditWriter
	dedupe  repository.ProcessedEventRepository
	logger  *slog.Logger
}

func NewDisputeConsumer(orders DisputeOrderStore, returns DisputeReturnStore, audit AdminAuditWriter, dedupe repository.ProcessedEventRepository, logger *slog.Logger) *DisputeConsumer {
	if logger == nil {
		logger = slog.Default()
	}
	if dedupe == nil {
		dedupe = repository.NewInMemoryProcessedEventRepository()
	}
	return &DisputeConsumer{orders: orders, returns: returns, audit: audit, dedupe: dedupe, logger: logger}
}

func (c *DisputeConsumer) HandleRaw(ctx context.Context, value []byte) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return fmt.Errorf("%w: unmarshal envelope: %v", ErrPermanent, err)
	}
	return c.HandleEnvelope(ctx, &env)
}

func (c *DisputeConsumer) Run(ctx context.Context, reader RecordReader, dlq DeadLetterProducer, cfg RunConfig) error {
	return runLoop(ctx, c.logger, c.HandleRaw, reader, dlq, cfg)
}

func (c *DisputeConsumer) HandleEnvelope(ctx context.Context, env *eventsv1.EventEnvelope) error {
	if env.GetType() != disputeResolvedType {
		return nil
	}
	if env.GetEventId() == "" {
		return fmt.Errorf("%w: envelope missing event_id", ErrPermanent)
	}
	already, err := c.dedupe.IsProcessed(ctx, disputeConsumerName, env.GetEventId())
	if err != nil {
		return fmt.Errorf("dedupe lookup: %w", err)
	}
	if already {
		return nil
	}

	var ev engagementv1.DisputeResolved
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		return fmt.Errorf("%w: unmarshal DisputeResolved: %v", ErrPermanent, err)
	}
	if ev.GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED ||
		ev.GetRuling() != engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR ||
		ev.GetReturnId() == "" {
		return nil
	}
	if ev.GetResolverId() == "" {
		return fmt.Errorf("%w: DisputeResolved missing resolver_id", ErrPermanent)
	}

	if err := c.apply(ctx, &ev); err != nil {
		return err
	}
	if _, err := c.dedupe.MarkProcessed(ctx, disputeConsumerName, env.GetEventId()); err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}
	return nil
}

func (c *DisputeConsumer) apply(ctx context.Context, ev *engagementv1.DisputeResolved) error {
	id := ev.GetReturnId()
	ret, err := c.returns.GetReturn(ctx, id)
	if errors.Is(err, repository.ErrReturnNotFound) {
		return fmt.Errorf("%w: return %q not found", ErrPermanent, id)
	}
	if err != nil {
		return fmt.Errorf("get return %q: %w", id, err)
	}
	if ret.OrderID != ev.GetOrderId() {
		return fmt.Errorf("%w: return %q belongs to order %q, not %q", ErrPermanent, id, ret.OrderID, ev.GetOrderId())
	}
	if ret.Status == repository.ReturnStatusRefunded {
		return nil
	}
	order, err := c.orders.GetOrder(ctx, ret.OrderID)
	if errors.Is(err, repository.ErrOrderNotFound) {
		return fmt.Errorf("%w: order %q not found", ErrPermanent, ret.OrderID)
	}
	if err != nil {
		return fmt.Errorf("get order %q: %w", ret.OrderID, err)
	}
	online := order.PaidAt != nil
	if ret.Status == repository.ReturnStatusApproved && !online {
		return nil
	}

	if c.audit == nil {
		return errors.New("audit client not configured; refusing to change the return")
	}
	meta := map[string]string{
		"order_id":    ret.OrderID,
		"dispute_id":  ev.GetDisputeId(),
		"ruling":      ev.GetRuling().String(),
		"from_status": orderv1.ReturnStatus(ret.Status).String(),
	}
	if err := c.audit.WriteAdminEvent(ctx, ev.GetResolverId(), disputeRefundAction, "return_request", id, meta); err != nil {
		return fmt.Errorf("audit %s: %w", disputeRefundAction, err)
	}

	if ret.Status == repository.ReturnStatusPending {
		_, err := c.returns.TransitionReturn(ctx, id, repository.ReturnStatusPending, repository.ReturnStatusApproved)
		if err != nil && !errors.Is(err, repository.ErrReturnStatusConflict) {
			return fmt.Errorf("approve return %q: %w", id, err)
		}
	} else if ret.Status == repository.ReturnStatusRejected {
		_, err := c.returns.ReapproveReturn(ctx, id, order.TotalAmount)
		switch {
		case err == nil, errors.Is(err, repository.ErrReturnStatusConflict):
		case errors.Is(err, repository.ErrReturnExceedsRemainder), errors.Is(err, repository.ErrNoReturnableRemainder):
			return fmt.Errorf("%w: return %q does not fit the order remainder: %v", ErrPermanent, id, err)
		case errors.Is(err, repository.ErrReturnNotFound):
			return fmt.Errorf("%w: return %q not found", ErrPermanent, id)
		default:
			return fmt.Errorf("reapprove return %q: %w", id, err)
		}
	}
	if !online {
		return nil
	}
	_, err = c.returns.TransitionReturn(ctx, id, repository.ReturnStatusApproved, repository.ReturnStatusRefunded)
	if err != nil && !errors.Is(err, repository.ErrReturnStatusConflict) {
		return fmt.Errorf("refund return %q: %w", id, err)
	}
	return nil
}
