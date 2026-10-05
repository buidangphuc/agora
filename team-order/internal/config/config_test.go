package config_test

import (
	"os"
	"testing"

	"github.com/buidangphuc/team-order/internal/config"
)

func TestConfigLoadSettings(t *testing.T) {
	os.Setenv("ENV", "test")
	os.Setenv("GRPC_PORT", "50055")
	os.Setenv("DATABASE_ENABLED", "false")

	cfg, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Runtime.Env != "test" {
		t.Errorf("expected Env test, got %s", cfg.Runtime.Env)
	}
	if cfg.Server.Port != 50055 {
		t.Errorf("expected Port 50055, got %d", cfg.Server.Port)
	}
}

// TestEnvExampleInSync is the env-drift gate: .env.example must document exactly
// the keys Settings declares (both directions).
func TestEnvExampleInSync(t *testing.T) {
	if err := config.CheckEnvExample("../../.env.example"); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxAndKafkaDefaults(t *testing.T) {
	for _, k := range []string{"KAFKA_ENABLED", "OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "ORDER_EVENTS_TOPIC"} {
		os.Unsetenv(k)
	}
	os.Setenv("DATABASE_ENABLED", "false")
	cfg, err := config.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kafka.Enabled || cfg.Kafka.OrderTopic != "order.events" {
		t.Errorf("kafka defaults wrong: %+v", cfg.Kafka)
	}
	if cfg.OutboxPollInterval().String() != "1s" || cfg.Outbox.BatchSize != 100 || !cfg.Outbox.Enabled {
		t.Errorf("outbox defaults wrong: %+v", cfg.Outbox)
	}
}
