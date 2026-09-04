// Package resilience provides circuit breakers and exponential backoff retry with full jitter.
package resilience

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

var (
	ErrCircuitOpen = errors.New("circuit breaker is open; request rejected")
)

// State represents the Circuit Breaker lifecycle state.
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreaker protects downstream services from cascading failure.
type CircuitBreaker struct {
	mu           sync.Mutex
	state        State
	failureCount int
	maxFailures  int
	coolDown     time.Duration
	openedAt     time.Time
}

// NewCircuitBreaker creates a circuit breaker that trips after maxFailures and cools down for coolDown duration.
func NewCircuitBreaker(maxFailures int, coolDown time.Duration) *CircuitBreaker {
	if maxFailures <= 0 {
		maxFailures = 5
	}
	if coolDown <= 0 {
		coolDown = 10 * time.Second
	}
	return &CircuitBreaker{
		state:       StateClosed,
		maxFailures: maxFailures,
		coolDown:    coolDown,
	}
}

// Execute runs the operation if the circuit breaker allows it.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	if cb.state == StateOpen {
		if time.Since(cb.openedAt) > cb.coolDown {
			cb.state = StateHalfOpen
		} else {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failureCount++
		if cb.state == StateHalfOpen || cb.failureCount >= cb.maxFailures {
			cb.state = StateOpen
			cb.openedAt = time.Now()
		}
		return err
	}

	// Success
	if cb.state == StateHalfOpen {
		cb.state = StateClosed
	}
	cb.failureCount = 0
	return nil
}

// RetryOptions configures exponential backoff retry.
type RetryOptions struct {
	MaxAttempts  int
	BaseBackoff  time.Duration
	MaxBackoff   time.Duration
	IsTransient  func(err error) bool
}

// DefaultRetryOptions provides sensible defaults for microservices.
func DefaultRetryOptions() RetryOptions {
	return RetryOptions{
		MaxAttempts: 3,
		BaseBackoff: 50 * time.Millisecond,
		MaxBackoff:  2 * time.Second,
		IsTransient: func(err error) bool { return err != nil },
	}
}

// RetryWithBackoff executes an operation with exponential backoff and full jitter.
func RetryWithBackoff(ctx context.Context, opts RetryOptions, op func(ctx context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt < opts.MaxAttempts; attempt++ {
		err := op(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		if opts.IsTransient != nil && !opts.IsTransient(err) {
			return err // Non-retryable
		}

		if attempt == opts.MaxAttempts-1 {
			break
		}

		// Calculate Exponential Backoff with Full Jitter: Sleep = random(0, min(MaxBackoff, BaseBackoff * 2^attempt))
		backoff := float64(opts.BaseBackoff) * math.Pow(2, float64(attempt))
		if backoff > float64(opts.MaxBackoff) {
			backoff = float64(opts.MaxBackoff)
		}
		jitter := time.Duration(rand.Float64() * backoff)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jitter):
		}
	}
	return fmt.Errorf("exceeded %d retry attempts, last error: %w", opts.MaxAttempts, lastErr)
}
