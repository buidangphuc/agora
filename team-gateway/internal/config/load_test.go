package config_test

import (
	"strings"
	"testing"

	"github.com/buidangphuc/team-gateway/internal/config"
)

// TestReflectionRejectedInProd: Load refuses EDGE_REFLECTION_ENABLED=true when
// ENV is production, and names the variable.
func TestReflectionRejectedInProd(t *testing.T) {
	t.Setenv("JWKS_URL", "http://identity/jwks")
	t.Setenv("ENV", "production")
	t.Setenv("EDGE_REFLECTION_ENABLED", "true")

	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "EDGE_REFLECTION_ENABLED") {
		t.Fatalf("Load = %v, want an error naming EDGE_REFLECTION_ENABLED", err)
	}

	t.Setenv("ENV", "local")
	if _, err := config.Load(); err != nil {
		t.Fatalf("ENV=local must allow reflection: %v", err)
	}
}

// Every deploy runs Kafka, so there is no KAFKA_ENABLED flag. A leftover
// KAFKA_ENABLED=false in an old environment is ignored, and the brokers still
// come from KAFKA_BROKERS.
func TestKafkaHasNoEnableFlag(t *testing.T) {
	t.Setenv("JWKS_URL", "http://identity/jwks")
	t.Setenv("KAFKA_ENABLED", "false")
	t.Setenv("KAFKA_BROKERS", "k1:9092, k2:9092")

	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := strings.Join(s.BrokerList(), ","); got != "k1:9092,k2:9092" {
		t.Errorf("BrokerList = %q", got)
	}
	if s.Events.AnalyticsTopic != "analytics.events" || s.Identity.Topic != "identity.events" {
		t.Errorf("topics = %q, %q", s.Events.AnalyticsTopic, s.Identity.Topic)
	}
}
