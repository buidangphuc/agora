package events_test

import (
	"bytes"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-identity/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/events"
)

const (
	secretNote     = "NOTE-spam-ring-7f3a9c"
	secretUsername = "alice_the_seller_x9"
)

func buildBan(t *testing.T, until *time.Time) (*eventsv1.EventEnvelope, []byte) {
	t.Helper()
	changed := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	// Signature contract: no note and no username parameter exists, so the
	// event cannot carry either (scenario: Event carries no note and no username).
	value, err := events.BuildUserStatusChangedEnvelope("evt-9", "user-1",
		identityv1.UserStatus_USER_STATUS_BANNED, identityv1.UserStatus_USER_STATUS_ACTIVE,
		"fraud", until, changed, &commonv1.Principal{Id: "admin-1"}, "req-9")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	return &env, value
}

// User enforcement: Event carries no note and no username
func TestUserStatusChangedPayloadMinimal(t *testing.T) {
	env, value := buildBan(t, nil)
	if env.GetType() != "platform.identity.v1.UserStatusChanged" ||
		events.UserStatusChangedEventType != "platform.identity.v1.UserStatusChanged" {
		t.Fatalf("type = %q", env.GetType())
	}
	if env.GetEventId() != "evt-9" || env.GetRequestId() != "req-9" {
		t.Fatalf("envelope ids: %+v", env)
	}
	var ev identityv1.UserStatusChanged
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if ev.GetUserId() != "user-1" || ev.GetStatus() != identityv1.UserStatus_USER_STATUS_BANNED ||
		ev.GetPreviousStatus() != identityv1.UserStatus_USER_STATUS_ACTIVE ||
		ev.GetReasonCode() != "fraud" || ev.GetUntil() != nil ||
		!ev.GetChangedAt().AsTime().Equal(time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("payload: %+v", &ev)
	}
	// Only contract fields: re-marshalling the decoded message is lossless
	// (no unknown fields), and neither secret text appears on the wire.
	if len(ev.ProtoReflect().GetUnknown()) != 0 {
		t.Fatalf("unknown fields in payload")
	}
	for _, s := range []string{secretNote, secretUsername} {
		if bytes.Contains(value, []byte(s)) {
			t.Fatalf("envelope contains %q", s)
		}
	}
}

// User enforcement: Event carries no note and no username (until is carried when set)
func TestUserStatusChangedCarriesUntil(t *testing.T) {
	until := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	env, _ := buildBan(t, &until)
	var ev identityv1.UserStatusChanged
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if ev.GetUntil() == nil || !ev.GetUntil().AsTime().Equal(until) {
		t.Fatalf("until = %v", ev.GetUntil())
	}
}
