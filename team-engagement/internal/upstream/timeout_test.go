package upstream

import (
	"context"
	"testing"
	"time"
)

func budget(ctx context.Context, cfg time.Duration) time.Duration {
	c, cancel := boundedCtx(ctx, cfg)
	defer cancel()
	dl, _ := c.Deadline()
	return time.Until(dl)
}

func TestBoundedCtx(t *testing.T) {
	near := func(got, want time.Duration) bool {
		d := got - want
		return d < 100*time.Millisecond && d > -100*time.Millisecond
	}

	if got := budget(context.Background(), time.Second); !near(got, time.Second) {
		t.Fatalf("no inbound deadline: want configured 1s, got %v", got)
	}
	if got := budget(context.Background(), 0); !near(got, DefaultCallTimeout) {
		t.Fatalf("zero config: want default, got %v", got)
	}
	long, c1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer c1()
	if got := budget(long, 2*time.Second); !near(got, 2*time.Second) {
		t.Fatalf("roomy inbound: want configured 2s, got %v", got)
	}
	tight, c2 := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer c2()
	if got := budget(tight, 2*time.Second); !near(got, 1200*time.Millisecond-deadlineMargin) {
		t.Fatalf("tight inbound: want remaining-margin, got %v", got)
	}
	gone, c3 := context.WithTimeout(context.Background(), deadlineMargin/2)
	defer c3()
	if got := budget(gone, 2*time.Second); got > 50*time.Millisecond {
		t.Fatalf("inbound inside the margin: lookup must fail at once, budget %v", got)
	}
}
