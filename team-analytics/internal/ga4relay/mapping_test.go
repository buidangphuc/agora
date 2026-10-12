package ga4relay

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
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

// ev builds a tracking record as the consumer emits it: the buyer is the
// envelope principal id, consent comes from the TrackingEvent consent state.
func ev(buyer, clientID string, analytics bool) warehouse.TrackingRecord {
	return warehouse.TrackingRecord{
		PrincipalID:      buyer,
		PrincipalType:    "user",
		GaClientID:       clientID,
		ConsentAnalytics: analytics,
	}
}

func mustGet(t *testing.T, s MappingStore, buyer string) (string, bool) {
	t.Helper()
	id, ok, err := s.Get(context.Background(), buyer)
	if err != nil {
		t.Fatalf("Get(%q): %v", buyer, err)
	}
	return id, ok
}

func TestMappingLatestConsentedClientIDWins(t *testing.T) { // Scenario: Latest consented client id wins
	h, s, clk := newTestHandler(t)
	ctx := context.Background()
	if err := h.Handle(ctx, ev("b1", "111.1", true)); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Minute)
	if err := h.Handle(ctx, ev("b1", "222.2", true)); err != nil {
		t.Fatal(err)
	}
	if id, ok := mustGet(t, s, "b1"); !ok || id != "222.2" {
		t.Fatalf("mapping = %q,%v; want 222.2,true", id, ok)
	}
}

func TestMappingEmptyClientIDDoesNotOverwrite(t *testing.T) { // Scenario: Latest consented client id wins (non-empty requirement)
	h, s, _ := newTestHandler(t)
	ctx := context.Background()
	_ = h.Handle(ctx, ev("b1", "111.1", true))
	if err := h.Handle(ctx, ev("b1", "", true)); err != nil {
		t.Fatal(err)
	}
	if id, ok := mustGet(t, s, "b1"); !ok || id != "111.1" {
		t.Fatalf("mapping = %q,%v; want 111.1,true", id, ok)
	}
}

func TestMappingDeniedConsentDeletesMapping(t *testing.T) { // Scenario: Denied consent deletes the mapping
	h, s, _ := newTestHandler(t)
	ctx := context.Background()
	_ = h.Handle(ctx, ev("b1", "222.2", true))
	if err := h.Handle(ctx, ev("b1", "222.2", false)); err != nil {
		t.Fatal(err)
	}
	if id, ok := mustGet(t, s, "b1"); ok {
		t.Fatalf("mapping still present: %q", id)
	}
	if got := h.SkippedCount("no_consent"); got != 1 {
		t.Fatalf("ga4_relay_mapping_skipped_total{no_consent} = %d; want 1", got)
	}
}

func TestMappingDeniedConsentDoesNotAffectOtherBuyers(t *testing.T) { // Scenario: Denied consent deletes the mapping
	h, s, _ := newTestHandler(t)
	ctx := context.Background()
	_ = h.Handle(ctx, ev("b1", "111.1", true))
	_ = h.Handle(ctx, ev("b2", "222.2", true))
	_ = h.Handle(ctx, ev("b1", "", false))
	if id, ok := mustGet(t, s, "b2"); !ok || id != "222.2" {
		t.Fatalf("b2 mapping = %q,%v; want 222.2,true", id, ok)
	}
}

func TestMappingAnonymousEventIsIgnored(t *testing.T) { // Scenario: Anonymous event is ignored
	h, s, _ := newTestHandler(t)
	ctx := context.Background()
	rec := ev("", "333.3", true)
	rec.PrincipalType = "anonymous"
	rec.AnonymousID = "anon-1"
	if err := h.Handle(ctx, rec); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", "anon-1"} {
		if id, ok := mustGet(t, s, key); ok {
			t.Fatalf("mapping written for key %q: %q", key, id)
		}
	}
}

func TestMappingExpiresAfter30Days(t *testing.T) { // Scenario: Mapping expires
	h, s, clk := newTestHandler(t)
	ctx := context.Background()
	if err := h.Handle(ctx, ev("b1", "222.2", true)); err != nil {
		t.Fatal(err)
	}
	clk.Advance(30*24*time.Hour - time.Second)
	if id, ok := mustGet(t, s, "b1"); !ok || id != "222.2" {
		t.Fatalf("before 30d: mapping = %q,%v; want 222.2,true", id, ok)
	}
	clk.Advance(2 * time.Second) // now 30d + 1s after the last consented event
	if id, ok := mustGet(t, s, "b1"); ok {
		t.Fatalf("after 30d: mapping still present: %q", id)
	}
}

func TestMappingTTLRefreshedByNewConsentedEvent(t *testing.T) { // Scenario: Mapping expires ("no new consented event")
	h, s, clk := newTestHandler(t)
	ctx := context.Background()
	_ = h.Handle(ctx, ev("b1", "111.1", true))
	clk.Advance(20 * 24 * time.Hour)
	_ = h.Handle(ctx, ev("b1", "111.1", true))
	clk.Advance(20 * 24 * time.Hour) // 40d after first, 20d after last
	if id, ok := mustGet(t, s, "b1"); !ok || id != "111.1" {
		t.Fatalf("mapping = %q,%v; want 111.1,true", id, ok)
	}
}

func TestMappingConsentRevokedBeforePayment(t *testing.T) { // Scenario: Consent revoked before payment
	h, s, clk := newTestHandler(t)
	ctx := context.Background()
	_ = h.Handle(ctx, ev("b1", "222.2", true))
	clk.Advance(time.Hour)
	// latest event before PAID has analytics=false, even carrying a client id
	if err := h.Handle(ctx, ev("b1", "222.2", false)); err != nil {
		t.Fatal(err)
	}
	if id, ok := mustGet(t, s, "b1"); ok {
		t.Fatalf("lookup at payment time found %q; want no mapping", id)
	}
}
