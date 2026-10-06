package bootstrap

import (
	"os"
	"strings"
	"testing"
)

func TestConsumerConfigDefaults(t *testing.T) {
	for _, k := range []string{
		"NOTIFICATION_CHAT_CONSUMER_GROUP", "CHAT_EVENTS_TOPIC", "CHAT_EVENTS_DLQ_TOPIC",
		"NOTIFICATION_ORDER_CONSUMER_GROUP", "ORDER_EVENTS_TOPIC", "ORDER_EVENTS_DLQ_TOPIC",
	} {
		t.Setenv(k, "")
	}
	chat := ChatKafkaConfigFromEnv()
	if chat.Topic != "chat.events" || chat.DLQTopic != "chat.events.dlq" || chat.ConsumerGroup != "team-notification.chat" {
		t.Errorf("chat defaults = %+v", chat)
	}
	order := OrderKafkaConfigFromEnv()
	if order.Topic != "order.events" || order.DLQTopic != "order.events.dlq" || order.ConsumerGroup != "team-notification.order" {
		t.Errorf("order defaults = %+v", order)
	}
	if chat.ConsumerGroup == order.ConsumerGroup || chat.ConsumerGroup == KafkaConfigFromEnv().ConsumerGroup {
		t.Errorf("each topic consumer needs its own consumer group")
	}
}

// Every env var the Kafka consumers read must be documented in .env.example, so a
// new knob cannot ship undocumented.
func TestEnvExampleListsKafkaVars(t *testing.T) {
	raw, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	for _, k := range []string{
		"KAFKA_ENABLED", "KAFKA_BROKERS",
		"LISTING_EVENTS_TOPIC", "LISTING_EVENTS_DLQ_TOPIC", "NOTIFICATION_LISTING_CONSUMER_GROUP",
		"CHAT_EVENTS_TOPIC", "CHAT_EVENTS_DLQ_TOPIC", "NOTIFICATION_CHAT_CONSUMER_GROUP",
		"ORDER_EVENTS_TOPIC", "ORDER_EVENTS_DLQ_TOPIC", "NOTIFICATION_ORDER_CONSUMER_GROUP",
	} {
		if !strings.Contains(string(raw), k+"=") {
			t.Errorf(".env.example is missing %s", k)
		}
	}
}
