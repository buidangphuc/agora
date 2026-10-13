package config_test

import (
	"os"
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-payment/internal/config"
)

// Every key the service reads is documented in .env.example.
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
	for _, k := range sdkconfig.DeclaredEnvKeys(config.Settings{}) {
		if !documented[k] {
			t.Errorf(".env.example does not document %s", k)
		}
	}
}

func TestKafkaConfigDefaults(t *testing.T) {
	for _, k := range sdkconfig.DeclaredEnvKeys(config.Settings{}) {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	t.Setenv("ENV", "local")
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
	t.Setenv("GRPC_PORT", "50056")
	s, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Events.PaymentTopic != "payment.events" || s.Events.OrderTopic != "order.events" {
		t.Errorf("topics: %+v", s.Events)
	}
	if s.Events.SettlementGroup != "team-payment.settlement" || s.Events.SettlementDLQTopic != "order.events.payment-settlement.dlq" {
		t.Errorf("consumer defaults: %+v", s.Events)
	}
}

// TestEnvExampleInSync is the env-drift gate (`make check-env`): .env.example documents
// exactly the keys Settings declares.
func TestEnvExampleInSync(t *testing.T) {
	if err := sdkconfig.CheckEnvExample("../../.env.example", config.Settings{}); err != nil {
		t.Fatal(err)
	}
}
