package edge

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
	"github.com/buidangphuc/team-gateway/internal/token"
)

type mockAnalyticsPublisher struct {
	events []*analyticsv1.TrackingEvent
	ids    []string
}

func (m *mockAnalyticsPublisher) PublishTrackingEvent(
	_ context.Context,
	ev *analyticsv1.TrackingEvent,
	_ *commonv1.Principal,
	_ string,
	eventID string,
) error {
	m.ids = append(m.ids, eventID)
	m.events = append(m.events, ev)
	return nil
}

func (m *mockAnalyticsPublisher) Close() {}

func TestHandleTrackWithAttribution(t *testing.T) {
	edge := &Edge{
		publicScopes: []string{"public"},
	}
	pub := &mockAnalyticsPublisher{}
	logger := slog.New(slog.DiscardHandler)
	handler := HandleTrack(edge, pub, logger)

	beacon := trackBeacon{
		Type:         "view",
		ListingID:    "listing-123",
		SessionID:    "sess-abc",
		AnonymousID:  "anon-xyz",
		Path:         "/listing/listing-123",
		Referrer:     "/home",
		Position:     1,
		Query:        "sneakers",
		PlacementID:  "home_feed",
		ImpressionID: "imp-uuid-999",
		ModelVersion: "als_v1",
		Properties: map[string]string{
			"source": "widget",
		},
	}

	body, err := json.Marshal(beacon)
	if err != nil {
		t.Fatalf("marshal beacon: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", rec.Code)
	}

	if len(pub.events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.events))
	}

	ev := pub.events[0]
	if ev.GetPlacementId() != "home_feed" {
		t.Errorf("expected placement_id 'home_feed', got %q", ev.GetPlacementId())
	}
	if ev.GetImpressionId() != "imp-uuid-999" {
		t.Errorf("expected impression_id 'imp-uuid-999', got %q", ev.GetImpressionId())
	}
	if ev.GetModelVersion() != "als_v1" {
		t.Errorf("expected model_version 'als_v1', got %q", ev.GetModelVersion())
	}
	if ev.GetListingId() != "listing-123" {
		t.Errorf("expected listing_id 'listing-123', got %q", ev.GetListingId())
	}
}

func TestHandleTrackEcommerceBatchAndGA4Aliases(t *testing.T) {
	edge := &Edge{
		publicScopes: []string{"public"},
	}
	pub := &mockAnalyticsPublisher{}
	logger := slog.New(slog.DiscardHandler)
	handler := HandleTrack(edge, pub, logger)

	batch := []trackBeacon{
		{
			Type:         "view_item_list", // GA4 alias for impression
			ListingID:    "prod-1",
			SessionID:    "sess-1",
			AnonymousID:  "anon-1",
			Price:        50000,
			Currency:     "VND",
			ItemListID:   "search_results",
			Position:     1,
			EventGroupID: "group-abc",
			ItemCategory: "Fashion",
		},
		{
			Type:          "purchase",
			ListingID:     "prod-2",
			SessionID:     "sess-1",
			AnonymousID:   "anon-1",
			Value:         150000,
			Price:         100000,
			Quantity:      1,
			Currency:      "VND",
			TransactionID: "order-999",
			EventGroupID:  "group-abc",
			ShippingTier:  "SPX_EXPRESS",
			PaymentType:   "MOCK_WALLET",
		},
	}

	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("marshal batch: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", rec.Code)
	}

	if len(pub.events) != 2 {
		t.Fatalf("expected 2 published events, got %d", len(pub.events))
	}

	ev1 := pub.events[0]
	if ev1.GetEventType() != analyticsv1.EventType_EVENT_TYPE_IMPRESSION {
		t.Errorf("expected EVENT_TYPE_IMPRESSION, got %v", ev1.GetEventType())
	}
	if ev1.GetPrice() != 50000 || ev1.GetCurrency() != "VND" || ev1.GetEventGroupId() != "group-abc" {
		t.Errorf("mismatched ecommerce fields on ev1: %+v", ev1)
	}

	ev2 := pub.events[1]
	if ev2.GetEventType() != analyticsv1.EventType_EVENT_TYPE_PURCHASE {
		t.Errorf("expected EVENT_TYPE_PURCHASE, got %v", ev2.GetEventType())
	}
	if ev2.GetTransactionId() != "order-999" || ev2.GetShippingTier() != "SPX_EXPRESS" || ev2.GetPaymentType() != "MOCK_WALLET" {
		t.Errorf("mismatched ecommerce fields on ev2: %+v", ev2)
	}
}

