package duckdb

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/store"
)

// attrFixture opens a file-backed warehouse, writes one tracking event per
// (id, time) and tags it with ga_client_id, seeds ga4_sessions with the given
// SQL VALUES tuples, then reopens so views are (re)created over the final schema.
//
// ga_client_id is a tracking_events column added by ga4-consent-contract; the
// ALTER ... IF NOT EXISTS keeps this fixture independent of which task lands first.
type attrEvent struct{ id, client, at string }

func attrFixture(t *testing.T, events []attrEvent, sessions []string) *Writer {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "w.duckdb")
	w, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.DB().Exec("ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS ga_client_id VARCHAR"); err != nil {
		t.Fatal(err)
	}
	var recs []*store.TrackingRecord
	for _, e := range events {
		at, err := time.Parse("2006-01-02 15:04:05", e.at)
		if err != nil {
			t.Fatal(err)
		}
		recs = append(recs, &store.TrackingRecord{EventID: e.id, EventType: "view", ListingID: "l1", AnonymousID: "a-" + e.client, OccurredAt: at})
	}
	if len(recs) > 0 {
		if err := w.Write(ctx, recs); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range events {
		if _, err := w.DB().Exec("UPDATE tracking_events SET ga_client_id = ? WHERE event_id = ?", e.client, e.id); err != nil {
			t.Fatal(err)
		}
	}
	if len(sessions) > 0 {
		q := "INSERT INTO ga4_sessions (export_date, user_pseudo_id, session_id, source, medium, campaign, landing_page, ts, ended_at) VALUES " + strings.Join(sessions, ",")
		if _, err := w.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	w2, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w2.Close() })
	return w2
}

func sess(client, id, source, medium, ts, end string) string {
	return "(DATE '2026-10-01','" + client + "','" + id + "','" + source + "','" + medium + "','camp-" + id + "','/p/1',TIMESTAMP '" + ts + "',TIMESTAMP '" + end + "')"
}

func attributedSource(t *testing.T, w *Writer, eventID string) (source, medium string, n int) {
	t.Helper()
	if err := w.DB().QueryRow("SELECT COUNT(*) FROM tracking_events_attributed WHERE event_id = ?", eventID).Scan(&n); err != nil {
		t.Fatalf("query view: %v", err)
	}
	if n == 0 {
		return
	}
	if err := w.DB().QueryRow("SELECT source, medium FROM tracking_events_attributed WHERE event_id = ?", eventID).Scan(&source, &medium); err != nil {
		t.Fatal(err)
	}
	return
}

// Scenario: An event with a matching client id is attributed
func TestAttributedView_AnEventWithAMatchingClientIdIsAttributed(t *testing.T) {
	w := attrFixture(t,
		[]attrEvent{{"e1", "111.222", "2026-10-01 10:05:00"}},
		[]string{sess("111.222", "s1", "google", "cpc", "2026-10-01 10:00:00", "2026-10-01 10:15:00")})
	src, med, n := attributedSource(t, w, "e1")
	if n != 1 || src != "google" || med != "cpc" {
		t.Fatalf("rows=%d source=%q medium=%q, want 1 google cpc", n, src, med)
	}
	var camp, landing string
	if err := w.DB().QueryRow("SELECT campaign, landing_page FROM tracking_events_attributed WHERE event_id='e1'").Scan(&camp, &landing); err != nil || camp != "camp-s1" || landing != "/p/1" {
		t.Errorf("campaign=%q landing=%q err=%v", camp, landing, err)
	}
}

// Scenario: An event with a matching client id is attributed (view = every resolved column + source, medium, campaign, landing_page)
func TestAttributedView_ExposesResolvedColumnsPlusAttribution(t *testing.T) {
	w := attrFixture(t, nil, nil)
	rn, _ := tableColumns(t, w, "tracking_events_resolved")
	an, _ := tableColumns(t, w, "tracking_events_attributed")
	want := append(append([]string{}, rn...), "source", "medium", "campaign", "landing_page")
	if strings.Join(an, ",") != strings.Join(want, ",") {
		t.Fatalf("columns\n got %v\nwant %v", an, want)
	}
}

