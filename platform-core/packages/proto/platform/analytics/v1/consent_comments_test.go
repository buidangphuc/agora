package analyticsv1_test

// Comment-assertion tests for change ga4-consent-contract: the contract text
// consumers rely on must live in analytics.proto comments.

import (
	"strings"
	"testing"
)

func requireComment(t *testing.T, got string, ok bool, subs ...string) {
	t.Helper()
	if !ok {
		t.Fatalf("no comment found on element")
	}
	for _, s := range subs {
		if !strings.Contains(got, s) {
			t.Errorf("comment missing %q; got:\n%s", s, got)
		}
	}
}

func TestConsentComments_VersionZeroWithGrantedFlagsIsNotTrusted(t *testing.T) {
	f := loadFile(t)
	_, mi := f.message(t, "ConsentState")
	m, _ := f.message(t, "ConsentState")
	_, fi := m.field(t, "version")
	c, ok := f.comment(4, mi, 2, fi)
	requireComment(t, c, ok, "version = 0 is treated as all denied regardless of flags")
}

func TestConsentComments_UnknownOrNegativeVersionIsDenied(t *testing.T) {
	f := loadFile(t)
	m, mi := f.message(t, "ConsentState")
	_, fi := m.field(t, "version")
	c, ok := f.comment(4, mi, 2, fi)
	requireComment(t, c, ok, "negative or unknown version (known: 1) is treated as all denied")
}

func TestConsentComments_ClientIdStatesFormatAndBlanking(t *testing.T) {
	f := loadFile(t)
	m, mi := f.message(t, "TrackingEvent")
	_, fi := m.field(t, "ga_client_id")
	c, ok := f.comment(4, mi, 2, fi)
	requireComment(t, c, ok, "without the GA1.1. prefix", "blanked by consumers when consent.analytics is false")
}

func TestConsentComments_ClientIdBlankedEventKept(t *testing.T) {
	f := loadFile(t)
	m, mi := f.message(t, "TrackingEvent")
	_, fi := m.field(t, "ga_client_id")
	c, ok := f.comment(4, mi, 2, fi)
	requireComment(t, c, ok, "event is kept")
}

func TestConsentComments_ConsentStampedByEdgeFromCookie(t *testing.T) {
	f := loadFile(t)
	m, mi := f.message(t, "TrackingEvent")
	_, fi := m.field(t, "consent")
	c, ok := f.comment(4, mi, 2, fi)
	requireComment(t, c, ok, "agora_consent")
}

func TestConsentComments_ScopeDocumented(t *testing.T) {
	f := loadFile(t)
	for si, s := range f.Service {
		if s.Name != "AnalyticsQueryService" {
			continue
		}
		for mi, mm := range s.Method {
			if mm.Name == "GetChannelFunnel" {
				c, ok := f.comment(6, si, 2, mi)
				requireComment(t, c, ok, `Admin-only (scope "admin")`)
				return
			}
		}
	}
	t.Fatal("GetChannelFunnel RPC missing")
}
