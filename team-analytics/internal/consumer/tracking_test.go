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
	"github.com/buidangphuc/team-analytics/internal/warehouse"
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
		EventType:    analyticsv1.EventType_EVENT_TYPE_VIEW,
		ListingId:    "prod-1",
		SessionId:    "sess-9",
		AnonymousId:  "anon-7",
		PagePath:     "/listing/prod-1",
		Referrer:     "/",
		Position:     3,
		SearchQuery:  "laptop",
		PlacementId:  "home_feed",
		ImpressionId: "imp-uuid-123",
		ModelVersion: "als_v1",
		Properties:   map[string]string{"experiment": "a"},
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
	if rec.PlacementID != "home_feed" || rec.ImpressionID != "imp-uuid-123" || rec.ModelVersion != "als_v1" {
		t.Errorf("attribution fields not mapped: placement=%q imp=%q model=%q", rec.PlacementID, rec.ImpressionID, rec.ModelVersion)
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

func consentRecord(t *testing.T, te *analyticsv1.TrackingEvent) *warehouse.TrackingRecord {
	t.Helper()
	value := wrap(t, consumer.TrackingEventType, te, nil, time.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC))
	rec, ok, err := consumer.RecordFromEnvelope(value)
	if err != nil || !ok || rec == nil {
		t.Fatalf("RecordFromEnvelope: rec=%v ok=%v err=%v", rec, ok, err)
	}
	return rec
}

// Scenario: Consent and client id are persisted
func TestConsentAndClientIdArePersisted(t *testing.T) {
	rec := consentRecord(t, &analyticsv1.TrackingEvent{
		EventType:  analyticsv1.EventType_EVENT_TYPE_VIEW,
		GaClientId: "111.222",
		Consent:    &analyticsv1.ConsentState{Version: 1, Analytics: true, Marketing: false, Personalization: true},
	})
	if rec.GaClientID != "111.222" {
		t.Errorf("GaClientID = %q, want 111.222", rec.GaClientID)
	}
	if rec.ConsentVersion != 1 || !rec.ConsentAnalytics || rec.ConsentMarketing || !rec.ConsentPersonalization {
		t.Errorf("consent = v%d a=%v m=%v p=%v, want v1 a=true m=false p=true",
			rec.ConsentVersion, rec.ConsentAnalytics, rec.ConsentMarketing, rec.ConsentPersonalization)
	}
}

// Scenario: Client id is blanked without analytics consent
func TestClientIdIsBlankedWithoutAnalyticsConsent(t *testing.T) {
	rec := consentRecord(t, &analyticsv1.TrackingEvent{
		EventType:  analyticsv1.EventType_EVENT_TYPE_VIEW,
		GaClientId: "111.222",
		Consent:    &analyticsv1.ConsentState{Version: 1, Analytics: false, Marketing: true},
	})
	if rec.GaClientID != "" {
		t.Errorf("GaClientID = %q, want empty", rec.GaClientID)
	}
	if rec.ConsentAnalytics {
		t.Error("ConsentAnalytics = true, want false")
	}
}

// Scenario: A legacy payload without consent is stored as no consent
func TestLegacyPayloadWithoutConsentIsStoredAsNoConsent(t *testing.T) {
	rec := consentRecord(t, &analyticsv1.TrackingEvent{
		EventType:  analyticsv1.EventType_EVENT_TYPE_VIEW,
		GaClientId: "111.222",
	})
	if rec.ConsentVersion != 0 || rec.ConsentAnalytics || rec.ConsentMarketing || rec.ConsentPersonalization {
		t.Errorf("consent = v%d a=%v m=%v p=%v, want all zero",
			rec.ConsentVersion, rec.ConsentAnalytics, rec.ConsentMarketing, rec.ConsentPersonalization)
	}
	if rec.GaClientID != "" {
		t.Errorf("GaClientID = %q, want empty", rec.GaClientID)
	}
}

