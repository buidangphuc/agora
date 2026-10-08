package upstream

import (
	"context"
	"time"
)

// DefaultCallTimeout bounds one upstream lookup when none is configured.
const DefaultCallTimeout = 2 * time.Second

// deadlineMargin is what a bounded lookup leaves of the inbound deadline for the
// caller's own remaining work (DB insert, response) after the lookup gives up.
const deadlineMargin = 500 * time.Millisecond

// boundedCtx derives the context one upstream lookup runs under: its budget is the
// configured per-call timeout (DefaultCallTimeout when <=0), cut down to the remaining
// inbound deadline minus deadlineMargin so a dead upstream can never consume the whole
// request budget. When the inbound deadline is already inside the margin the budget
// is 0 and the returned context is already expired: the lookup fails at once.
//
// Callers must run their own follow-up work (inserts) on the ORIGINAL ctx, not this one.
func boundedCtx(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = DefaultCallTimeout
	}
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl) - deadlineMargin; rem < timeout {
			if rem < 0 {
				rem = 0
			}
			timeout = rem
		}
	}
	return context.WithTimeout(ctx, timeout)
}
