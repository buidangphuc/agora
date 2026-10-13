package edge

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// callRead retries an idempotent read on Unavailable only, up to retryMax more
// times, and returns the last upstream error unchanged.
func TestCallReadRetriesOnUnavailableOnly(t *testing.T) {
	cases := []struct {
		name      string
		code      codes.Code
		retryMax  int
		wantCalls int
	}{
		{"unavailable is retried", codes.Unavailable, 2, 3},
		{"unavailable without retries", codes.Unavailable, 0, 1},
		{"deadline exceeded is not retried", codes.DeadlineExceeded, 2, 1},
		{"internal is not retried", codes.Internal, 2, 1},
		{"resource exhausted is not retried", codes.ResourceExhausted, 2, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEdge(nil, nil, time.Second, tc.retryMax, 1000, 1000)
			calls := 0
			err := e.callRead(context.Background(), func(context.Context) error {
				calls++
				return status.Error(tc.code, "boom")
			})
			if calls != tc.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tc.wantCalls)
			}
			if status.Code(err) != tc.code || status.Convert(err).Message() != "boom" {
				t.Errorf("err = %v, want the upstream %s error unchanged", err, tc.code)
			}
		})
	}

	t.Run("success after a retry", func(t *testing.T) {
		e := NewEdge(nil, nil, time.Second, 2, 1000, 1000)
		calls := 0
		err := e.callRead(context.Background(), func(context.Context) error {
			calls++
			if calls == 1 {
				return status.Error(codes.Unavailable, "down")
			}
			return nil
		})
		if err != nil || calls != 2 {
			t.Errorf("err = %v, calls = %d, want nil after 2 calls", err, calls)
		}
	})

	t.Run("a cancelled context stops the retry wait", func(t *testing.T) {
		e := NewEdge(nil, nil, time.Second, 5, 1000, 1000)
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		err := e.callRead(ctx, func(context.Context) error {
			calls++
			cancel()
			return status.Error(codes.Unavailable, "down")
		})
		if calls != 1 || err == nil {
			t.Errorf("calls = %d, err = %v, want one call and an error", calls, err)
		}
	})
}
