package consumer

import (
	"os"
	"strings"
	"testing"
)

// Task 5 (notification-order-work): README documents the new order-work event
// types and the LOW_STOCK_THRESHOLD setting. No scenario ids assigned.
func TestTask5ReadmeDocumentsOrderWorkEventsAndLowStockThreshold(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(b)
	for _, want := range []string{
		"LOW_STOCK_THRESHOLD", "OrderPlaced", "OrderPaidEvent", "OrderPaymentReminder",
		"OrderCancelled", "OrderCompleted", "ReturnRequested", "ReturnRefunded",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README.md does not mention %s", want)
		}
	}
}
