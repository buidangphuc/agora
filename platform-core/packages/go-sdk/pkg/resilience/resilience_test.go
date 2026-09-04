package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/resilience"
)

func TestCircuitBreaker(t *testing.T) {
	cb := resilience.NewCircuitBreaker(2, 50*time.Millisecond)

	customErr := errors.New("temporary failure")

	// Call 1 fails
	_ = cb.Execute(func() error { return customErr })
	// Call 2 fails -> Trips circuit breaker to OPEN
	_ = cb.Execute(func() error { return customErr })

	// Call 3 immediately rejected by open circuit breaker
	err := cb.Execute(func() error { return nil })
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}

	// Wait for cooldown -> Transitions to Half-Open
	time.Sleep(60 * time.Millisecond)

	// Call succeeds -> Resets circuit breaker to CLOSED
	err = cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected success in half-open, got %v", err)
	}

	// Normal call succeeds
	err = cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected success in closed state, got %v", err)
	}
}

func TestRetryWithBackoff(t *testing.T) {
	ctx := context.Background()
	opts := resilience.DefaultRetryOptions()
	opts.MaxAttempts = 3
	opts.BaseBackoff = 5 * time.Millisecond
	opts.MaxBackoff = 20 * time.Millisecond

	attempts := 0
	err := resilience.RetryWithBackoff(ctx, opts, func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("transient error")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected retry to succeed on 3rd attempt, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected exactly 3 attempts, got %d", attempts)
	}
}