func TestHandleTrackExceedsBatchLimit(t *testing.T) {
	edge := &Edge{
		publicScopes: []string{"public"},
	}
	pub := &mockAnalyticsPublisher{}
	logger := slog.New(slog.DiscardHandler)
	handler := HandleTrack(edge, pub, logger)

	oversized := make([]trackBeacon, 101)
	for i := range oversized {
		oversized[i] = trackBeacon{
			Type:      "view",
			ListingID: "item",
		}
	}

	body, err := json.Marshal(oversized)
	if err != nil {
		t.Fatalf("marshal oversized: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for batch exceeding maxBatchItems, got %d", rec.Code)
	}
}

// ---- gateway-loop-event-intake: loop and AI beacon types ----

type loopPublished struct {
	ev *analyticsv1.TrackingEvent
	pr *commonv1.Principal
}

type loopRecorder struct{ got []loopPublished }

func (l *loopRecorder) PublishTrackingEvent(_ context.Context, ev *analyticsv1.TrackingEvent, pr *commonv1.Principal, _, _ string) error {
	l.got = append(l.got, loopPublished{ev, pr})
	return nil
}
func (l *loopRecorder) Close() {}

func loopPost(t *testing.T, e *Edge, body string, mod func(*http.Request)) (*loopRecorder, *httptest.ResponseRecorder) {
	t.Helper()
	pub := &loopRecorder{}
	h := HandleTrack(e, pub, slog.New(slog.DiscardHandler))
	req := httptest.NewRequest(http.MethodPost, "/api/track", strings.NewReader(body))
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return pub, rec
}

func loopCounts(t *testing.T, rec *httptest.ResponseRecorder) (int, int) {
	t.Helper()
	var r trackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return r.Accepted, r.Dropped
}

func loopWantRejected(t *testing.T, body string) {
	t.Helper()
	pub, rec := loopPost(t, &Edge{}, body, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("%s: want 400, got %d", body, rec.Code)
	}
	if len(pub.got) != 0 {
		t.Fatalf("%s: published %d events, want 0", body, len(pub.got))
	}
}

func aiBody(surface, action string) string {
	return fmt.Sprintf(`{"type":"ai_interaction","sessionId":"s","properties":{"surface":%q,"action":%q}}`, surface, action)
}

// Scenario: Loop intake: each activation and loop beacon becomes a tracking event.
func TestHandleTrackLoopEventTypes(t *testing.T) {
	cases := map[string]analyticsv1.EventType{
		"sign_up":         analyticsv1.EventType_EVENT_TYPE_SIGN_UP,
		"login":           analyticsv1.EventType_EVENT_TYPE_LOGIN,
		"follow_seller":   analyticsv1.EventType_EVENT_TYPE_FOLLOW_SELLER,
		"referral_redeem": analyticsv1.EventType_EVENT_TYPE_REFERRAL_REDEEM,
		"daily_check_in":  analyticsv1.EventType_EVENT_TYPE_DAILY_CHECK_IN,
		"alert_subscribe": analyticsv1.EventType_EVENT_TYPE_ALERT_SUBSCRIBE,
		" Sign_UP ":       analyticsv1.EventType_EVENT_TYPE_SIGN_UP,
	}
	for typ, want := range cases {
		t.Run(strings.TrimSpace(typ), func(t *testing.T) {
			pub, rec := loopPost(t, &Edge{}, fmt.Sprintf(`{"type":%q,"sessionId":"marker-1"}`, typ), nil)
			if rec.Code != http.StatusAccepted {
				t.Fatalf("code %d", rec.Code)
			}
			if a, _ := loopCounts(t, rec); a != 1 {
				t.Fatalf("accepted %d", a)
			}
			if len(pub.got) != 1 {
				t.Fatalf("published %d", len(pub.got))
			}
			if got := pub.got[0].ev.GetEventType(); got != want {
				t.Errorf("event type %v, want %v", got, want)
			}
			if pub.got[0].ev.GetSessionId() != "marker-1" {
				t.Errorf("sessionId %q", pub.got[0].ev.GetSessionId())
			}
		})
	}
}

