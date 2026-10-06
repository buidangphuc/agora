package duckdb

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

func count(t *testing.T, w *Writer, table string) int {
	t.Helper()
	var n int
	if err := w.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWriteIsIdempotentOnEventID(t *testing.T) {
	ctx := context.Background()
	w, err := Open(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	batch := []*warehouse.TrackingRecord{
		{EventID: "e1", EventType: "view", ListingID: "l1", OccurredAt: at, Properties: map[string]string{"k": "v"}},
		{EventID: "e2", EventType: "view", ListingID: "l2", OccurredAt: at},
		{EventID: "e2", EventType: "view", ListingID: "l2", OccurredAt: at}, // dup inside the batch
	}
	for i := 0; i < 3; i++ { // redelivery of the same batch
		if err := w.Write(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	if got := count(t, w, warehouse.TableName); got != 2 {
		t.Fatalf("tracking rows = %d, want 2", got)
	}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{{EventID: "e3", EventType: "view", OccurredAt: at}}); err != nil {
		t.Fatal(err)
	}
	if got := count(t, w, warehouse.TableName); got != 3 {
		t.Fatalf("after new event rows = %d, want 3", got)
	}
}

func TestWriteOrderFactsIsIdempotentOnEventID(t *testing.T) {
	ctx := context.Background()
	w, err := Open(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	batch := []*warehouse.OrderFactRecord{
		{EventID: "evt-0", OrderID: "o1", ListingID: "l1", SellerID: "s1", Quantity: 1, UnitPrice: 10, Currency: "VND", OccurredAt: at, Status: "PAID"},
		{EventID: "evt-1", OrderID: "o1", ListingID: "l2", SellerID: "s1", Quantity: 2, UnitPrice: 5, Currency: "VND", OccurredAt: at, Status: "PAID"},
	}
	for i := 0; i < 3; i++ {
		if err := w.WriteOrderFacts(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	if got := count(t, w, warehouse.OrderFactsTableName); got != 2 {
		t.Fatalf("order_facts rows = %d, want 2", got)
	}
}

// Existing volumes may already contain duplicates; Open and further writes must
// still work (no unique index) and not add more.
func TestWriteOnDatabaseWithPreexistingDuplicates(t *testing.T) {
	ctx := context.Background()
	w, err := Open(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for i := 0; i < 2; i++ {
		if _, err := w.DB().Exec("INSERT INTO tracking_events (event_id, event_type, occurred_at) VALUES ('dup','view', now())"); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{{EventID: "dup", EventType: "view", OccurredAt: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	if got := count(t, w, warehouse.TableName); got != 2 {
		t.Fatalf("rows = %d, want 2 (no new dup)", got)
	}
}
