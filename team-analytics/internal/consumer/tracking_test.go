package consumer_test

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-analytics/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	"github.com/buidangphuc/team-analytics/internal/consumer"
)

// wrap marshals a payload into an EventEnvelope of the given type.
func wrap(t *testing.T, typ string, payload proto.Message, principal *commonv1.Principal, at time.Time) []byte {
	t.Helper()
	var body []byte
	if payload != nil {
		b, err := proto.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = b
	}
	env := &eventsv1.EventEnvelope{
		EventId:    "evt-1",
		Type:       typ,
		OccurredAt: timestamppb.New(at),
		Principal:  principal,
		Payload:    body,
	}
	v, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return v
}

// TestTrackingEnvelopeMapsToRecord covers the spec scenario "A produced tracking
// event lands in the warehouse": the behavioral fields + envelope principal
// appear on the record.
func TestTrackingEnvelopeMapsToRecord(t *testing.T) {
	at := time.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC)
	te := &analyticsv1.TrackingEvent{
		EventType:   analyticsv1.EventType_EVENT_TYPE_VIEW,
		ListingId:   "prod-1",
		SessionId:   "sess-9",
		AnonymousId: "anon-7",
		PagePath:    "/listing/prod-1",
		Referrer:    "/",
		Position:    3,
		SearchQuery: "laptop",
		Properties:  map[string]string{"experiment": "a"},
	}
	value := wrap(t, consumer.TrackingEventType, te,
		&commonv1.Principal{Id: "user-1", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER}, at)

	rec, ok, err := consumer.RecordFromEnvelope(value)
	if err != nil {
		t.Fatalf("RecordFromEnvelope: %v", err)
	}
	if !ok || rec == nil {
		t.Fatal("expected a tracking record, got skip")
	}
	if rec.EventID != "evt-1" {
		t.Errorf("EventID = %q, want evt-1", rec.EventID)
	}
	if rec.EventType != "view" {
		t.Errorf("EventType = %q, want view", rec.EventType)
	}
	if rec.ListingID != "prod-1" || rec.SessionID != "sess-9" || rec.PagePath != "/listing/prod-1" {
		t.Errorf("behavioral fields not mapped: %+v", rec)
	}
	if rec.Position != 3 || rec.SearchQuery != "laptop" {
		t.Errorf("position/query not mapped: pos=%d q=%q", rec.Position, rec.SearchQuery)
	}
	if !rec.OccurredAt.Equal(at) {
		t.Errorf("OccurredAt = %v, want %v", rec.OccurredAt, at)
	}
	if rec.PrincipalID != "user-1" || rec.PrincipalType != "user" {
		t.Errorf("principal not mapped: id=%q type=%q", rec.PrincipalID, rec.PrincipalType)
	}
	if rec.Properties["experiment"] != "a" {
		t.Errorf("properties not mapped: %v", rec.Properties)
	}
}

// TestNonTrackingEnvelopeIsSkipped covers the spec scenario "Non-tracking
// envelopes are ignored": a wrong-type envelope is skipped without a row and
// without an error.
func TestNonTrackingEnvelopeIsSkipped(t *testing.T) {
	// A well-formed envelope of a different type (payload is irrelevant here).
	value := wrap(t, "platform.listing.v1.ListingChanged", nil, nil, time.Now())

	rec, ok, err := consumer.RecordFromEnvelope(value)
	if err != nil {
		t.Fatalf("unexpected error skipping non-tracking envelope: %v", err)
	}
	if ok || rec != nil {
		t.Fatalf("expected skip, got record %+v", rec)
	}
}

// TestMalformedEnvelopeErrors: a corrupt value is a real error, not a silent
// skip (so it is logged, not mistaken for a non-tracking envelope).
func TestMalformedEnvelopeErrors(t *testing.T) {
	if _, _, err := consumer.RecordFromEnvelope([]byte{0xff, 0xff, 0xff, 0xff}); err == nil {
		t.Fatal("expected error for malformed envelope bytes")
	}
}
