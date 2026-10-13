package upstream

import (
	"context"
	"time"

	sdkupstream "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/upstream"
)

// DefaultCallTimeout bounds one upstream lookup when none is configured.
const DefaultCallTimeout = sdkupstream.DefaultTimeout

// boundedCtx derives the context one upstream lookup runs under: its budget is the
// configured per-call timeout (DefaultCallTimeout when <=0), cut down by the SDK's
// upstream.Bounded to the remaining inbound deadline minus its margin so a dead
// upstream can never consume the whole request budget. When the inbound deadline
// is already inside the margin the context is already expired: the lookup fails at once.
//
// Callers must run their own follow-up work (inserts) on the ORIGINAL ctx, not this one.
func boundedCtx(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = DefaultCallTimeout
	}
	return sdkupstream.Bounded(ctx, timeout)
}
