package bootstrap

import (
	"os"
	"strings"
	"testing"

	"github.com/buidangphuc/team-payment/internal/config"
)

// Every key the service reads, from config.Settings and from this package's Kafka
// wiring, is documented in .env.example.
func TestEnvExampleDocumentsEveryKey(t *testing.T) {
	raw, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		if k, _, ok := strings.Cut(line, "="); ok && !strings.Contains(k, " ") {
			documented[strings.TrimSpace(k)] = true
		}
	}
	for _, k := range append(config.DeclaredEnvKeys(), KafkaEnvKeys()...) {
		if !documented[k] {
			t.Errorf(".env.example does not document %s", k)
		}
	}
}

func TestKafkaConfigDefaults(t *testing.T) {
	for _, k := range KafkaEnvKeys() {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	c := kafkaConfigFromEnv()
	if c.Enabled {
		t.Error("KAFKA_ENABLED must default to false (neither relayer nor consumer starts)")
	}
	if c.Topic != "payment.events" || c.OrderEventsTopic != "order.events" {
		t.Errorf("topics: %+v", c)
	}
	if !c.ConsumerEnabled || c.ConsumerGroup != "team-payment.settlement" || c.DLQTopic != "order.events.payment-settlement.dlq" {
		t.Errorf("consumer defaults: %+v", c)
	}
}
