package config

import (
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

func TestEnvExampleInSync(t *testing.T) {
	if err := sdkconfig.CheckEnvExample("../../.env.example", Settings{}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("GRPC_PORT", "50054")
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Port != 50054 {
		t.Errorf("GRPC_PORT = %d, want 50054", s.Port)
	}
}

// GRPC_PORT has no default (ADR-0016): a service that forgets it fails at load.
func TestLoadRequiresGRPCPort(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("GRPC_PORT", "")
	if _, err := Load(); err == nil {
		t.Error("an empty GRPC_PORT must fail validation")
	}
}

func TestKafkaDefaultsAndValidation(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("GRPC_PORT", "50054")
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Events.ListingTopic != "listing.events" || s.Events.ConsumerGroup != "team-engagement-feed" {
		t.Errorf("unexpected kafka defaults: %+v", s.Events)
	}
	t.Setenv("KAFKA_BROKERS", "a:9092, b:9092")
	s, err = Load()
	if err != nil {
		t.Fatalf("Load with brokers: %v", err)
	}
	if got := s.Kafka.BrokerList(); len(got) != 2 || got[1] != "b:9092" {
		t.Errorf("brokers = %v", got)
	}
	t.Setenv("KAFKA_BROKERS", " ")
	if _, err := Load(); err == nil {
		t.Error("no brokers must fail validation")
	}
}

func TestEngagementEventsDefaultsAndValidation(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("GRPC_PORT", "50054")
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Events.EventsTopic != "engagement.events" || s.Outbox.PollInterval != time.Second {
		t.Errorf("defaults: %+v %+v", s.Events, s.Outbox)
	}
	t.Setenv("OUTBOX_POLL_INTERVAL", "2s")
	if s, err = Load(); err != nil || s.Outbox.PollInterval != 2*time.Second {
		t.Errorf("interval: %v %v", s, err)
	}
	t.Setenv("OUTBOX_POLL_INTERVAL", "0s")
	if _, err := Load(); err == nil {
		t.Error("zero interval must fail")
	}
}

// ENV has no default (go-sdk v1): a deployment that forgets it fails at load
// instead of running as local. A deployed ENV needs KAFKA_BROKERS spelled out.
func TestEnvIsRequiredAndDeployedEnvNeedsBrokers(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("GRPC_PORT", "50054")
	t.Setenv("ENV", "")
	if _, err := Load(); err == nil {
		t.Error("an empty ENV must fail validation")
	}
	t.Setenv("ENV", "staging")
	if _, err := Load(); err == nil {
		t.Error("ENV=staging without KAFKA_BROKERS must fail")
	}
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	if _, err := Load(); err != nil {
		t.Errorf("ENV=staging with brokers: %v", err)
	}
}