// recordForType maps a TrackingEvent of the given raw enum number.
func recordForType(t *testing.T, et analyticsv1.EventType, mut func(*analyticsv1.TrackingEvent)) *warehouse.TrackingRecord {
	t.Helper()
	te := &analyticsv1.TrackingEvent{EventType: et, SessionId: "s-1"}
	if mut != nil {
		mut(te)
	}
	return consentRecord(t, te)
}

// Scenario: Loop storage: each new type is stored under its name
func TestNewLoopEventTypesMapToNames(t *testing.T) {
	cases := []struct {
		et   analyticsv1.EventType
		want string
	}{
		{analyticsv1.EventType_EVENT_TYPE_SIGN_UP, "sign_up"},
		{analyticsv1.EventType_EVENT_TYPE_LOGIN, "login"},
		{analyticsv1.EventType_EVENT_TYPE_FOLLOW_SELLER, "follow_seller"},
		{analyticsv1.EventType_EVENT_TYPE_REFERRAL_REDEEM, "referral_redeem"},
		{analyticsv1.EventType_EVENT_TYPE_DAILY_CHECK_IN, "daily_check_in"},
		{analyticsv1.EventType_EVENT_TYPE_ALERT_SUBSCRIBE, "alert_subscribe"},
		{analyticsv1.EventType_EVENT_TYPE_AI_INTERACTION, "ai_interaction"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			rec := recordForType(t, c.et, nil)
			if rec.EventType != c.want {
				t.Errorf("EventType = %q, want %q", rec.EventType, c.want)
			}
			if rec.EventType == "unspecified" {
				t.Error("EventType must not be unspecified")
			}
		})
	}
}

// Scenario: Loop storage: existing types keep their names
func TestExistingEventTypeNamesUnchanged(t *testing.T) {
	cases := []struct {
		et   analyticsv1.EventType
		want string
	}{
		{analyticsv1.EventType_EVENT_TYPE_VIEW, "view"},
		{analyticsv1.EventType_EVENT_TYPE_CLICK, "click"},
		{analyticsv1.EventType_EVENT_TYPE_ADD_TO_CART, "add_to_cart"},
		{analyticsv1.EventType_EVENT_TYPE_IMPRESSION, "impression"},
		{analyticsv1.EventType_EVENT_TYPE_REMOVE_FROM_CART, "remove_from_cart"},
		{analyticsv1.EventType_EVENT_TYPE_BEGIN_CHECKOUT, "begin_checkout"},
		{analyticsv1.EventType_EVENT_TYPE_APPLY_PROMOTION, "apply_promotion"},
		{analyticsv1.EventType_EVENT_TYPE_SEARCH_FILTER, "search_filter"},
		{analyticsv1.EventType_EVENT_TYPE_FAVORITE, "favorite"},
		{analyticsv1.EventType_EVENT_TYPE_SHARE, "share"},
		{analyticsv1.EventType_EVENT_TYPE_VIEW_CART, "view_cart"},
		{analyticsv1.EventType_EVENT_TYPE_ADD_SHIPPING_INFO, "add_shipping_info"},
		{analyticsv1.EventType_EVENT_TYPE_ADD_PAYMENT_INFO, "add_payment_info"},
		{analyticsv1.EventType_EVENT_TYPE_PURCHASE, "purchase"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			if got := recordForType(t, c.et, nil).EventType; got != c.want {
				t.Errorf("EventType = %q, want %q", got, c.want)
			}
		})
	}
}

// Scenario: Loop storage: an unrecognised enum value is stored as unspecified without failing
func TestUnrecognisedEventTypeIsUnspecified(t *testing.T) {
	for _, n := range []int32{0, 22, 99, -1} {
		te := &analyticsv1.TrackingEvent{EventType: analyticsv1.EventType(n), SessionId: "s-1"}
		value := wrap(t, consumer.TrackingEventType, te, nil, time.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC))
		rec, ok, err := consumer.RecordFromEnvelope(value)
		if err != nil || !ok || rec == nil {
			t.Fatalf("enum %d: rec=%v ok=%v err=%v", n, rec, ok, err)
		}
		if rec.EventType != "unspecified" {
			t.Errorf("enum %d: EventType = %q, want unspecified", n, rec.EventType)
		}
	}
}

