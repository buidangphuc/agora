package config

import (
	"os"
	"testing"
)

func TestEnvExampleInSync(t *testing.T) {
	const path = "../../.env.example"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".env.example not found at %s: %v", path, err)
	}
	if err := CheckEnvExample(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.Server.Port != 50054 {
		t.Errorf("default GRPC_PORT = %d, want 50054", s.Server.Port)
	}
}

func TestKafkaDefaultsAndValidation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.Kafka.Enabled || s.Kafka.ListingTopic != "listing.events" || s.Kafka.ConsumerGroup != "team-engagement-feed" {
		t.Errorf("unexpected kafka defaults: %+v", s.Kafka)
	}
	t.Setenv("KAFKA_ENABLED", "true")
	t.Setenv("KAFKA_BROKERS", "a:9092, b:9092")
	s, err = LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings enabled: %v", err)
	}
	if got := s.KafkaBrokers(); len(got) != 2 || got[1] != "b:9092" {
		t.Errorf("brokers = %v", got)
	}
	t.Setenv("KAFKA_BROKERS", " ")
	if _, err := LoadSettings(); err == nil {
		t.Error("enabled with no brokers must fail validation")
	}
}
