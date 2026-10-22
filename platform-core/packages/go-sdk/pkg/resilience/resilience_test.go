package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/resilience"
)

var fastRetry = resilience.RetryOptions{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}

func retry(t *testing.T, o resilience.RetryOptions) resilience.Policy {
	t.Helper()
	p, err := resilience.Retry(o)
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	return p
}

func breaker(t *testing.T, o resilience.BreakerOptions) resilience.Policy {
	t.Helper()
	p, err := resilience.Breaker(o)
	if err != nil {
		t.Fatalf("Breaker: %v", err)
	}
	return p
}

func TestRetrySkipsNonRetryableCodes(t *testing.T) {
	for _, code := range []codes.Code{
		codes.InvalidArgument, codes.NotFound, codes.AlreadyExists,
		codes.PermissionDenied, codes.Unauthenticated, codes.FailedPrecondition,
	} {
		calls := 0
		err := resilience.Do(context.Background(), func(context.Context) error {
			calls++
			return status.Error(code, "nope")
		}, retry(t, fastRetry))
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
	}, retry(t, fastRetry))
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
	}, retry(t, fastRetry))
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
	}, retry(t, resilience.RetryOptions{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}))
	if calls != 1 {
		t.Fatalf("fn ran %d times, want 1", calls)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestBreakerOpensAfterFailures(t *testing.T) {
	br := breaker(t, resilience.BreakerOptions{FailureThreshold: 2, Delay: time.Minute})
	calls := 0
	fail := func(context.Context) error {
		calls++
		return status.Error(codes.Unavailable, "down")
	}
	for range 2 {
		_ = resilience.Do(context.Background(), fail, br)
	}
	err := resilience.Do(context.Background(), fail, br)
	if !errors.Is(err, resilience.ErrOpen) {
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
		{status.Error(codes.ResourceExhausted, ""), true},
		{status.Error(codes.Aborted, ""), true},
		{status.Error(codes.Internal, ""), false},
		{status.Error(codes.Unknown, ""), false},
		{errors.New("plain"), false},
		{context.DeadlineExceeded, false},
	}
	for _, c := range cases {
		if got := resilience.Retryable(c.err); got != c.want {
			t.Errorf("Retryable(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestRetryIfOverridesDefault(t *testing.T) {
	o := fastRetry
	o.RetryIf = func(err error) bool { return status.Code(err) == codes.Unavailable }
	calls := 0
	err := resilience.Do(context.Background(), func(context.Context) error {
		calls++
		return status.Error(codes.DeadlineExceeded, "slow")
	}, retry(t, o))
	if calls != 1 {
		t.Errorf("fn ran %d times, want 1 (RetryIf rejects DeadlineExceeded)", calls)
	}
	if status.Code(err) != codes.DeadlineExceeded {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}

	calls = 0
	_ = resilience.Do(context.Background(), func(context.Context) error {
		calls++
		return status.Error(codes.Unavailable, "down")
	}, retry(t, o))
	if calls != o.MaxAttempts {
		t.Errorf("Unavailable ran %d times, want %d", calls, o.MaxAttempts)
	}
}

func TestZeroValueRetryOptionsBounded(t *testing.T) {
	calls := 0
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := resilience.Do(ctx, func(context.Context) error {
		calls++
		return status.Error(codes.Unavailable, "down")
	}, retry(t, resilience.RetryOptions{}))
	if calls != resilience.DefaultMaxAttempts {
		t.Fatalf("fn ran %d times, want %d", calls, resilience.DefaultMaxAttempts)
	}
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("err = %v, want last Unavailable", err)
	}
	if time.Since(start) < resilience.DefaultBaseDelay/2 {
		t.Fatal("zero-value options retried without any backoff")
	}
}

func TestZeroValueBreakerOptionsDoNotPanic(t *testing.T) {
	br := breaker(t, resilience.BreakerOptions{})
	for range resilience.DefaultFailureThreshold {
		_ = resilience.Do(context.Background(), func(context.Context) error {
			return status.Error(codes.Unavailable, "down")
		}, br)
	}
	err := resilience.Do(context.Background(), func(context.Context) error { return nil }, br)
	if !errors.Is(err, resilience.ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen after the default threshold", err)
	}
}

func TestInvalidOptionsRejected(t *testing.T) {
	if _, err := resilience.Retry(resilience.RetryOptions{MaxAttempts: -1}); err == nil {
		t.Error("negative MaxAttempts accepted")
	}
	if _, err := resilience.Retry(resilience.RetryOptions{BaseDelay: time.Minute, MaxDelay: time.Second}); err == nil {
		t.Error("BaseDelay above MaxDelay accepted")
	}
	if _, err := resilience.Breaker(resilience.BreakerOptions{Delay: -time.Second}); err == nil {
		t.Error("negative breaker Delay accepted")
	}
}

func TestInternalAndPlainErrorsAreNotRetried(t *testing.T) {
	for _, e := range []error{status.Error(codes.Internal, "bug"), errors.New("plain")} {
		calls := 0
		_ = resilience.Do(context.Background(), func(context.Context) error {
			calls++
			return e
		}, retry(t, fastRetry))
		if calls != 1 {
			t.Errorf("%v: fn ran %d times, want 1", e, calls)
		}
	}
}

func TestDoRejectsZeroPolicy(t *testing.T) {
	called := false
	err := resilience.Do(context.Background(), func(context.Context) error { called = true; return nil },
		retry(t, fastRetry), resilience.Policy{})
	if err == nil {
		t.Fatal("Do with a zero Policy: want an error, not a panic")
	}
	if called {
		t.Error("fn ran although a policy was invalid")
	}
}
