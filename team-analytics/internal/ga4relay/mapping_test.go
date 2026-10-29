package ga4relay

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-analytics/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
)

// fakeClock is a deterministic, manually advanced clock.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestHandler(t *testing.T) (*MappingHandler, MappingStore, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
	store := NewMemoryMappingStore(clk.Now)
	return NewMappingHandler(store), store, clk
}

func trackingRec(t *testing.T, principalID, principalType, clientID string, analytics bool) consume.Record {
	t.Helper()
	pt := commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	if principalType == "anonymous" {
		pt = commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS
	}
	te, err := proto.Marshal(&analyticsv1.TrackingEvent{
		GaClientId:  clientID,
		AnonymousId: "anon-1",
		Consent:     &analyticsv1.ConsentState{Analytics: analytics, Version: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	env, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId: "ev", Type: "platform.analytics.v1.TrackingEvent", Payload: te,
		Principal: &commonv1.Principal{Id: principalID, Type: pt},
	})
	if err != nil {
		t.Fatal(err)
	}
	return consume.Record{Topic: "analytics.events", Key: []byte(principalID), Value: env}
}

func ev(t *testing.T, buyer, clientID string, analytics bool) consume.Record {
	t.Helper()
	return trackingRec(t, buyer, "user", clientID, analytics)
}

func mustGet(ctx context.Context, t *testing.T, s MappingStore, buyer string) (string, bool) {
	t.Helper()
	id, ok, err := s.Get(ctx, buyer)
	if err != nil {
		t.Fatalf("Get(%q): %v", buyer, err)
	}
	return id, ok
}

func TestMappingLatestConsentedClientIDWins(t *testing.T) { // Scenario: Latest consented client id wins
	h, s, clk := newTestHandler(t)
	ctx := context.Background()
	if err := h.Handle(ctx, ev(t, "b1", "111.1", true)); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Minute)
	if err := h.Handle(ctx, ev(t, "b1", "222.2", true)); err != nil {
		t.Fatal(err)
	}
	if id, ok := mustGet(ctx, t, s, "b1"); !ok || id != "222.2" {
		t.Fatalf("mapping = %q,%v; want 222.2,true", id, ok)
	}
}

func TestMappingEmptyClientIDDoesNotOverwrite(t *testing.T) { // Scenario: Latest consented client id wins (non-empty requirement)
	h, s, _ := newTestHandler(t)
	ctx := context.Background()
	_ = h.Handle(ctx, ev(t, "b1", "111.1", true))
	if err := h.Handle(ctx, ev(t, "b1", "", true)); err != nil {
		t.Fatal(err)
	}
	if id, ok := mustGet(ctx, t, s, "b1"); !ok || id != "111.1" {
		t.Fatalf("mapping = %q,%v; want 111.1,true", id, ok)
	}
}

func TestMappingDeniedConsentDeletesMapping(t *testing.T) { // Scenario: Denied consent deletes the mapping
	h, s, _ := newTestHandler(t)
	ctx := context.Background()
	_ = h.Handle(ctx, ev(t, "b1", "222.2", true))
	if err := h.Handle(ctx, ev(t, "b1", "222.2", false)); err != nil {
		t.Fatal(err)
	}
	if id, ok := mustGet(ctx, t, s, "b1"); ok {
		t.Fatalf("mapping still present: %q", id)
	}
	if got := h.SkippedCount("no_consent"); got != 1 {
		t.Fatalf("ga4_relay_mapping_skipped_total{no_consent} = %d; want 1", got)
	}
}

func TestMappingDeniedConsentDoesNotAffectOtherBuyers(t *testing.T) { // Scenario: Denied consent deletes the mapping
	h, s, _ := newTestHandler(t)
	ctx := context.Background()
	_ = h.Handle(ctx, ev(t, "b1", "111.1", true))
	_ = h.Handle(ctx, ev(t, "b2", "222.2", true))
	_ = h.Handle(ctx, ev(t, "b1", "", false))
	if id, ok := mustGet(ctx, t, s, "b2"); !ok || id != "222.2" {
		t.Fatalf("b2 mapping = %q,%v; want 222.2,true", id, ok)
	}
}

func TestMappingAnonymousEventIsIgnored(t *testing.T) { // Scenario: Anonymous event is ignored
	h, s, _ := newTestHandler(t)
	ctx := context.Background()
	rec := trackingRec(t, "", "anonymous", "333.3", true)
	if err := h.Handle(ctx, rec); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", "anon-1"} {
		if id, ok := mustGet(ctx, t, s, key); ok {
			t.Fatalf("mapping written for key %q: %q", key, id)
		}
	}
}

func TestMappingExpiresAfter30Days(t *testing.T) { // Scenario: Mapping expires
	h, s, clk := newTestHandler(t)
	ctx := context.Background()
	if err := h.Handle(ctx, ev(t, "b1", "222.2", true)); err != nil {
		t.Fatal(err)
	}
	clk.Advance(30*24*time.Hour - time.Second)
	if id, ok := mustGet(ctx, t, s, "b1"); !ok || id != "222.2" {
		t.Fatalf("before 30d: mapping = %q,%v; want 222.2,true", id, ok)
	}
	clk.Advance(2 * time.Second) // now 30d + 1s after the last consented event
	if id, ok := mustGet(ctx, t, s, "b1"); ok {
		t.Fatalf("after 30d: mapping still present: %q", id)
	}
}

func TestMappingTTLRefreshedByNewConsentedEvent(t *testing.T) { // Scenario: Mapping expires ("no new consented event")
	h, s, clk := newTestHandler(t)
	ctx := context.Background()
	_ = h.Handle(ctx, ev(t, "b1", "111.1", true))
	clk.Advance(20 * 24 * time.Hour)
	_ = h.Handle(ctx, ev(t, "b1", "111.1", true))
	clk.Advance(20 * 24 * time.Hour) // 40d after first, 20d after last
	if id, ok := mustGet(ctx, t, s, "b1"); !ok || id != "111.1" {
		t.Fatalf("mapping = %q,%v; want 111.1,true", id, ok)
	}
}

func TestMappingConsentRevokedBeforePayment(t *testing.T) { // Scenario: Consent revoked before payment
	h, s, clk := newTestHandler(t)
	ctx := context.Background()
	_ = h.Handle(ctx, ev(t, "b1", "222.2", true))
	clk.Advance(time.Hour)
	// latest event before PAID has analytics=false, even carrying a client id
	if err := h.Handle(ctx, ev(t, "b1", "222.2", false)); err != nil {
		t.Fatal(err)
	}
	if id, ok := mustGet(ctx, t, s, "b1"); ok {
		t.Fatalf("lookup at payment time found %q; want no mapping", id)
	}
}
