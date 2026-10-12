package config_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-notification/internal/config"
)

func TestNoEnableFlagsDeclared(t *testing.T) {
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	for _, k := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "GRPC_REFLECTION_ENABLED"} {
		if slices.Contains(keys, k) {
			t.Errorf("Settings still declares %s", k)
		}
	}
}

func TestKafkaBrokersIsTheOnlyBrokerSetting(t *testing.T) {
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	if slices.Contains(keys, "KAFKA_BROKER") {
		t.Error("Settings still declares the stray KAFKA_BROKER")
	}
	if !slices.Contains(keys, "KAFKA_BROKERS") {
		t.Error("Settings does not declare KAFKA_BROKERS")
	}
}

func TestEnvExampleInSync(t *testing.T) {
	if err := sdkconfig.CheckEnvExample("../../.env.example", config.Settings{}); err != nil {
		t.Fatal(err)
	}
}

func TestConfigLoad(t *testing.T) {
	t.Setenv("GRPC_PORT", "50099")
	t.Setenv("DATABASE_URL", "postgres://test:pass@localhost:5432/db")
	t.Setenv("KAFKA_BROKERS", "localhost:9092")

	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Port != 50099 {
		t.Errorf("expected 50099, got %d", s.Port)
	}
	if s.Postgres.URL != "postgres://test:pass@localhost:5432/db" {
		t.Errorf("unexpected database URL")
	}
	if s.Kafka.Brokers != "localhost:9092" {
		t.Errorf("unexpected kafka broker")
	}
}

func TestConfigLoadRequiresDatabaseURLAndPort(t *testing.T) {
	t.Setenv("GRPC_PORT", "50099")
	t.Setenv("DATABASE_URL", "")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
	}

	t.Setenv("GRPC_PORT", "")
	t.Setenv("DATABASE_URL", "postgres://test:pass@localhost:5432/db")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "GRPC_PORT") {
		t.Fatalf("err = %v, want an error naming GRPC_PORT", err)
	}
}

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GRPC_PORT", "50099")
	t.Setenv("DATABASE_URL", "postgres://test:pass@localhost:5432/db")
}

func TestUpstreamAddrs(t *testing.T) {
	baseEnv(t)
	t.Setenv("UPSTREAM_DOMAIN_ADDR", "")
	t.Setenv("UPSTREAM_IDENTITY_ADDR", "")
	s, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Upstream.DomainAddr != "localhost:50051" || s.Upstream.IdentityAddr != "localhost:50053" {
		t.Errorf("defaults = %q / %q", s.Upstream.DomainAddr, s.Upstream.IdentityAddr)
	}
	t.Setenv("UPSTREAM_DOMAIN_ADDR", "team-domain-svc:50051")
	t.Setenv("UPSTREAM_IDENTITY_ADDR", "team-identity-svc:50053")
	s, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Upstream.DomainAddr != "team-domain-svc:50051" || s.Upstream.IdentityAddr != "team-identity-svc:50053" {
		t.Errorf("overrides = %q / %q", s.Upstream.DomainAddr, s.Upstream.IdentityAddr)
	}
}

func TestConsumerConfigDefaults(t *testing.T) {
	baseEnv(t)
	for _, k := range []string{
		"NOTIFICATION_LISTING_CONSUMER_GROUP", "LISTING_EVENTS_TOPIC", "LISTING_EVENTS_DLQ_TOPIC",
		"NOTIFICATION_CHAT_CONSUMER_GROUP", "CHAT_EVENTS_TOPIC", "CHAT_EVENTS_DLQ_TOPIC",
		"NOTIFICATION_ORDER_CONSUMER_GROUP", "ORDER_EVENTS_TOPIC", "ORDER_EVENTS_DLQ_TOPIC",
	} {
		t.Setenv(k, "")
	}
	s, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	listing, chat, order := s.Listing, s.Chat, s.Order
	if listing.Topic != "listing.events" || listing.DLQTopic != "listing.events.dlq" || listing.Group != "team-notification.listing" {
		t.Errorf("listing defaults = %+v", listing)
	}
	if chat.Topic != "chat.events" || chat.DLQTopic != "chat.events.dlq" || chat.Group != "team-notification.chat" {
		t.Errorf("chat defaults = %+v", chat)
	}
	if order.Topic != "order.events" || order.DLQTopic != "order.events.dlq" || order.Group != "team-notification.order" {
		t.Errorf("order defaults = %+v", order)
	}
	if chat.Group == order.Group || chat.Group == listing.Group {
		t.Errorf("each topic consumer needs its own consumer group")
	}
}

// Every env var the service reads must be documented in .env.example, so a new
// knob cannot ship undocumented.
func TestEnvExampleListsConfigVars(t *testing.T) {
	raw, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	for _, k := range []string{
		"GRPC_PORT", "DATABASE_URL", "UPSTREAM_DOMAIN_ADDR", "UPSTREAM_IDENTITY_ADDR",
		"KAFKA_BROKERS",
		"LISTING_EVENTS_TOPIC", "LISTING_EVENTS_DLQ_TOPIC", "NOTIFICATION_LISTING_CONSUMER_GROUP",
		"CHAT_EVENTS_TOPIC", "CHAT_EVENTS_DLQ_TOPIC", "NOTIFICATION_CHAT_CONSUMER_GROUP",
		"ORDER_EVENTS_TOPIC", "ORDER_EVENTS_DLQ_TOPIC", "NOTIFICATION_ORDER_CONSUMER_GROUP",
	} {
		if !strings.Contains(string(raw), k+"=") {
			t.Errorf(".env.example is missing %s", k)
		}
	}
}
