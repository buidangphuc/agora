package docs

import (
	"os"
	"strings"
	"testing"
)

// Scenario: .env.example documents exactly the declared keys (the README config table follows)
func TestReadmeDoesNotDocumentRemovedKeys(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, gone := range []string{"KAFKA_ENABLED", "SHUTDOWN_GRACE_SECONDS", "GA4_EXPORT_SOURCE", "GA4_RELAY_PROBE_PORT"} {
		if strings.Contains(text, gone) {
			t.Errorf("README still mentions removed key %s", gone)
		}
	}
}

// Scenario: .env.example documents exactly the declared keys (new keys appear in the README)
func TestReadmeDocumentsNewKeys(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{
		"SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY", "LOG_KEY_FP_SECRET", "MOCK_DATA",
		"KAFKA_DLQ_TOPIC", "KAFKA_LISTING_DLQ_TOPIC", "GA4_IMPORT_TRIGGER_URL", "OTEL_SERVICE_NAME",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("README does not document %s", want)
		}
	}
}
