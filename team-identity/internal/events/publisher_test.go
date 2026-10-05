package events_test

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-identity/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/events"
)

func TestNoopPublisherClose(t *testing.T) {
	events.NoopPublisher{}.Close()
}

// The envelope carries the caller-supplied (stable outbox) event_id, the
// SessionRevoked type discriminator and a payload the gateway can decode.
func TestBuildSessionRevokedEnvelope(t *testing.T) {
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	value, err := events.BuildSessionRevokedEnvelope("evt-1", "sid-1", "user-1", exp,
		&commonv1.Principal{Id: "user-1"}, "req-1")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.GetEventId() != "evt-1" || env.GetType() != events.SessionRevokedEventType || env.GetRequestId() != "req-1" {
		t.Fatalf("unexpected envelope: %+v", &env)
	}
	var ev identityv1.SessionRevoked
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if ev.GetSessionId() != "sid-1" || ev.GetUserId() != "user-1" || !ev.GetExpiresAt().AsTime().Equal(exp) {
		t.Fatalf("unexpected payload: %+v", &ev)
	}
}
