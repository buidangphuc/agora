package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/resilience"
)

var fastRetry = resilience.RetryOptions{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}

func TestRetrySkipsNonRetryableCodes(t *testing.T) {
	for _, code := range []codes.Code{
		codes.InvalidArgument, codes.NotFound, codes.AlreadyExists,
		codes.PermissionDenied, codes.Unauthenticated, codes.FailedPrecondition,
	} {
		calls := 0
		err := resilience.Do(context.Background(), func(context.Context) error {
			calls++
			return status.Error(code, "nope")
		}, resilience.Retry(fastRetry))
		if calls != 1 {
			t.Errorf("%v: fn ran %d times, want 1", code, calls)
		}
		if status.Code(err) != code {
			t.Errorf("%v: got err %v", code, err)
		}
	}
}

func TestRetryRetriesUnavailable(t *testing.T) {
	calls := 0
	err := resilience.Do(context.Background(), func(context.Context) error {
		calls++
		if calls < 3 {
			return status.Error(codes.Unavailable, "try again")
		}
		return nil
	}, resilience.Retry(fastRetry))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if calls != 3 {
		t.Fatalf("fn ran %d times, want 3", calls)
	}
}

func TestRetryGivesUpAfterMaxAttempts(t *testing.T) {
	calls := 0
	err := resilience.Do(context.Background(), func(context.Context) error {
		calls++
		return status.Error(codes.Unavailable, "down")
	}, resilience.Retry(fastRetry))
	if calls != 3 {
		t.Fatalf("fn ran %d times, want 3", calls)
	}
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want last error, got %v", err)
	}
}

func TestDoStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := resilience.Do(ctx, func(context.Context) error {
		calls++
		cancel()
		return status.Error(codes.Unavailable, "down")
	}, resilience.Retry(resilience.RetryOptions{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}))
	if calls != 1 {
		t.Fatalf("fn ran %d times, want 1", calls)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestBreakerOpensAfterFailures(t *testing.T) {
	br := resilience.Breaker(resilience.BreakerOptions{FailureThreshold: 2, Delay: time.Minute})
	calls := 0
	fail := func(context.Context) error {
		calls++
		return status.Error(codes.Unavailable, "down")
	}
	for range 2 {
		_ = resilience.Do(context.Background(), fail, br)
	}
	err := resilience.Do(context.Background(), fail, br)
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("want ErrOpen, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("fn ran %d times, want 2", calls)
	}
}

func TestRetryable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{context.Canceled, false},
		{status.Error(codes.Unavailable, ""), true},
		{status.Error(codes.DeadlineExceeded, ""), true},
		{status.Error(codes.NotFound, ""), false},
		{errors.New("plain"), true},
	}
	for _, c := range cases {
		if got := resilience.Retryable(c.err); got != c.want {
			t.Errorf("Retryable(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
