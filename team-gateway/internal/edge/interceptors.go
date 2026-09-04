package edge

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/time/rate"
)

// defaultLimiterTTL is how long an idle per-key bucket is kept before eviction.
// A key untouched for this long is dropped so the map can't grow without bound
// under many distinct principals/IPs (SA-Med: unbounded limiter map).
const defaultLimiterTTL = 10 * time.Minute

// limiterEntry pairs a token bucket with the last time its key was seen, so idle
// keys can be swept.
type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// rateLimiter is a per-key token bucket (one limiter per principal id, or per IP
// for anonymous callers). Idle keys are evicted after ttl so memory stays bounded.
//
// NOTE: this limiter is per gateway instance and in-process only. With N gateway
// replicas a caller's effective limit is multiplied by N (each replica keeps its
// own buckets). That is accepted for now; a shared/coordinated limit needs an
// out-of-process store.
// TODO(ADR-0010): shared limiter when gateway scales out.
type rateLimiter struct {
	mu        sync.Mutex
	limiters  map[string]*limiterEntry
	rps       rate.Limit
	burst     int
	ttl       time.Duration
	lastSweep time.Time
	now       func() time.Time // injectable clock; time.Now in production, fixed in tests
}

func newRateLimiter(rps float64, burst int) *rateLimiter {
	return &rateLimiter{
		limiters: map[string]*limiterEntry{},
		rps:      rate.Limit(rps),
		burst:    burst,
		ttl:      defaultLimiterTTL,
		now:      time.Now,
	}
}

// clientIP strips the port from a peer address so rate-limit keys are per-host.
func clientIP(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

func (r *rateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.sweep(now)
	e, ok := r.limiters[key]
	if !ok {
		e = &limiterEntry{limiter: rate.NewLimiter(r.rps, r.burst)}
		r.limiters[key] = e
	}
	e.lastSeen = now
	return e.limiter.Allow()
}

// sweep drops entries idle past the TTL. It runs at most once per ttl window
// (guarded by lastSweep) so the hot path stays O(1) amortized. Callers hold r.mu.
func (r *rateLimiter) sweep(now time.Time) {
	if r.ttl <= 0 || now.Sub(r.lastSweep) < r.ttl {
		return
	}
	r.lastSweep = now
	for k, e := range r.limiters {
		if now.Sub(e.lastSeen) >= r.ttl {
			delete(r.limiters, k)
		}
	}
}

// size reports the number of live per-key buckets (test/observability helper).
func (r *rateLimiter) size() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.limiters)
}

// Interceptors returns the ordered edge interceptor chain (outermost first):
// request-id -> auth (resolve Principal) -> logging -> rate limit.
func (e *Edge) Interceptors(logger *slog.Logger) []connect.Interceptor {
	return []connect.Interceptor{
		e.requestIDInterceptor(),
		e.authInterceptor(),
		e.loggingInterceptor(logger),
		e.rateLimitInterceptor(),
	}
}

func (e *Edge) requestIDInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			rid := strings.TrimSpace(req.Header().Get("X-Request-Id"))
			if rid == "" {
				rid = newRequestID()
			}
			ctx = withRequestID(ctx, rid)
			res, err := next(ctx, req)
			// On error, res is a typed-nil AnyResponse — calling Header() would
			// panic. Only stamp the header on a successful response.
			if err == nil && res != nil {
				res.Header().Set("X-Request-Id", rid)
			}
			return res, err
		}
	}
}

func (e *Edge) authInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			ctx = withPrincipal(ctx, e.resolve(req.Header()))
			return next(ctx, req)
		}
	}
}

func (e *Edge) loggingInterceptor(logger *slog.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()
			res, err := next(ctx, req)
			code := "ok"
			if err != nil {
				code = connect.CodeOf(err).String()
			}
			principalID := "anonymous"
			if p, ok := principalFrom(ctx); ok {
				principalID = p.id
			}
			logger.Info("edge.request",
				slog.String("method", req.Spec().Procedure),
				slog.String("principal", principalID),
				slog.String("code", code),
				slog.Int64("latency_ms", time.Since(start).Milliseconds()),
				slog.String("request_id", requestIDFrom(ctx)),
			)
			return res, err
		}
	}
}

func (e *Edge) rateLimitInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Key by identity; anonymous callers share a bucket per client IP
			// (strip the ephemeral port so a caller can't dodge the limit).
			key := "ip:" + clientIP(req.Peer().Addr)
			if p, ok := principalFrom(ctx); ok && p.id != "anonymous" {
				key = "user:" + p.id
			}
			if !e.limiter.allow(key) {
				return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("rate limit exceeded"))
			}
			return next(ctx, req)
		}
	}
}
