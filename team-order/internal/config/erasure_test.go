package config_test

import "testing"

func TestErasureConsumerDefaults(t *testing.T) {
	// ORD-3, ORD-6: own consumer group and DLQ for the UserErased consumer
	localEnv(t)
	s := mustLoad(t)
	if got := str(t, s, "Events", "ErasureGroup"); got != "team-order.erasure" {
		t.Errorf("Events.ErasureGroup = %q, want team-order.erasure", got)
	}
	if got := str(t, s, "Events", "ErasureDLQ"); got != "identity.events.order.dlq" {
		t.Errorf("Events.ErasureDLQ = %q, want identity.events.order.dlq", got)
	}
}

func TestErasureConsumerEnvOverrides(t *testing.T) {
	// ORD-3, ORD-6
	localEnv(t)
	t.Setenv("ORDER_ERASURE_CONSUMER_GROUP", "g-x")
	t.Setenv("ORDER_ERASURE_DLQ_TOPIC", "dlq-x")
	s := mustLoad(t)
	if got := str(t, s, "Events", "ErasureGroup"); got != "g-x" {
		t.Errorf("Events.ErasureGroup = %q, want g-x", got)
	}
	if got := str(t, s, "Events", "ErasureDLQ"); got != "dlq-x" {
		t.Errorf("Events.ErasureDLQ = %q, want dlq-x", got)
	}
}
