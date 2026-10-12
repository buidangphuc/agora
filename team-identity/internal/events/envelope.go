// Package events builds identity's domain events (ADR-0002, ADR-0003 addendum).
// Events are wrapped in a platform.events.v1.EventEnvelope, keyed by user id for
// per-user ordering, and written through the transactional outbox; the SDK outbox
// relayer wired in internal/app drains stored rows to the topic.
package events

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-identity/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
)

// SessionRevokedEventType is the discriminator carried in EventEnvelope.Type (and
// stamped on the outbox row) for the SessionRevoked event.
const SessionRevokedEventType = "platform.identity.v1.SessionRevoked"

// BuildSessionRevokedEnvelope marshals the EventEnvelope for a revoked session with
// a caller-supplied eventID (the outbox row id), so a re-delivered row carries a
// STABLE event_id. expiresAt is the revoked token's expiry: the gateway keeps the
// denylist entry until then.
func BuildSessionRevokedEnvelope(
	eventID, sessionID, userID string,
	expiresAt time.Time,
	principal *commonv1.Principal,
	requestID string,
) ([]byte, error) {
	payload, err := proto.Marshal(&identityv1.SessionRevoked{
		SessionId: sessionID,
		UserId:    userID,
		ExpiresAt: timestamppb.New(expiresAt),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal SessionRevoked: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       SessionRevokedEventType,
		OccurredAt: timestamppb.Now(),
		Principal:  principal,
		RequestId:  requestID,
		Payload:    payload,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return value, nil
}