// Scenario: An event without a client id is kept as unattributed
func TestAttributedView_AnEventWithoutAClientIdIsKeptAsUnattributed(t *testing.T) {
	w := attrFixture(t,
		[]attrEvent{{"e-empty", "", "2026-10-01 10:05:00"}},
		// an empty-id session must not capture empty-id events
		[]string{sess("", "s0", "google", "cpc", "2026-10-01 10:00:00", "2026-10-01 10:15:00")})
	src, _, n := attributedSource(t, w, "e-empty")
	if n != 1 || src != "(none)" {
		t.Fatalf("rows=%d source=%q, want 1 (none)", n, src)
	}
	// NULL ga_client_id too
	if _, err := w.DB().Exec("UPDATE tracking_events SET ga_client_id = NULL WHERE event_id = 'e-empty'"); err != nil {
		t.Fatal(err)
	}
	if src, _, n := attributedSource(t, w, "e-empty"); n != 1 || src != "(none)" {
		t.Fatalf("null client: rows=%d source=%q, want 1 (none)", n, src)
	}
}

// Scenario: An event without a client id is kept as unattributed (unknown client id)
func TestAttributedView_AnEventWithUnknownClientIdIsUnattributed(t *testing.T) {
	w := attrFixture(t,
		[]attrEvent{{"e1", "999.999", "2026-10-01 10:05:00"}},
		[]string{sess("111.222", "s1", "google", "cpc", "2026-10-01 10:00:00", "2026-10-01 10:15:00")})
	if src, _, n := attributedSource(t, w, "e1"); n != 1 || src != "(none)" {
		t.Fatalf("rows=%d source=%q, want 1 (none)", n, src)
	}
}

// Scenario: A client with two sessions does not duplicate events
func TestAttributedView_AClientWithTwoSessionsDoesNotDuplicateEvents(t *testing.T) {
	var evs []attrEvent
	for i, at := range []string{"10:01:00", "10:05:00", "10:12:00", "10:16:00", "10:19:00"} {
		evs = append(evs, attrEvent{"e" + string(rune('1'+i)), "111.222", "2026-10-01 " + at})
	}
	w := attrFixture(t, evs, []string{
		sess("111.222", "A", "google", "cpc", "2026-10-01 10:00:00", "2026-10-01 10:15:00"),
		sess("111.222", "B", "bing", "organic", "2026-10-01 10:20:00", "2026-10-01 10:30:00"),
	})
	var n int
	if err := w.DB().QueryRow("SELECT COUNT(*) FROM tracking_events_attributed WHERE ga_client_id = '111.222'").Scan(&n); err != nil || n != 5 {
		t.Fatalf("rows = %d err=%v, want 5", n, err)
	}
	var total, base int
	w.DB().QueryRow("SELECT COUNT(*) FROM tracking_events_attributed").Scan(&total)
	w.DB().QueryRow("SELECT COUNT(*) FROM tracking_events_resolved").Scan(&base)
	if total != base {
		t.Errorf("view rows %d != resolved rows %d", total, base)
	}
}

// Scenario: An event outside any session is unattributed
func TestAttributedView_AnEventOutsideAnySessionIsUnattributed(t *testing.T) {
	w := attrFixture(t,
		[]attrEvent{{"late", "111.222", "2026-10-01 12:15:00"}, {"early", "111.222", "2026-10-01 09:59:59"}},
		[]string{sess("111.222", "A", "google", "cpc", "2026-10-01 10:00:00", "2026-10-01 10:15:00")})
	for _, id := range []string{"late", "early"} {
		if src, _, n := attributedSource(t, w, id); n != 1 || src != "(none)" {
			t.Errorf("%s: rows=%d source=%q, want 1 (none)", id, n, src)
		}
	}
}

