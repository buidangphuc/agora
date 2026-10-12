package bigquery

// Seam the implementer must add (mirrors GA4ViewDDL):
//
//	func AttributedViewDDL(project, dataset string) string // CREATE OR REPLACE VIEW tracking_events_attributed
//
// Writer.Open must execute it after ga4_sessions is migrated (not unit-testable without BigQuery).

import (
	"strings"
	"testing"
)

// Scenario: An event with a matching client id is attributed (BigQuery DDL joins on ga_client_id = user_pseudo_id)
// Scenario: A client with two sessions does not duplicate events (single LEFT JOIN on a windowed session set)
// Scenario: Session end is bounded by the next session or 30 minutes after ended_at
// Scenario: An event without a client id is kept as unattributed
// Parity: structurally matches the DuckDB view (same tokens as duckdb TestAttributedView_DefinitionStructure).
func TestAttributedViewDDL_StructurallyMatchesDuckDBView(t *testing.T) {
	ddl := AttributedViewDDL("p", "d")
	u := strings.ToUpper(ddl)
	if !strings.HasPrefix(u, "CREATE OR REPLACE VIEW `P.D.TRACKING_EVENTS_ATTRIBUTED`") {
		t.Errorf("not a CREATE OR REPLACE VIEW of tracking_events_attributed: %s", ddl)
	}
	for _, tok := range []string{"`p.d.tracking_events_resolved`", "`p.d.ga4_sessions`", "ga_client_id", "user_pseudo_id", "ended_at", "'(none)'"} {
		if !strings.Contains(ddl, tok) {
			t.Errorf("DDL lacks %s", tok)
		}
	}
	for _, tok := range []string{"LEFT JOIN", "LEAD(", "INTERVAL 30 MINUTE"} {
		if !strings.Contains(u, tok) {
			t.Errorf("DDL lacks %s", tok)
		}
	}
	for _, c := range []string{"source", "medium", "campaign", "landing_page"} {
		if !strings.Contains(ddl, c) {
			t.Errorf("DDL does not project %s", c)
		}
	}
}
