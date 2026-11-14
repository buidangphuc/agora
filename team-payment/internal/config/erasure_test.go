package config_test

import (
	"testing"

	"github.com/buidangphuc/team-payment/internal/config"
)

func TestErasureConsumerDefaults(t *testing.T) {
	// PER-3, PER-5: own consumer group and DLQ for the UserErased consumer
	localEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Events.ErasureGroup != "team-payment.erasure" {
		t.Errorf("Events.ErasureGroup = %q, want team-payment.erasure", s.Events.ErasureGroup)
	}
	if s.Events.ErasureDLQ != "identity.events.payment.dlq" {
		t.Errorf("Events.ErasureDLQ = %q, want identity.events.payment.dlq", s.Events.ErasureDLQ)
	}
}

func TestErasureConsumerEnvOverrides(t *testing.T) {
	// PER-3, PER-5
	localEnv(t)
	t.Setenv("PAYMENT_ERASURE_CONSUMER_GROUP", "g-x")
	t.Setenv("PAYMENT_ERASURE_DLQ_TOPIC", "dlq-x")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Events.ErasureGroup != "g-x" || s.Events.ErasureDLQ != "dlq-x" {
		t.Errorf("Events erasure = %q / %q, want g-x / dlq-x", s.Events.ErasureGroup, s.Events.ErasureDLQ)
	}
}
