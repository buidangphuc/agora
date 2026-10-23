package payment_test

import (
	"os"
	"strings"
	"testing"
)

// Task 5 (docs, no scenario): README documents the CANCELLED payment status and the
// unpaid-cancel consumer behaviour of payment-unpaid-cancel.
func TestReadmeDocumentsUnpaidCancel_task5(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(b)
	for _, want := range []string{
		"CANCELLED",         // status list gains CANCELLED
		"cancel:<order_id>", // late-settle refund key (already present for paid cancel)
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README.md must mention %q", want)
		}
	}
	// the status list in the migrations table must list CANCELLED next to the other statuses
	if !strings.Contains(readme, "REFUNDED, 5 PARTIALLY_REFUNDED") || !strings.Contains(readme, "6 CANCELLED") {
		t.Errorf("README.md status list must include `6 CANCELLED`")
	}
	lower := strings.ToLower(readme)
	if !strings.Contains(lower, "unavailable") || !strings.Contains(lower, "failedprecondition") {
		t.Errorf("README.md must document ProcessMockPayment refusal on CANCELLED and Unavailable when team-order is unreachable")
	}
	if !strings.Contains(lower, "void") {
		t.Errorf("README.md must describe voiding PENDING/FAILED transactions of a cancelled unpaid order")
	}
}
