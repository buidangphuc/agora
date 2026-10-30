// Package events builds identity's domain event envelopes (ADR-0002, ADR-0003
// addendum) and redacts relay log output.
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

// UserStatusChangedEventType is the EventEnvelope.Type discriminator for the
// UserStatusChanged event.
const UserStatusChangedEventType = "platform.identity.v1.UserStatusChanged"

// BuildUserStatusChangedEnvelope marshals the EventEnvelope for a user status
// change. The payload carries only contract fields: there is deliberately no
// note and no username parameter. until may be nil.
func BuildUserStatusChangedEnvelope(
	eventID, userID string,
	status, previous identityv1.UserStatus,
	reasonCode string,
	until *time.Time,
	changedAt time.Time,
	principal *commonv1.Principal,
	requestID string,
) ([]byte, error) {
	ev := &identityv1.UserStatusChanged{
		UserId:         userID,
		Status:         status,
		PreviousStatus: previous,
		ReasonCode:     reasonCode,
		ChangedAt:      timestamppb.New(changedAt),
	}
	if until != nil {
		ev.Until = timestamppb.New(*until)
	}
	payload, err := proto.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("marshal UserStatusChanged: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       UserStatusChangedEventType,
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
