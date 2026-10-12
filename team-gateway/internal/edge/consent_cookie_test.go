package edge

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
)

const consentBeacon = `{"type":"view","listingId":"l1","sessionId":"s","anonymousId":"a"}`

func trackWith(t *testing.T, body string, hdr map[string]string) (*httptest.ResponseRecorder, *mockAnalyticsPublisher) {
	t.Helper()
	pub := &mockAnalyticsPublisher{}
	h := HandleTrack(&Edge{publicScopes: []string{"public"}}, pub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader([]byte(body)))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, pub
}

func cookieHdr(v string) map[string]string { return map[string]string{"Cookie": "agora_consent=" + v} }

func assertConsent(t *testing.T, ev *analyticsv1.TrackingEvent, a, m, p bool, ver int32) {
	t.Helper()
	c := ev.GetConsent()
	if c == nil {
		t.Fatalf("consent not set on event")
	}
	if c.GetAnalytics() != a || c.GetMarketing() != m || c.GetPersonalization() != p || c.GetVersion() != ver {
		t.Fatalf("consent = a:%v m:%v p:%v v:%d, want a:%v m:%v p:%v v:%d",
			c.GetAnalytics(), c.GetMarketing(), c.GetPersonalization(), c.GetVersion(), a, m, p, ver)
	}
}

func assertDeniedV0(t *testing.T, rec *httptest.ResponseRecorder, pub *mockAnalyticsPublisher) {
	t.Helper()
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	if len(pub.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(pub.events))
	}
	assertConsent(t, pub.events[0], false, false, false, 0)
}

// Scenario: Valid cookie is stamped on each event
func TestConsentStamp_ValidCookieIsStampedOnEachEvent(t *testing.T) {
	body := "[" + consentBeacon + "," + consentBeacon + "," + consentBeacon + "]"
	rec, pub := trackWith(t, body, cookieHdr("v=1&a=1&m=0&p=1&ts=1760000000"))
	if rec.Code != http.StatusAccepted || len(pub.events) != 3 {
		t.Fatalf("code=%d events=%d", rec.Code, len(pub.events))
	}
	for _, ev := range pub.events {
		assertConsent(t, ev, true, false, true, 1)
	}
}

// Scenario: Rejected buyer is flagged personalization-denied
func TestConsentStamp_RejectedBuyerIsFlaggedPersonalizationDenied(t *testing.T) {
	rec, pub := trackWith(t, consentBeacon, cookieHdr("v=1&a=0&m=0&p=0&ts=1760000000"))
	if rec.Code != http.StatusAccepted || len(pub.events) != 1 {
		t.Fatalf("code=%d events=%d", rec.Code, len(pub.events))
	}
	assertConsent(t, pub.events[0], false, false, false, 1)
}

// Scenario: Body consent is ignored
func TestConsentStamp_BodyConsentIsIgnored(t *testing.T) {
	body := `{"type":"view","listingId":"l1","sessionId":"s","anonymousId":"a","consent":{"personalization":true}}`
	rec, pub := trackWith(t, body, cookieHdr("v=1&a=0&m=0&p=0&ts=1760000000"))
	if rec.Code != http.StatusAccepted || len(pub.events) != 1 {
		t.Fatalf("code=%d events=%d", rec.Code, len(pub.events))
	}
	if pub.events[0].GetConsent().GetPersonalization() {
		t.Fatalf("body consent leaked into event")
	}
	assertConsent(t, pub.events[0], false, false, false, 1)
}

// Scenario: No cookie stamps version 0
func TestConsentStamp_NoCookieStampsVersion0(t *testing.T) {
	rec, pub := trackWith(t, consentBeacon, nil)
	assertDeniedV0(t, rec, pub)
}

// Scenario: Non-0/1 flag degrades to denied
func TestConsentStamp_Non01FlagDegradesToDenied(t *testing.T) {
	rec, pub := trackWith(t, consentBeacon, cookieHdr("v=1&a=1&m=1&p=7&ts=1760000000"))
	assertDeniedV0(t, rec, pub)
}

// Scenario: Unknown version degrades to denied
func TestConsentStamp_UnknownVersionDegradesToDenied(t *testing.T) {
	rec, pub := trackWith(t, consentBeacon, cookieHdr("v=9&a=1&m=1&p=1&ts=1760000000"))
	assertDeniedV0(t, rec, pub)
}

// Scenario: Duplicate key degrades to denied
func TestConsentStamp_DuplicateKeyDegradesToDenied(t *testing.T) {
	rec, pub := trackWith(t, consentBeacon, cookieHdr("a=1&a=0&v=1&m=0&p=0&ts=1760000000"))
	assertDeniedV0(t, rec, pub)
}

// Scenario: Non-numeric ts degrades to denied
func TestConsentStamp_NonNumericTsDegradesToDenied(t *testing.T) {
	rec, pub := trackWith(t, consentBeacon, cookieHdr("v=1&a=1&m=1&p=1&ts=x"))
	assertDeniedV0(t, rec, pub)
}

// Scenario: Missing ts degrades to denied
func TestConsentStamp_MissingTsDegradesToDenied(t *testing.T) {
	rec, pub := trackWith(t, consentBeacon, cookieHdr("v=1&a=1&m=1&p=1"))
	assertDeniedV0(t, rec, pub)
}

func paddedConsent(total int) string {
	base := "v=1&a=1&m=0&p=1&ts=1760000000&x="
	return base + strings.Repeat("y", total-len(base))
}

// Scenario: Oversized cookie degrades to denied
func TestConsentStamp_OversizedCookieDegradesToDenied(t *testing.T) {
	v := paddedConsent(257)
	if len(v) != 257 {
		t.Fatalf("fixture len %d", len(v))
	}
	rec, pub := trackWith(t, consentBeacon, cookieHdr(v))
	assertDeniedV0(t, rec, pub)
}

// Scenario: Cookie of exactly 256 bytes is accepted
func TestConsentStamp_CookieOfExactly256BytesIsAccepted(t *testing.T) {
	v := paddedConsent(256)
	if len(v) != 256 {
		t.Fatalf("fixture len %d", len(v))
	}
	rec, pub := trackWith(t, consentBeacon, cookieHdr(v))
	if rec.Code != http.StatusAccepted || len(pub.events) != 1 {
		t.Fatalf("code=%d events=%d", rec.Code, len(pub.events))
	}
	assertConsent(t, pub.events[0], true, false, true, 1)
}

// Scenario: Consent header does not stand in for the cookie
func TestConsentStamp_ConsentHeaderDoesNotStandInForCookie(t *testing.T) {
	rec, pub := trackWith(t, consentBeacon, map[string]string{"X-Consent": "v=1&a=1&m=1&p=1&ts=1"})
	assertDeniedV0(t, rec, pub)
}
