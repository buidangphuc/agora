package bigquery

import (
	"testing"

	"cloud.google.com/go/bigquery"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// An order_facts table created before buyer_id existed must gain exactly that column.
func TestWithMissingColumnsAddsBuyerIDToOldOrderFacts(t *testing.T) {
	want, err := bqOrderFactsSchema()
	if err != nil {
		t.Fatal(err)
	}
	var old bigquery.Schema
	for _, f := range want {
		if f.Name != "buyer_id" {
			old = append(old, f)
		}
	}
	got := withMissingColumns(old, want)
	if len(got) != len(old)+1 || got[len(got)-1].Name != "buyer_id" {
		t.Fatalf("got %d columns, last %q; want old+buyer_id", len(got), got[len(got)-1].Name)
	}
	if again := withMissingColumns(got, want); len(again) != len(got) {
		t.Fatalf("second evolve changed the schema: %d -> %d", len(got), len(again))
	}
}

func TestOrderFactRowSaverOmitsEmptyBuyer(t *testing.T) {
	row, _, _ := (&orderFactRowSaver{rec: &warehouse.OrderFactRecord{EventID: "e"}}).Save()
	if _, ok := row["buyer_id"]; ok {
		t.Fatal("empty buyer must be omitted (NULL)")
	}
	row, _, _ = (&orderFactRowSaver{rec: &warehouse.OrderFactRecord{EventID: "e", BuyerID: "u1"}}).Save()
	if row["buyer_id"] != "u1" {
		t.Fatalf("buyer_id = %v", row["buyer_id"])
	}
}

// Scenario: Consent and client id are persisted (BigQuery row carries the consent columns)
func TestConsentAndClientIdArePersisted_BigQueryRowSaver(t *testing.T) {
	row, _, err := (&rowSaver{rec: &warehouse.TrackingRecord{
		EventID: "e", GaClientID: "111.222", ConsentVersion: 1,
		ConsentAnalytics: true, ConsentMarketing: false, ConsentPersonalization: true,
	}}).Save()
	if err != nil {
		t.Fatal(err)
	}
	if row["ga_client_id"] != "111.222" || row["consent_analytics"] != true || row["consent_marketing"] != false || row["consent_personalization"] != true {
		t.Errorf("row = %v", row)
	}
	if v, ok := row["consent_version"]; !ok || v != int64(1) {
		t.Errorf("consent_version = %v (%T), want int64(1)", v, v)
	}
}
