package events

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
)

func BuildEnvelope(eventID, typ string, occurredAt time.Time, p *commonv1.Principal, requestID string, payload []byte) ([]byte, error) {
	b, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       typ,
		OccurredAt: timestamppb.New(occurredAt),
		Principal:  p,
		RequestId:  requestID,
		Payload:    payload,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return b, nil
}

func FactRow(ctx context.Context, aggregateType, key string, msg proto.Message) (outbox.Row, error) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal fact: %w", err)
	}
	typ := string(msg.ProtoReflect().Descriptor().FullName())
	id := uuid.NewString()
	reqID := reqctx.RequestID(ctx)
	envelope, err := BuildEnvelope(id, typ, time.Now(), principalFrom(ctx), reqID, payload)
	if err != nil {
		return outbox.Row{}, err
	}
	return outbox.Row{
		EventID:       id,
		AggregateType: aggregateType,
		AggregateID:   key,
		EventType:     typ,
		Payload:       envelope,
		RequestID:     reqID,
	}, nil
}

func principalFrom(ctx context.Context) *commonv1.Principal {
	p, ok := auth.FromContext(ctx)
	if !ok {
		return nil
	}
	t := wireType(p.Type)
	if p.ID == "" && t == commonv1.PrincipalType_PRINCIPAL_TYPE_UNSPECIFIED {
		return nil
	}
	return &commonv1.Principal{Id: p.ID, Type: t}
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
