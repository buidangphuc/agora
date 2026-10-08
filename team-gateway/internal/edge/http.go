package edge

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// attemptCounter counts upstream attempts made while serving one request, so the
// edge.request log line can report them (a retried read shows attempts > 1).
type attemptCounter struct{ n atomic.Int32 }

type attemptsKeyType struct{}

func withAttempts(ctx context.Context, c *attemptCounter) context.Context {
	return context.WithValue(ctx, attemptsKeyType{}, c)
}

func (c *attemptCounter) add() {
	if c != nil {
		c.n.Add(1)
	}
}

func (c *attemptCounter) value() int {
	if c == nil {
		return 0
	}
	return int(c.n.Load())
}

func noteAttempt(ctx context.Context) {
	c, _ := ctx.Value(attemptsKeyType{}).(*attemptCounter)
	c.add()
}

// statusRecorder captures the status code a plain-HTTP handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// httpCode maps an HTTP status to the Connect code name the unary log uses.
func httpCode(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "ok"
	case status == http.StatusUnauthorized:
		return "unauthenticated"
	case status == http.StatusForbidden:
		return "permission_denied"
	case status == http.StatusTooManyRequests:
		return "resource_exhausted"
	case status == http.StatusBadRequest:
		return "invalid_argument"
	case status >= 500:
		return "internal"
	default:
		return "http_" + strconv.Itoa(status)
	}
}

// edgeHTTP gives a plain http.Handler the edge policy Connect handlers get from
// the interceptor chain: request id (validated or minted, echoed on the
// response), a per-caller rate limit and one edge.request log line. The wrapped
// handler keeps resolving the principal itself. A nil limiter skips the limit.
func (e *Edge) edgeHTTP(next http.Handler, route string, lim *rateLimiter, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rid := sanitizeRequestID(strings.TrimSpace(r.Header.Get("X-Request-Id")))
		w.Header().Set("X-Request-Id", rid)
		attempts := &attemptCounter{}
		ctx := withAttempts(withRequestID(r.Context(), rid), attempts)
		r = r.WithContext(ctx)
		rec := &statusRecorder{ResponseWriter: w}

		// Identify the caller for the bucket key and the log line. A verified
		// user is keyed by id; everyone else (including an invalid token, which
		// the handler answers itself) by client IP.
		principal, key := "anonymous", "ip:"+e.httpClientIP(r)
		if p := e.beaconPrincipal(r); p.GetId() != "anonymous" {
			principal, key = p.GetId(), "user:"+p.GetId()
		}

		if lim != nil && !lim.allow(key) {
			rec.Header().Set("Retry-After", "1")
			http.Error(rec, "rate limit exceeded", http.StatusTooManyRequests)
		} else {
			next.ServeHTTP(rec, r)
		}
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		logger.Info("edge.request",
			slog.String("method", route),
			slog.String("principal", principal),
			slog.String("code", httpCode(rec.status)),
			slog.Int("status", rec.status),
			slog.Int64("latency_ms", time.Since(start).Milliseconds()),
			slog.String("request_id", rid),
			slog.Int("attempts", attempts.value()),
		)
	})
}

func (e *Edge) httpClientIP(r *http.Request) string {
	if ip := e.clientInfoFor(r.RemoteAddr, r.Header).ip; ip != "" {
		return ip
	}
	return clientIP(r.RemoteAddr)
}
