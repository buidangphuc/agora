package bootstrap_test

import (
	"strings"
	"testing"

	"github.com/buidangphuc/team-order/internal/bootstrap"
)

func TestDisputeConsumerSkippedWithoutAudit(t *testing.T) {
	start, reason := bootstrap.DisputeConsumerStartDecision(true, false)
	if start {
		t.Fatal("dispute consumer must not start without an audit client")
	}
	low := strings.ToLower(reason)
	if !strings.Contains(low, "audit") {
		t.Fatalf("skip reason must name the audit client, got %q", reason)
	}
}

func TestDisputeConsumerSkippedWhenKafkaDisabled(t *testing.T) {
	for _, audit := range []bool{true, false} {
		start, reason := bootstrap.DisputeConsumerStartDecision(false, audit)
		if start {
			t.Fatalf("dispute consumer must not start with kafka disabled (audit=%v)", audit)
		}
		if !strings.Contains(strings.ToLower(reason), "kafka") {
			t.Fatalf("skip reason must name kafka, got %q", reason)
		}
	}
}

func TestDisputeConsumerStartsWithKafkaAndAudit(t *testing.T) {
	start, reason := bootstrap.DisputeConsumerStartDecision(true, true)
	if !start {
		t.Fatalf("dispute consumer must start with kafka and audit, reason %q", reason)
	}
	if reason != "" {
		t.Fatalf("no skip reason expected when starting, got %q", reason)
	}
}
