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

func decodeRegistered(t *testing.T, value []byte) (*eventsv1.EventEnvelope, *identityv1.UserRegistered) {
	t.Helper()
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var ev identityv1.UserRegistered
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return &env, &ev
}

// Scenario: One event per successful registration (envelope type and payload fields)
func TestBuildUserRegisteredEnvelopeTypeAndPayload(t *testing.T) {
	registered := time.Date(2026, 11, 6, 8, 15, 0, 0, time.UTC)
	value, err := events.BuildUserRegisteredEnvelope("evt-ur-1", "user-9", "AB12cd_-", registered,
		&commonv1.Principal{Id: "user-9", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER}, "req-ur-1")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if events.UserRegisteredEventType != "platform.identity.v1.UserRegistered" {
		t.Fatalf("event type const = %q", events.UserRegisteredEventType)
	}
	env, ev := decodeRegistered(t, value)
	if env.GetType() != "platform.identity.v1.UserRegistered" || env.GetEventId() != "evt-ur-1" ||
		env.GetRequestId() != "req-ur-1" || env.GetPrincipal().GetId() != "user-9" {
		t.Fatalf("envelope: %+v", env)
	}
	if env.GetOccurredAt() == nil {
		t.Fatalf("envelope has no occurred_at")
	}
	if ev.GetUserId() != "user-9" || ev.GetReferralCode() != "AB12cd_-" || !ev.GetRegisteredAt().AsTime().Equal(registered) {
		t.Fatalf("payload: %+v", ev)
	}
	if len(ev.ProtoReflect().GetUnknown()) != 0 {
		t.Fatalf("unknown fields in payload")
	}
}

// Scenario: Register without a code succeeds (payload carries an empty referral_code)
func TestBuildUserRegisteredEnvelopeEmptyCode(t *testing.T) {
	value, err := events.BuildUserRegisteredEnvelope("evt-ur-2", "user-10", "", time.Unix(1_700_000_000, 0), nil, "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	_, ev := decodeRegistered(t, value)
	if ev.GetReferralCode() != "" || ev.GetUserId() != "user-10" {
		t.Fatalf("payload: %+v", ev)
	}
}

// Scenario: Redelivery carries a stable event id
func TestBuildUserRegisteredEnvelopeStableEventID(t *testing.T) {
	registered := time.Unix(1_700_000_000, 0)
	a, err := events.BuildUserRegisteredEnvelope("evt-stable", "user-11", "REFa1B2c3D4", registered, nil, "r")
	if err != nil {
		t.Fatal(err)
	}
	b, err := events.BuildUserRegisteredEnvelope("evt-stable", "user-11", "REFa1B2c3D4", registered, nil, "r")
	if err != nil {
		t.Fatal(err)
	}
	envA, evA := decodeRegistered(t, a)
	envB, evB := decodeRegistered(t, b)
	if envA.GetEventId() != "evt-stable" || envB.GetEventId() != "evt-stable" {
		t.Fatalf("event ids = %q, %q; want evt-stable twice", envA.GetEventId(), envB.GetEventId())
	}
	if !bytes.Equal(envA.GetPayload(), envB.GetPayload()) || evA.GetUserId() != evB.GetUserId() {
		t.Fatalf("payload must be identical across builds")
	}
}

// Scenario: Case is preserved
func TestBuildUserRegisteredEnvelopeKeepsCase(t *testing.T) {
	value, err := events.BuildUserRegisteredEnvelope("e", "u", "REFa1B2c3D4", time.Unix(1, 0), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	_, ev := decodeRegistered(t, value)
	if ev.GetReferralCode() != "REFa1B2c3D4" {
		t.Fatalf("referral_code = %q", ev.GetReferralCode())
	}
}

// Scenario: Register logs contain no raw identifiers (relay redaction of the referral code)
func TestRelayLogsDoNotCarryReferralCode(t *testing.T) {
	var buf bytes.Buffer
	log := newRedactingLogger(t, &buf)

	log.Error("relay publish failed", "aggregate_id", "u-777", "referral_code", "REFa1B2c3D4", "event_id", "evt-1")

	out := buf.String()
	if bytes.Contains([]byte(out), []byte("REFa1B2c3D4")) {
		t.Fatalf("captured output leaks the referral code: %s", out)
	}
	if bytes.Contains([]byte(out), []byte("u-777")) {
		t.Fatalf("captured output leaks the user id: %s", out)
	}
}
