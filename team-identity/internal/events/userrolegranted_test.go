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

// Seller onboarding: the first grant emits one event
func TestBuildUserRoleGrantedEnvelope(t *testing.T) {
	granted := time.Date(2026, 11, 4, 9, 30, 0, 0, time.UTC)
	value, err := events.BuildUserRoleGranted("evt-rg-1", "user-7", "seller", granted,
		&commonv1.Principal{Id: "user-7"}, "req-rg-1")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if events.UserRoleGrantedEventType != "platform.identity.v1.UserRoleGranted" {
		t.Fatalf("event type const = %q", events.UserRoleGrantedEventType)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.GetType() != "platform.identity.v1.UserRoleGranted" || env.GetEventId() != "evt-rg-1" ||
		env.GetRequestId() != "req-rg-1" || env.GetPrincipal().GetId() != "user-7" {
		t.Fatalf("envelope: %+v", &env)
	}
	if env.GetOccurredAt() == nil {
		t.Fatalf("envelope has no occurred_at")
	}
	var ev identityv1.UserRoleGranted
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if ev.GetUserId() != "user-7" || ev.GetRole() != "seller" || !ev.GetGrantedAt().AsTime().Equal(granted) {
		t.Fatalf("payload: %+v", &ev)
	}
	if len(ev.ProtoReflect().GetUnknown()) != 0 {
		t.Fatalf("unknown fields in payload")
	}
}

// Seller onboarding: the first grant emits one event (event id is stable, caller supplied)
func TestBuildUserRoleGrantedKeepsEventID(t *testing.T) {
	a, err := events.BuildUserRoleGranted("evt-a", "u", "seller", time.Unix(1, 0), nil, "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(a, &env); err != nil {
		t.Fatal(err)
	}
	if env.GetEventId() != "evt-a" {
		t.Fatalf("event id = %q", env.GetEventId())
	}
}