// Scenario: Session end is bounded by the next session or 30 minutes after ended_at
func TestAttributedView_SessionEndIsBoundedByNextSessionOr30MinutesAfterEndedAt(t *testing.T) {
	w := attrFixture(t,
		[]attrEvent{
			{"at-start", "111.222", "2026-10-01 10:00:00"}, // window start inclusive -> A
			{"e1018", "111.222", "2026-10-01 10:18:00"},    // A (ended_at+30 would also cover, next ts caps at 10:20)
			{"at-next", "111.222", "2026-10-01 10:20:00"},  // next ts exclusive for A -> B
			{"e1025", "111.222", "2026-10-01 10:25:00"},    // B
			{"b-last", "111.222", "2026-10-01 10:59:59"},   // B (ended_at 10:30 + 30 min = 11:00 exclusive)
			{"b-end", "111.222", "2026-10-01 11:00:00"},    // none
			{"e1130", "111.222", "2026-10-01 11:30:00"},    // none
		},
		[]string{
			sess("111.222", "A", "google", "cpc", "2026-10-01 10:00:00", "2026-10-01 10:15:00"),
			sess("111.222", "B", "bing", "organic", "2026-10-01 10:20:00", "2026-10-01 10:30:00"),
		})
	want := map[string]string{"at-start": "google", "e1018": "google", "at-next": "bing", "e1025": "bing", "b-last": "bing", "b-end": "(none)", "e1130": "(none)"}
	for id, src := range want {
		got, _, n := attributedSource(t, w, id)
		if n != 1 || got != src {
			t.Errorf("%s: rows=%d source=%q, want 1 %q", id, n, got, src)
		}
	}
}

// Scenario: Session end is bounded by the next session or 30 minutes after ended_at (next session of a different client does not cap the window)
func TestAttributedView_NextSessionCapIsPerClient(t *testing.T) {
	w := attrFixture(t,
		[]attrEvent{{"e1", "111.222", "2026-10-01 10:25:00"}},
		[]string{
			sess("111.222", "A", "google", "cpc", "2026-10-01 10:00:00", "2026-10-01 10:15:00"),
			sess("333.444", "X", "bing", "organic", "2026-10-01 10:20:00", "2026-10-01 10:30:00"),
		})
	if src, _, n := attributedSource(t, w, "e1"); n != 1 || src != "google" {
		t.Fatalf("rows=%d source=%q, want 1 google (A runs to ended_at+30 = 10:45)", n, src)
	}
}

// Scenario: ga4_sessions stores ended_at
func TestAttributedView_GA4SessionsStoresEndedAt(t *testing.T) {
	w := attrFixture(t, nil, []string{sess("111.222", "A", "google", "cpc", "2026-10-01 10:00:00", "2026-10-01 10:15:00")})
	var ts, end time.Time
	if err := w.DB().QueryRow("SELECT ts, ended_at FROM ga4_sessions WHERE session_id='A'").Scan(&ts, &end); err != nil {
		t.Fatal(err)
	}
	if ts.UTC().Format("15:04") != "10:00" || end.UTC().Format("15:04") != "10:15" {
		t.Errorf("ts=%v ended_at=%v", ts, end)
	}
}

// Scenario: A missing import adds a reason only (empty ga4_sessions: no event is dropped, all unattributed)
func TestAttributedView_AMissingImportKeepsEveryEventUnattributed(t *testing.T) {
	w := attrFixture(t,
		[]attrEvent{{"e1", "111.222", "2026-10-01 10:05:00"}, {"e2", "", "2026-10-01 10:06:00"}}, nil)
	var n, none int
	w.DB().QueryRow("SELECT COUNT(*), COUNT(*) FILTER (WHERE source = '(none)') FROM tracking_events_attributed").Scan(&n, &none)
	if n != 2 || none != 2 {
		t.Fatalf("rows=%d none=%d, want 2 and 2", n, none)
	}
}

// Scenario: View parity (DuckDB view definition uses the same structural elements as the BigQuery DDL)
func TestAttributedView_DefinitionStructure(t *testing.T) {
	w := attrFixture(t, nil, nil)
	var sql string
	if err := w.DB().QueryRow("SELECT sql FROM duckdb_views() WHERE view_name = 'tracking_events_attributed'").Scan(&sql); err != nil {
		t.Fatalf("view missing: %v", err)
	}
	u := strings.ToUpper(sql)
	for _, tok := range []string{"TRACKING_EVENTS_RESOLVED", "GA4_SESSIONS", "GA_CLIENT_ID", "USER_PSEUDO_ID", "LEFT JOIN", "LEAD(", "ENDED_AT", "(NONE)"} {
		if !strings.Contains(u, tok) {
			t.Errorf("view SQL lacks %s", tok)
		}
	}
}
