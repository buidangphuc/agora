package edge

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
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

// edgeInterceptor adapts one edge step to all three Connect shapes: unary
// handlers, streaming handlers, and (pass-through) streaming clients. The same
// steps therefore run for unary calls and streams (Connect skips unary-only
// interceptors for streaming handlers).
type edgeInterceptor struct {
	unary  func(connect.UnaryFunc) connect.UnaryFunc
	stream func(connect.StreamingHandlerFunc) connect.StreamingHandlerFunc
}

func (i edgeInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return i.unary(next)
}

func (i edgeInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i edgeInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return i.stream(next)
}

// Interceptors returns the ordered edge interceptor chain (outermost first):
// request-id -> auth (resolve Principal) -> logging -> rate limit. Every step
// covers unary calls and server/bidi streams.
func (e *Edge) Interceptors(logger *slog.Logger) []connect.Interceptor {
	return []connect.Interceptor{
		e.requestIDInterceptor(),
		e.authInterceptor(),
		e.loggingInterceptor(logger),
		e.rateLimitInterceptor(),
	}
}

func (e *Edge) requestIDInterceptor() connect.Interceptor {
	return edgeInterceptor{
		unary: func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				rid := sanitizeRequestID(strings.TrimSpace(req.Header().Get("X-Request-Id")))
				ctx = withRequestID(ctx, rid)
				res, err := next(ctx, req)
				// On error, res is a typed-nil AnyResponse — calling Header() would
				// panic. Only stamp the header on a successful response.
				if err == nil && res != nil {
					res.Header().Set("X-Request-Id", rid)
				}
				return res, err
			}
		},
		stream: func(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
			return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
				rid := sanitizeRequestID(strings.TrimSpace(conn.RequestHeader().Get("X-Request-Id")))
				// Before the first message, so the header is sent with the stream.
				conn.ResponseHeader().Set("X-Request-Id", rid)
				return next(withRequestID(ctx, rid), conn)
			}
		},
	}
}

// admit resolves the principal for a call and applies the edge procedure
// policy. A presented-but-invalid token is a 401 (RFC 6750 §3.1), never a
// silent anonymous downgrade.
func (e *Edge) admit(ctx context.Context, procedure, peer string, header http.Header) (context.Context, error) {
	p, err := e.resolve(header)
	if err != nil {
		cerr := connect.NewError(connect.CodeUnauthenticated, err)
		cerr.Meta().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		return ctx, cerr
	}
	// Edge procedure policy (adminProcedures): gate before forwarding.
	if err := requireProcedureScope(procedure, p); err != nil {
		return ctx, err
	}
	ctx = withPrincipal(ctx, p)
	ctx = withClient(ctx, e.clientInfoFor(peer, header))
	return ctx, nil
}

func (e *Edge) authInterceptor() connect.Interceptor {
	return edgeInterceptor{
		unary: func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				ctx, err := e.admit(ctx, req.Spec().Procedure, req.Peer().Addr, req.Header())
				if err != nil {
					return nil, err
				}
				return next(ctx, req)
			}
		},
		stream: func(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
			return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
				ctx, err := e.admit(ctx, conn.Spec().Procedure, conn.Peer().Addr, conn.RequestHeader())
				if err != nil {
					return err
				}
				// The token was valid at open; end the stream when it expires or
				// its session is revoked (the upstream call is cancelled with it).
				p, _ := principalFrom(ctx)
				ctx, life := e.watchStream(ctx, p)
				defer life.stop()
				err = next(ctx, conn)
				// Also when the handler ended cleanly: an upstream may turn the cancellation
				// into EOF, and the client must not read a cut stream as a completed one.
				if life.ended() {
					return endedError()
				}
				return err
			}
		},
	}
}

func (e *Edge) logRequest(logger *slog.Logger, ctx context.Context, procedure string, start time.Time, err error, attempts *attemptCounter) {
	code := "ok"
	if err != nil {
		code = connect.CodeOf(err).String()
		logUpstreamError(logger, procedure, requestIDFrom(ctx), err)
	}
	principalID := "anonymous"
	if p, ok := principalFrom(ctx); ok {
		principalID = p.id
	}
	logger.Info("edge.request",
		slog.String("method", procedure),
		slog.String("principal", principalID),
		slog.String("code", code),
		slog.Int64("latency_ms", time.Since(start).Milliseconds()),
		slog.String("request_id", requestIDFrom(ctx)),
		slog.Int("attempts", attempts.value()),
	)
}

func (e *Edge) loggingInterceptor(logger *slog.Logger) connect.Interceptor {
	return edgeInterceptor{
		unary: func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				start := time.Now()
				attempts := &attemptCounter{}
				res, err := next(withAttempts(ctx, attempts), req)
				e.logRequest(logger, ctx, req.Spec().Procedure, start, err, attempts)
				return res, err
			}
		},
		stream: func(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
			return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
				start := time.Now()
				attempts := &attemptCounter{}
				err := next(withAttempts(ctx, attempts), conn)
				e.logRequest(logger, ctx, conn.Spec().Procedure, start, err, attempts)
				return err
			}
		},
	}
}

// limitKey keys a bucket by identity; anonymous callers share a bucket per
// client IP (the ephemeral port is stripped so a caller can't dodge the limit).
func limitKey(ctx context.Context, peer string) string {
	if p, ok := principalFrom(ctx); ok && p.id != "anonymous" {
		return "user:" + p.id
	}
	return "ip:" + clientIP(peer)
}

var errRateLimited = errors.New("rate limit exceeded")

func (e *Edge) rateLimitInterceptor() connect.Interceptor {
	return edgeInterceptor{
		unary: func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				if !e.limiter.allow(limitKey(ctx, req.Peer().Addr)) {
					return nil, connect.NewError(connect.CodeResourceExhausted, errRateLimited)
				}
				return next(ctx, req)
			}
		},
		stream: func(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
			return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
				// One stream costs one token, the same as a unary call.
				if !e.limiter.allow(limitKey(ctx, conn.Peer().Addr)) {
					return connect.NewError(connect.CodeResourceExhausted, errRateLimited)
				}
				return next(ctx, conn)
			}
		},
	}
}
