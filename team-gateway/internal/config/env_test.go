package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/buidangphuc/team-gateway/internal/config"
)

// ENV has no default: an unset ENV must fail Load by name, not run as local.
func TestLoadRequiresEnv(t *testing.T) {
	t.Setenv("JWKS_URL", "http://identity/jwks")
	prev, had := os.LookupEnv("ENV")
	if err := os.Unsetenv("ENV"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("ENV", prev)
		}
	})

	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "ENV") {
		t.Fatalf("Load without ENV = %v, want an error naming ENV", err)
	}
}

// In a strict ENV KAFKA_BROKERS has no fallback; a local ENV keeps localhost:9092.
func TestKafkaBrokersRequiredInStrictEnv(t *testing.T) {
	t.Setenv("JWKS_URL", "http://identity/jwks")
	t.Setenv("KAFKA_BROKERS", "")

	for _, env := range []string{"staging", "stage", "prod", "production"} {
		t.Setenv("ENV", env)
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "KAFKA_BROKERS") {
			t.Errorf("ENV=%s: Load = %v, want an error naming KAFKA_BROKERS", env, err)
		}
	}

	t.Setenv("ENV", "local")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("ENV=local: %v", err)
	}
	if got := strings.Join(s.BrokerList(), ","); got != "localhost:9092" {
		t.Errorf("ENV=local brokers = %q, want localhost:9092", got)
	}
}
