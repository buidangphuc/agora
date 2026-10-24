package events

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	commonv1 "github.com/buidangphuc/team-promotion/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-promotion/generated/platform/events/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
)

const (
	TypeVoucherChanged   = "platform.promotion.v1.VoucherChanged"
	TypeFlashSaleChanged = "platform.promotion.v1.FlashSaleChanged"
)

func Voucher(ctx context.Context, topic string, v *promotionv1.Voucher) (outbox.Row, error) {
	payload, err := proto.Marshal(&promotionv1.VoucherChanged{Voucher: v})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal VoucherChanged: %w", err)
	}
	return row(ctx, topic, "voucher", v.GetId(), TypeVoucherChanged, payload)
}

func FlashSale(ctx context.Context, topic string, c *promotionv1.FlashSaleCampaign) (outbox.Row, error) {
	payload, err := proto.Marshal(&promotionv1.FlashSaleChanged{Campaign: c})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal FlashSaleChanged: %w", err)
	}
	return row(ctx, topic, "flash_sale", c.GetId(), TypeFlashSaleChanged, payload)
}

func row(ctx context.Context, topic, aggregateType, id, eventType string, payload []byte) (outbox.Row, error) {
	eventID := uuid.NewString()
	reqID := reqctx.RequestID(ctx)
	envelope, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:     eventID,
		Type:        eventType,
		OccurredAt:  timestamppb.New(time.Now().UTC()),
		Principal:   principalFrom(ctx),
		Traceparent: traceparent(ctx),
		RequestId:   reqID,
		Payload:     payload,
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return outbox.Row{
		EventID:       eventID,
		AggregateType: aggregateType,
		AggregateID:   id,
		EventType:     eventType,
		Topic:         topic,
		Payload:       envelope,
		RequestID:     reqID,
	}, nil
}

func principalFrom(ctx context.Context) *commonv1.Principal {
	p, ok := auth.FromContext(ctx)
	if !ok || p.ID == "" {
		return nil
	}
	return &commonv1.Principal{Id: p.ID, Type: wireType(p.Type)}
}

func wireType(t auth.PrincipalType) commonv1.PrincipalType {
	switch t {
	case auth.TypeUser:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	case auth.TypeService:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE
	case auth.TypeAnonymous:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS
	default:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_UNSPECIFIED
	}
}

func traceparent(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return fmt.Sprintf("00-%s-%s-%s", sc.TraceID(), sc.SpanID(), sc.TraceFlags())
}
