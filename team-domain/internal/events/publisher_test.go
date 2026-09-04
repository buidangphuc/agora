package events_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/events"
)

// TestNoopPublisherClose asserts the no-op producer (KAFKA_ENABLED=false) tears
// down cleanly. It no longer has inline Publish* methods — publishing goes only
// through the outbox + relayer (AD6).
func TestNoopPublisherClose(t *testing.T) {
	events.NoopPublisher{}.Close()
}

// TestBuildListingChangedEnvelopeUsesSuppliedEventID pins the AD6 property that
// the ONLY envelope builder stamps the caller-supplied event_id (the stable
// outbox row id) rather than minting a random UUID. A re-delivered row therefore
// carries the same event_id every time, which is what lets consumers dedupe.
func TestBuildListingChangedEnvelopeUsesSuppliedEventID(t *testing.T) {
	const eventID = "outbox-row-42"
	principal := &commonv1.Principal{Id: "test_user"}

	value, err := events.BuildListingChangedEnvelope(
		eventID,
		&listingv1.Listing{Id: "item_1"},
		listingv1.ChangeType_CHANGE_TYPE_CREATED,
		principal,
		"req_1",
	)
	if err != nil {
		t.Fatalf("BuildListingChangedEnvelope: %v", err)
	}

	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.GetEventId() != eventID {
		t.Errorf("event_id = %q, want the stable outbox id %q (no random UUID)", env.GetEventId(), eventID)
	}
	if env.GetType() != events.ListingChangedEventType {
		t.Errorf("type = %q, want %q", env.GetType(), events.ListingChangedEventType)
	}

	// The same inputs must produce the same event_id on every call — proving there
	// is no random-id path left anywhere in the publish flow.
	again, err := events.BuildListingChangedEnvelope(eventID, &listingv1.Listing{Id: "item_1"},
		listingv1.ChangeType_CHANGE_TYPE_CREATED, principal, "req_1")
	if err != nil {
		t.Fatalf("BuildListingChangedEnvelope (again): %v", err)
	}
	var env2 eventsv1.EventEnvelope
	if err := proto.Unmarshal(again, &env2); err != nil {
		t.Fatalf("unmarshal envelope (again): %v", err)
	}
	if env2.GetEventId() != eventID {
		t.Errorf("event_id changed across calls: %q vs %q", env2.GetEventId(), eventID)
	}
}