// Scenario: Loop storage: properties and listing id are preserved
func TestNewLoopEventsPreservePropertiesAndPrincipal(t *testing.T) {
	at := time.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC)
	principal := &commonv1.Principal{Id: "user-7", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER}
	cases := []struct {
		name    string
		te      *analyticsv1.TrackingEvent
		want    string
		props   map[string]string
		listing string
	}{
		{"ai_interaction", &analyticsv1.TrackingEvent{
			EventType: analyticsv1.EventType_EVENT_TYPE_AI_INTERACTION, ListingId: "l-1",
			SessionId: "s-1", AnonymousId: "a-1",
			Properties: map[string]string{"surface": "assistant", "action": "accepted"},
		}, "ai_interaction", map[string]string{"surface": "assistant", "action": "accepted"}, "l-1"},
		{"alert_subscribe", &analyticsv1.TrackingEvent{
			EventType: analyticsv1.EventType_EVENT_TYPE_ALERT_SUBSCRIBE, SessionId: "s-2",
			Properties: map[string]string{"alert_type": "price_drop"},
		}, "alert_subscribe", map[string]string{"alert_type": "price_drop"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, ok, err := consumer.RecordFromEnvelope(wrap(t, consumer.TrackingEventType, c.te, principal, at))
			if err != nil || !ok || rec == nil {
				t.Fatalf("rec=%v ok=%v err=%v", rec, ok, err)
			}
			if rec.EventType != c.want {
				t.Errorf("EventType = %q, want %q", rec.EventType, c.want)
			}
			if len(rec.Properties) != len(c.props) {
				t.Errorf("Properties = %v, want %v", rec.Properties, c.props)
			}
			for k, v := range c.props {
				if rec.Properties[k] != v {
					t.Errorf("Properties[%q] = %q, want %q", k, rec.Properties[k], v)
				}
			}
			if rec.ListingID != c.listing {
				t.Errorf("ListingID = %q, want %q", rec.ListingID, c.listing)
			}
			if rec.SessionID != c.te.SessionId || rec.AnonymousID != c.te.AnonymousId {
				t.Errorf("session/anon = %q/%q", rec.SessionID, rec.AnonymousID)
			}
			if rec.PrincipalID != "user-7" {
				t.Errorf("PrincipalID = %q, want user-7", rec.PrincipalID)
			}
		})
	}
}

// Scenario: Loop storage: an event with no properties is stored with empty properties
func TestReferralRedeemWithoutPropertiesIsStored(t *testing.T) {
	te := &analyticsv1.TrackingEvent{EventType: analyticsv1.EventType_EVENT_TYPE_REFERRAL_REDEEM, SessionId: "s-1"}
	rec, ok, err := consumer.RecordFromEnvelope(wrap(t, consumer.TrackingEventType, te, nil, time.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || rec == nil {
		t.Fatal("expected a record, got skip")
	}
	if rec.EventType != "referral_redeem" {
		t.Errorf("EventType = %q, want referral_redeem", rec.EventType)
	}
	if len(rec.Properties) != 0 {
		t.Errorf("Properties = %v, want empty", rec.Properties)
	}
}

// Scenario: Loop storage: the client id is blanked without analytics consent
func TestNewLoopEventClientIdBlankedWithoutConsent(t *testing.T) {
	rec := recordForType(t, analyticsv1.EventType_EVENT_TYPE_SIGN_UP, func(te *analyticsv1.TrackingEvent) {
		te.GaClientId = "111.222"
		te.Consent = &analyticsv1.ConsentState{Version: 1, Analytics: false}
	})
	if rec.EventType != "sign_up" {
		t.Errorf("EventType = %q, want sign_up", rec.EventType)
	}
	if rec.GaClientID != "" {
		t.Errorf("GaClientID = %q, want empty", rec.GaClientID)
	}
}