// Scenario: Loop intake: properties of a loop beacon reach the event.
func TestHandleTrackLoopEventProperties(t *testing.T) {
	pub, rec := loopPost(t, &Edge{}, `[
		{"type":"follow_seller","properties":{"seller_id":"seller-42"}},
		{"type":"daily_check_in","properties":{"streak":"7"}}]`, nil)
	if rec.Code != http.StatusAccepted || len(pub.got) != 2 {
		t.Fatalf("code %d published %d", rec.Code, len(pub.got))
	}
	if p := pub.got[0].ev.GetProperties(); len(p) != 1 || p["seller_id"] != "seller-42" {
		t.Errorf("follow_seller props %v", p)
	}
	if p := pub.got[1].ev.GetProperties(); len(p) != 1 || p["streak"] != "7" {
		t.Errorf("daily_check_in props %v", p)
	}
}

func loopSignedEdge(t *testing.T, sub string) (*Edge, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	b64 := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "kid-1",
			"n": b64(key.N.Bytes()),
			"e": b64(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	t.Cleanup(jwks.Close)
	e := NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read"}, time.Second, 0, 1000, 1000)
	claims := &token.Claims{
		Type:   "user",
		Scopes: []string{"listing.read"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tk := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tk.Header["kid"] = "kid-1"
	signed, err := tk.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return e, signed
}

// Scenario: Loop intake: the envelope principal is the signed-in buyer.
func TestHandleTrackLoopEventPrincipalFromCookie(t *testing.T) {
	e, tok := loopSignedEdge(t, "buyer-7")
	pub, rec := loopPost(t, e, `{"type":"login","sessionId":"s","properties":{"userId":"someone-else"}}`, func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: "session", Value: tok})
	})
	if rec.Code != http.StatusAccepted || len(pub.got) != 1 {
		t.Fatalf("code %d published %d", rec.Code, len(pub.got))
	}
	if pub.got[0].ev.GetEventType() != analyticsv1.EventType_EVENT_TYPE_LOGIN {
		t.Fatalf("event type %v", pub.got[0].ev.GetEventType())
	}
	pr := pub.got[0].pr
	if pr.GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER || pr.GetId() != "buyer-7" {
		t.Errorf("principal %v, want user buyer-7", pr)
	}
}

// Scenario: Loop intake: an anonymous caller is attributed to the anonymous principal.
func TestHandleTrackLoopEventAnonymousPrincipal(t *testing.T) {
	pub, rec := loopPost(t, &Edge{}, `{"type":"sign_up","sessionId":"s"}`, nil)
	if rec.Code != http.StatusAccepted || len(pub.got) != 1 {
		t.Fatalf("code %d published %d", rec.Code, len(pub.got))
	}
	if pub.got[0].ev.GetEventType() != analyticsv1.EventType_EVENT_TYPE_SIGN_UP {
		t.Fatalf("event type %v", pub.got[0].ev.GetEventType())
	}
	if ty := pub.got[0].pr.GetType(); ty != commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS {
		t.Errorf("principal type %v", ty)
	}
}

// Scenario: Loop intake: free text in a loop property is scrubbed.
func TestHandleTrackLoopEventScrubsProperties(t *testing.T) {
	pub, rec := loopPost(t, &Edge{}, `{"type":"follow_seller","properties":{"seller_id":"x a.b@example.com y"}}`, nil)
	if rec.Code != http.StatusAccepted || len(pub.got) != 1 {
		t.Fatalf("code %d published %d", rec.Code, len(pub.got))
	}
	if pub.got[0].ev.GetEventType() != analyticsv1.EventType_EVENT_TYPE_FOLLOW_SELLER {
		t.Fatalf("event type %v", pub.got[0].ev.GetEventType())
	}
	if v := pub.got[0].ev.GetProperties()["seller_id"]; strings.Contains(v, "a.b@example.com") {
		t.Errorf("email survived scrubbing: %q", v)
	}
}

