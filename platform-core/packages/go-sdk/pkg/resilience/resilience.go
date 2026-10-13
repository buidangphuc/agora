package resilience

import (
	"context"
	"errors"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryOptions configures Retry. MaxAttempts counts the first call.
type RetryOptions struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	// RetryIf decides whether an error is retried. Nil keeps Retryable.
	RetryIf func(error) bool
}

// BreakerOptions configures Breaker. The breaker opens after FailureThreshold
// consecutive failures and stays open for Delay before probing again.
type BreakerOptions struct {
	FailureThreshold uint
	Delay            time.Duration
}

const jitterFactor = 0.25

// Retry retries retryable errors with exponential backoff and jitter.
func Retry(o RetryOptions) failsafe.Policy[any] {
	retryIf := o.RetryIf
	if retryIf == nil {
		retryIf = Retryable
	}
	return retrypolicy.NewBuilder[any]().
		WithMaxAttempts(o.MaxAttempts).
		WithBackoff(o.BaseDelay, o.MaxDelay).
		WithJitterFactor(jitterFactor).
		HandleIf(func(_ any, err error) bool { return retryIf(err) }).
		ReturnLastFailure().
		Build()
}

// Breaker fails calls fast with circuitbreaker.ErrOpen once the threshold of
// consecutive failures is reached.
func Breaker(o BreakerOptions) failsafe.Policy[any] {
	return circuitbreaker.NewBuilder[any]().
		WithFailureThreshold(o.FailureThreshold).
		WithDelay(o.Delay).
		Build()
}

// Do runs fn under policies, the first being the outermost. It stops as soon
// as ctx is done, including while waiting between attempts.
func Do(ctx context.Context, fn func(ctx context.Context) error, policies ...failsafe.Policy[any]) error {
	return failsafe.With(policies...).WithContext(ctx).Run(func() error { return fn(ctx) })
}

// Retryable reports whether err is worth retrying: context cancellation and
// gRPC codes that describe a caller or state problem are not.
func Retryable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	switch status.Code(err) {
	case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists,
		codes.PermissionDenied, codes.Unauthenticated, codes.FailedPrecondition:
		return false
	default:
		return true
	}
}
