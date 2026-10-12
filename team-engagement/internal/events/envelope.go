// Package events holds team-engagement's Kafka side: the engagement facts written
// to the outbox as full EventEnvelopes, the outbox retention component, and the
// listing.events consumer that fills the follow feed.
//
// Delivery of the outbox rows is the go-sdk relayer's job (outbox.Relayer): rows
// are published verbatim, so the envelope is built when the fact is written.
package events

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
)

// BuildEnvelope marshals the standard EventEnvelope around a fact payload. The
// principal is the caller recorded with the fact (nil when none), and typ is the
// payload's full proto name.
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

// FactRow builds the outbox row for one engagement fact. It carries the whole
// marshalled EventEnvelope, with a fresh event id (the key consumers dedupe on),
// the caller's principal and request id taken from ctx, and the fact's full proto
// name as type. key is the Kafka partition key (a listing id, or a seller id for
// follows) and aggregateType names what it identifies.
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

// principalFrom returns the caller in its wire form (id and type; scopes are not
// published), or nil when the call carries no principal.
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

func wireType(t string) commonv1.PrincipalType {
	switch t {
	case "user":
		return commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	case "service":
		return commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE
	case "anonymous":
		return commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS
	default:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_UNSPECIFIED
	}
}