// Scenario: Loop intake: an oversized loop property is rejected.
func TestHandleTrackLoopEventBounds(t *testing.T) {
	one := func(val string) string {
		return fmt.Sprintf(`{"type":"referral_redeem","properties":{"code":%q}}`, val)
	}
	keys := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = fmt.Sprintf(`"k%d":"v"`, i)
		}
		return `{"type":"referral_redeem","properties":{` + strings.Join(parts, ",") + `}}`
	}
	loopWantRejected(t, one(strings.Repeat("a", 257)))
	loopWantRejected(t, keys(21))
	for name, body := range map[string]string{"256 chars": one(strings.Repeat("a", 256)), "20 keys": keys(20)} {
		pub, rec := loopPost(t, &Edge{}, body, nil)
		if rec.Code != http.StatusAccepted {
			t.Errorf("%s: code %d", name, rec.Code)
			continue
		}
		if a, _ := loopCounts(t, rec); a != 1 || len(pub.got) != 1 {
			t.Errorf("%s: accepted %d published %d", name, a, len(pub.got))
		}
	}
}

// Scenario: Loop intake: existing beacon types are unchanged.
func TestHandleTrackExistingTypesUnchanged(t *testing.T) {
	pub, rec := loopPost(t, &Edge{}, `[{"type":"view"},{"type":"share"},{"type":"favorite"},{"type":"select_item"}]`, nil)
	if rec.Code != http.StatusAccepted || len(pub.got) != 4 {
		t.Fatalf("code %d published %d", rec.Code, len(pub.got))
	}
	want := []analyticsv1.EventType{
		analyticsv1.EventType_EVENT_TYPE_VIEW, analyticsv1.EventType_EVENT_TYPE_SHARE,
		analyticsv1.EventType_EVENT_TYPE_FAVORITE, analyticsv1.EventType_EVENT_TYPE_CLICK,
	}
	for i, w := range want {
		if got := pub.got[i].ev.GetEventType(); got != w {
			t.Errorf("beacon %d: %v, want %v", i, got, w)
		}
	}
}

// Scenario: Loop intake: an unknown type is still rejected.
func TestHandleTrackUnknownTypeStillRejected(t *testing.T) {
	loopWantRejected(t, `{"type":"teleport","sessionId":"s"}`)
}

// Scenario: Loop intake: a valid AI interaction becomes a tracking event.
func TestHandleTrackAIInteractionAccepted(t *testing.T) {
	pub, rec := loopPost(t, &Edge{}, `{"type":"ai_interaction","listingId":"lst-1","properties":{"surface":"assistant","action":"accepted"}}`, nil)
	if rec.Code != http.StatusAccepted || len(pub.got) != 1 {
		t.Fatalf("code %d published %d", rec.Code, len(pub.got))
	}
	ev := pub.got[0].ev
	if ev.GetEventType() != analyticsv1.EventType_EVENT_TYPE_AI_INTERACTION {
		t.Errorf("event type %v", ev.GetEventType())
	}
	if ev.GetListingId() != "lst-1" {
		t.Errorf("listing id %q", ev.GetListingId())
	}
	if p := ev.GetProperties(); len(p) != 2 || p["surface"] != "assistant" || p["action"] != "accepted" {
		t.Errorf("props %v", p)
	}
}

// Scenario: Loop intake: every allowlisted surface and action pair is accepted.
func TestHandleTrackAIInteractionAllowlistMatrix(t *testing.T) {
	var items []string
	for _, s := range []string{"assistant", "chat_copilot", "magic_listing", "review_summary"} {
		for _, a := range []string{"shown", "accepted", "edited", "dismissed"} {
			items = append(items, aiBody(s, a))
		}
	}
	if len(items) != 16 {
		t.Fatalf("setup: %d", len(items))
	}
	total := 0
	for start := 0; start < len(items); start += 100 {
		end := min(start+100, len(items))
		pub, rec := loopPost(t, &Edge{}, "["+strings.Join(items[start:end], ",")+"]", nil)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("code %d", rec.Code)
		}
		a, d := loopCounts(t, rec)
		if d != 0 || len(pub.got) != a {
			t.Fatalf("accepted %d dropped %d published %d", a, d, len(pub.got))
		}
		total += a
	}
	if total != 16 {
		t.Errorf("accepted sum %d, want 16", total)
	}
}

