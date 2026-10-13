package resilience

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Defaults applied to the zero value of RetryOptions and BreakerOptions.
const (
	DefaultMaxAttempts      = 3
	DefaultBaseDelay        = 100 * time.Millisecond
	DefaultMaxDelay         = 5 * time.Second
	DefaultFailureThreshold = 5
	DefaultBreakerDelay     = 30 * time.Second
)

const jitterFactor = 0.25

// ErrOpen is returned by Do, wrapped, when a Breaker policy rejects the call
// because it is open. Test with errors.Is.
var ErrOpen = circuitbreaker.ErrOpen

// Policy is a retry or breaker policy built by Retry or Breaker and run by Do.
// Its implementation is private so the SDK API does not depend on the
// underlying library.
type Policy struct {
	p failsafe.Policy[any]
}

// RetryOptions configures Retry. Every zero field takes its default.
type RetryOptions struct {
	// MaxAttempts counts the first call. Zero means DefaultMaxAttempts.
	MaxAttempts int
	// BaseDelay is the first backoff. Zero means DefaultBaseDelay.
	BaseDelay time.Duration
	// MaxDelay caps the backoff. Zero means DefaultMaxDelay, or BaseDelay when
	// that is larger.
	MaxDelay time.Duration
	// RetryIf decides whether an error is retried. Nil keeps Retryable.
	RetryIf func(error) bool
}

// BreakerOptions configures Breaker. The breaker opens after FailureThreshold
// consecutive failures and stays open for Delay before probing again. Every
// zero field takes its default.
type BreakerOptions struct {
	// FailureThreshold is the consecutive failures that open the breaker. Zero
	// means DefaultFailureThreshold.
	FailureThreshold uint
	// Delay is how long the breaker stays open. Zero means DefaultBreakerDelay.
	Delay time.Duration
}

// Retry returns a policy that retries retryable errors with exponential
// backoff and jitter. It returns an error for a negative value or a BaseDelay
// above MaxDelay.
func Retry(o RetryOptions) (Policy, error) {
	if o.MaxAttempts < 0 || o.BaseDelay < 0 || o.MaxDelay < 0 {
		return Policy{}, fmt.Errorf("resilience: retry options must not be negative: %+v", o)
	}
	if o.MaxAttempts == 0 {
		o.MaxAttempts = DefaultMaxAttempts
	}
	if o.BaseDelay == 0 {
		o.BaseDelay = DefaultBaseDelay
	}
	if o.MaxDelay == 0 {
		o.MaxDelay = max(DefaultMaxDelay, o.BaseDelay)
	}
	if o.BaseDelay > o.MaxDelay {
		return Policy{}, fmt.Errorf("resilience: BaseDelay %s exceeds MaxDelay %s", o.BaseDelay, o.MaxDelay)
	}
	retryIf := o.RetryIf
	if retryIf == nil {
		retryIf = Retryable
	}
	return Policy{retrypolicy.NewBuilder[any]().
		WithMaxAttempts(o.MaxAttempts).
		WithBackoff(o.BaseDelay, o.MaxDelay).
		WithJitterFactor(jitterFactor).
		HandleIf(func(_ any, err error) bool { return retryIf(err) }).
		ReturnLastFailure().
		Build()}, nil
}

// Breaker returns a policy that fails calls fast with ErrOpen once the
// threshold of consecutive failures is reached. It returns an error for a
// negative Delay.
func Breaker(o BreakerOptions) (Policy, error) {
	if o.Delay < 0 {
		return Policy{}, fmt.Errorf("resilience: breaker Delay must not be negative: %s", o.Delay)
	}
	if o.FailureThreshold == 0 {
		o.FailureThreshold = DefaultFailureThreshold
	}
	if o.Delay == 0 {
		o.Delay = DefaultBreakerDelay
	}
	return Policy{circuitbreaker.NewBuilder[any]().
		WithFailureThreshold(o.FailureThreshold).
		WithDelay(o.Delay).
		Build()}, nil
}

// Do runs fn under policies, the first being the outermost. It stops as soon
// as ctx is done, including while waiting between attempts.
func Do(ctx context.Context, fn func(ctx context.Context) error, policies ...Policy) error {
	ps := make([]failsafe.Policy[any], len(policies))
	for i, p := range policies {
		ps[i] = p.p
	}
	return failsafe.With(ps...).WithContext(ctx).Run(func() error { return fn(ctx) })
}

// Retryable reports whether err is worth retrying. Only gRPC statuses that
// describe a transient condition are: Unavailable, ResourceExhausted, Aborted
// and DeadlineExceeded. Everything else, including Internal, plain errors and
// context cancellation, is not.
func Retryable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.ResourceExhausted, codes.Aborted, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}
