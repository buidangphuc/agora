// Package events builds the chat.events outbox row written in the message
// transaction (ADR-0002, ADR-0013).
package events

import (
	"context"
	"fmt"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	commonv1 "github.com/buidangphuc/team-chat/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-chat/generated/platform/events/v1"
	"github.com/buidangphuc/team-chat/internal/store"
)

// Discriminator types carried in EventEnvelope.Type
const (
	ChatMessageSentType = "platform.chat.v1.ChatMessage"
)

// chatMessageNamespace seeds deterministic event ids so a message maps onto exactly
// one outbox row (ON CONFLICT DO NOTHING) and one consumer-side dedupe key.
var chatMessageNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("agora/team-chat/chat.events/ChatMessage"))

// ChatMessageEventID is the stable EventEnvelope.event_id for a message's event.
func ChatMessageEventID(messageID string) string {
	return uuid.NewSHA1(chatMessageNamespace, []byte(messageID)).String()
}

// BuildMessageOutboxRow is the repository.MessageOutboxBuilder for team-chat: it
// wraps the message (with recipient_id) in an EventEnvelope keyed by thread id,
// ready to commit in the same transaction as the message. The caller's principal
// and request id are read from ctx.
func BuildMessageOutboxRow(ctx context.Context, msg store.ChatMessage) (outbox.Row, error) {
	occurredAt := msg.CreatedAt
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	payload, err := proto.Marshal(&chatv1.ChatMessage{
		Id:          msg.ID,
		ThreadId:    msg.ThreadID,
		SenderId:    msg.SenderID,
		SenderName:  msg.SenderName,
		Content:     msg.Content,
		CreatedAt:   timestamppb.New(occurredAt),
		MessageType: chatv1.MessageType(msg.MessageType),
		ListingId:   msg.ListingID,
		Payload:     msg.Payload,
		RecipientId: msg.RecipientID,
		SellerId:    msg.SellerID,
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal ChatMessage: %w", err)
	}
	reqID := reqctx.RequestID(ctx)
	eventID := ChatMessageEventID(msg.ID)
	envelope := &eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       ChatMessageSentType,
		OccurredAt: timestamppb.New(occurredAt.UTC()),
		RequestId:  reqID,
		Payload:    payload,
	}
	if p, ok := auth.FromContext(ctx); ok {
		envelope.Principal = &commonv1.Principal{Id: p.ID, Type: principalType(p.Type), Scopes: p.Scopes}
	}
	value, err := proto.Marshal(envelope)
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return outbox.Row{
		EventID:       eventID,
		AggregateType: "ChatThread",
		AggregateID:   msg.ThreadID,
		EventType:     ChatMessageSentType,
		Payload:       value,
		RequestID:     reqID,
	}, nil
}

func principalType(t auth.PrincipalType) commonv1.PrincipalType {
	switch t {
	case auth.TypeUser:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	case auth.TypeService:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE
	case auth.TypeAnonymous:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS
	}
	return commonv1.PrincipalType_PRINCIPAL_TYPE_UNSPECIFIED
}