// Scenario: Loop intake: an AI interaction with a surface outside the allowlist is rejected.
func TestHandleTrackAIInteractionBadSurface(t *testing.T) {
	loopWantRejected(t, aiBody("search_bar", "shown"))
}

// Scenario: Loop intake: an AI interaction with an action outside the allowlist is rejected.
func TestHandleTrackAIInteractionBadAction(t *testing.T) {
	loopWantRejected(t, aiBody("assistant", "typed_a_prompt"))
}

// Scenario: Loop intake: an AI interaction missing surface or action is rejected.
func TestHandleTrackAIInteractionMissingProperties(t *testing.T) {
	loopWantRejected(t, `{"type":"ai_interaction"}`)
	loopWantRejected(t, `{"type":"ai_interaction","properties":{"surface":"assistant"}}`)
	loopWantRejected(t, `{"type":"ai_interaction","properties":{"action":"shown"}}`)
}

// Scenario: Loop intake: AI allowlist matching is exact.
func TestHandleTrackAIInteractionExactMatch(t *testing.T) {
	loopWantRejected(t, aiBody("Assistant", "shown"))
	loopWantRejected(t, aiBody("assistant ", "shown"))
	loopWantRejected(t, aiBody("assistant", "SHOWN"))
}

// Scenario: Loop intake: an invalid AI beacon does not drop the valid ones in a batch.
func TestHandleTrackAIInteractionBatchIndependent(t *testing.T) {
	pub, rec := loopPost(t, &Edge{}, "["+aiBody("review_summary", "shown")+","+aiBody("free_text", "shown")+"]", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("code %d", rec.Code)
	}
	if a, d := loopCounts(t, rec); a != 1 || d != 1 {
		t.Fatalf("accepted %d dropped %d", a, d)
	}
	if len(pub.got) != 1 || pub.got[0].ev.GetProperties()["surface"] != "review_summary" {
		t.Fatalf("published %v", pub.got)
	}
}

// Scenario: Loop intake: an AI interaction is attributed by the cookie not by the body.
func TestHandleTrackAIInteractionAnonymous(t *testing.T) {
	pub, rec := loopPost(t, &Edge{}, aiBody("assistant", "shown"), nil)
	if rec.Code != http.StatusAccepted || len(pub.got) != 1 {
		t.Fatalf("code %d published %d", rec.Code, len(pub.got))
	}
	if pub.got[0].ev.GetEventType() != analyticsv1.EventType_EVENT_TYPE_AI_INTERACTION {
		t.Fatalf("event type %v", pub.got[0].ev.GetEventType())
	}
	if ty := pub.got[0].pr.GetType(); ty != commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS {
		t.Errorf("principal type %v", ty)
	}
}

// Scenario: Loop intake: an AI interaction with an extra property key is rejected.
func TestHandleTrackAIInteractionExtraKeyRejected(t *testing.T) {
	loopWantRejected(t, `{"type":"ai_interaction","properties":{"surface":"assistant","action":"shown","prompt":"hello"}}`)
}

// Scenario: Loop intake: an extra-key AI beacon in a batch is dropped and counted.
func TestHandleTrackAIInteractionExtraKeyBatch(t *testing.T) {
	pub, rec := loopPost(t, &Edge{}, `[`+aiBody("assistant", "shown")+`,{"type":"ai_interaction","properties":{"surface":"assistant","action":"shown","note":"x"}}]`, nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("code %d", rec.Code)
	}
	if a, d := loopCounts(t, rec); a != 1 || d != 1 {
		t.Fatalf("accepted %d dropped %d", a, d)
	}
	if len(pub.got) != 1 || len(pub.got[0].ev.GetProperties()) != 2 {
		t.Fatalf("published %v", pub.got)
	}
}

// Scenario: Loop intake: an AI beacon type with odd casing is still validated.
func TestHandleTrackAIInteractionTypeCasingValidated(t *testing.T) {
	loopWantRejected(t, `{"type":" AI_Interaction ","properties":{"surface":"search_bar","action":"shown"}}`)
}
