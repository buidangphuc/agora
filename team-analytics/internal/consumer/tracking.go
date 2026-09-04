package consumer

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-analytics/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// TrackingEventType is the EventEnvelope.Type discriminator this worker acts on.
// Any other type is skipped (spec: "Non-tracking envelopes are ignored").
const TrackingEventType = "platform.analytics.v1.TrackingEvent"

// RecordFromEnvelope decodes one Kafka record value: it unmarshals the
// EventEnvelope, skips it (ok=false) when it is not a TrackingEvent, and
// otherwise unmarshals the payload and maps envelope + TrackingEvent into a
// driver-neutral warehouse.TrackingRecord. A malformed envelope/payload is a
// real error (returned); a well-formed non-tracking envelope is a clean skip.
func RecordFromEnvelope(value []byte) (rec *warehouse.TrackingRecord, ok bool, err error) {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return nil, false, fmt.Errorf("unmarshal envelope: %w", err)
	}
	if env.GetType() != TrackingEventType {
		return nil, false, nil // not ours; skip without error
	}
	var te analyticsv1.TrackingEvent
	if err := proto.Unmarshal(env.GetPayload(), &te); err != nil {
		return nil, false, fmt.Errorf("unmarshal TrackingEvent: %w", err)
	}

	principal := env.GetPrincipal()
	return &warehouse.TrackingRecord{
		EventID:       env.GetEventId(),
		EventType:     eventTypeName(te.GetEventType()),
		ListingID:     te.GetListingId(),
		SessionID:     te.GetSessionId(),
		AnonymousID:   te.GetAnonymousId(),
		PagePath:      te.GetPagePath(),
		Referrer:      te.GetReferrer(),
		Position:      te.GetPosition(),
		SearchQuery:   te.GetSearchQuery(),
		OccurredAt:    env.GetOccurredAt().AsTime().UTC(),
		PrincipalID:   principal.GetId(),
		PrincipalType: principalTypeName(principal.GetType()),
		Properties:    te.GetProperties(),
	}, true, nil
}

// eventTypeName normalizes the EventType enum to a compact lowercase name that
// is stable across DuckDB and BigQuery (stored as a STRING column).
func eventTypeName(t analyticsv1.EventType) string {
	switch t {
	case analyticsv1.EventType_EVENT_TYPE_VIEW:
		return "view"
	case analyticsv1.EventType_EVENT_TYPE_CLICK:
		return "click"
	case analyticsv1.EventType_EVENT_TYPE_ADD_TO_CART:
		return "add_to_cart"
	case analyticsv1.EventType_EVENT_TYPE_IMPRESSION:
		return "impression"
	default:
		return "unspecified"
	}
}

// principalTypeName normalizes the PrincipalType enum to a lowercase name.
func principalTypeName(t commonv1.PrincipalType) string {
	switch t {
	case commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS:
		return "anonymous"
	case commonv1.PrincipalType_PRINCIPAL_TYPE_USER:
		return "user"
	case commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE:
		return "service"
	default:
		return "unspecified"
	}
}
