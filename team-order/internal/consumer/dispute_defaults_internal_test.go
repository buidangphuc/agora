package consumer

import (
	"testing"
	"time"
)

// Dispute refund: Audit outage longer than the retry window is not dead-lettered
func TestDisputeRunConfigDefaultMaxBackoffIsFiveSeconds(t *testing.T) {
	got := RunConfig{RetryTransientUntilApplied: true}.withDefaults().MaxBackoff
	if got != 5*time.Second {
		t.Fatalf("default MaxBackoff = %v, want 5s", got)
	}
	if d := retryDelay(RunConfig{RetryTransientUntilApplied: true}.withDefaults(), 1000); d != 5*time.Second {
		t.Fatalf("capped retry delay = %v, want 5s", d)
	}
}
