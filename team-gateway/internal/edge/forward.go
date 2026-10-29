// Package edge is the gateway's Connect layer: it exposes each contract service
// over Connect (gRPC + gRPC-web + JSON), resolves the caller's Principal at the
// edge (ADR-0003), and forwards every call to the upstream gRPC service with a
// deadline + read retry. No business logic here (Rule 2).
package edge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/idempotency"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/resilience"
	"github.com/buidangphuc/team-gateway/internal/token"
)

type ctxKey int

const (
	principalKey ctxKey = iota
	requestIDKey
)

// resolvedPrincipal is the identity the edge resolved for a request.
type resolvedPrincipal struct {
	id     string
	ptype  string
	scopes []string
	// exp and sid come from the verified token (zero for anonymous); streams use
	// them to end at expiry or revocation.
	exp time.Time
	sid string
}

// Edge holds the request-scoped machinery: auth resolution, upstream call
// timeout + retry, and the rate limiter.
type Edge struct {
	verifier     *token.Verifier
	revocations  SessionRevocations
	trusted      *TrustedProxies
	publicScopes []string
	callTimeout  time.Duration
	retryMax     int
	limiter      *rateLimiter
	// trackLimiter is the collector's own bucket (nil = unlimited) so a beacon
	// burst never drains the caller's RPC bucket.
	trackLimiter   *rateLimiter
	aiTimeout      time.Duration
	streamMaxBytes int
	// streamRevocationEvery is how often an open stream polls the revocation list.
	streamRevocationEvery time.Duration
	reflection            bool
	retry                 resilience.Policy
	trackTimeout          time.Duration
}

const defaultTrackPublishTimeout = 2 * time.Second

// WithTrackPublishTimeout bounds one tracking publish (TRACK_PUBLISH_TIMEOUT).
func (e *Edge) WithTrackPublishTimeout(d time.Duration) *Edge {
	e.trackTimeout = d
	return e
}

// defaultStreamMaxBytes caps a streaming request message (STREAM_MAX_REQUEST_BYTES).
const defaultStreamMaxBytes = 16384

// WithTrackLimit gives POST /api/track its own per-visitor bucket.
func (e *Edge) WithTrackLimit(rps float64, burst int) *Edge {
	e.trackLimiter = newRateLimiter(rps, burst)
	return e
}

// WithAICallTimeout sets the single-attempt deadline for AI generation calls.
func (e *Edge) WithAICallTimeout(d time.Duration) *Edge {
	e.aiTimeout = d
	return e
}

// WithStreamMaxBytes caps the size of one streaming request message.
func (e *Edge) WithStreamMaxBytes(n int) *Edge {
	e.streamMaxBytes = n
	return e
}

// WithReflection mounts gRPC reflection (off by default).
func (e *Edge) WithReflection(on bool) *Edge {
	e.reflection = on
	return e
}

// SessionRevocations answers whether a session id was revoked (ADR-0003 addendum).
// Satisfied by *revocation.Denylist; nil means "no revocation source" and every
// session is treated as live (fail open).
type SessionRevocations interface {
	Revoked(sessionID string) bool
}

// WithRevocations makes the edge reject any token whose `sid` is revoked. It is
// optional and set once at startup, before the edge serves.
func (e *Edge) WithRevocations(r SessionRevocations) *Edge {
	e.revocations = r
	return e
}

// verifyToken verifies a bearer token locally (signature, expiry) and then checks
// its session against the revocation denylist. Both failures read as an invalid
// token; there is no call to identity.
func (e *Edge) verifyToken(tok string) (*token.Claims, error) {
	claims, err := e.verifier.Verify(tok)
	if err != nil {
		return nil, err
	}
	if claims.SessionID != "" && e.revocations != nil && e.revocations.Revoked(claims.SessionID) {
		return nil, errSessionRevoked
	}
	return claims, nil
}

// NewEdge builds the edge helper. The verifier checks RS256 tokens against
// team-identity's JWKS (ADR-0006); the edge holds no signing material.
func NewEdge(verifier *token.Verifier, publicScopes []string, callTimeout time.Duration, retryMax int, rps float64, burst int) *Edge {
	retry, _ := resilience.Retry(resilience.RetryOptions{
		MaxAttempts: max(retryMax, 0) + 1,
		BaseDelay:   50 * time.Millisecond,
		MaxDelay:    200 * time.Millisecond,
		RetryIf:     func(err error) bool { return status.Code(err) == codes.Unavailable },
	})
	return &Edge{
		retry:        retry,
		trackTimeout: defaultTrackPublishTimeout,
		verifier:     verifier,
		publicScopes: publicScopes,
		callTimeout:  callTimeout,
		retryMax:     retryMax,
		limiter:      newRateLimiter(rps, burst),
		aiTimeout:    30 * time.Second,
	}
}

// errInvalidToken marks a bearer credential that was presented but did not
// verify (malformed, bad signature, unknown kid, expired).
var errInvalidToken = errors.New("invalid or expired bearer token")

// errSessionRevoked marks a token whose session was revoked. It is reported to the
// client exactly like any invalid token (resolve maps every verify failure to
// errInvalidToken).
var errSessionRevoked = errors.New("session revoked")

// resolve turns the Authorization header into a Principal (RFC 6750 §3.1):
//   - no bearer credential        → anonymous with the configured public scopes;
//   - a bearer that verifies      → the token's Principal;
//   - a bearer that does not      → errInvalidToken. A presented-but-bad token is
//     never silently downgraded to anonymous; callers must answer Unauthenticated.
func (e *Edge) resolve(header http.Header) (resolvedPrincipal, error) {
	p := resolvedPrincipal{id: "anonymous", ptype: "anonymous", scopes: e.publicScopes}
	tok, present := bearerCredential(header.Get("Authorization"))
	if !present {
		return p, nil
	}
	claims, err := e.verifyToken(tok)
	if err != nil {
		return p, errInvalidToken
	}
	p.id = claims.Subject
	if claims.Type != "" {
		p.ptype = claims.Type
	}
	p.scopes = claims.Scopes
	p.sid = claims.SessionID
	if claims.ExpiresAt != nil {
		p.exp = claims.ExpiresAt.Time
	}
	return p, nil
}

// outgoing builds the outbound gRPC context carrying the resolved Principal +
// request id as trusted metadata.
func (e *Edge) outgoing(ctx context.Context, header http.Header) context.Context {
	p, ok := principalFrom(ctx)
	if !ok {
		// Reached only when the auth interceptor did not run (direct forwarder
		// use). Fail closed: an invalid token gets no scopes at all.
		var err error
		if p, err = e.resolve(header); err != nil {
			p.scopes = nil
		}
	}
	rid := requestIDFrom(ctx)
	if rid == "" {
		rid = sanitizeRequestID(header.Get("X-Request-Id"))
	}
	md := metadata.MD{}
	md.Set(auth.MDPrincipalID, p.id)
	md.Set(auth.MDPrincipalType, p.ptype)
	md.Set(auth.MDPrincipalScopes, strings.Join(p.scopes, ","))
	md.Set(reqctx.MetadataRequestID, rid)
	// Client context is built here from edge-observed values only, never copied
	// from inbound headers, so a client-supplied x-client-* cannot pass through.
	ci, ok := clientFrom(ctx)
	if !ok {
		// Auth interceptor did not run (direct forwarder use): no peer address is
		// known, so forward no IP — only the user agent.
		ci = clientInfo{userAgent: clip(header.Get("User-Agent"), maxClientUALen)}
	}
	if ci.ip != "" {
		md.Set(mdClientIP, ci.ip)
	}
	if ci.userAgent != "" {
		md.Set(mdClientUserAgent, ci.userAgent)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

// callRead runs an idempotent read with a deadline, retrying on Unavailable.
func (e *Edge) callRead(ctx context.Context, fn func(context.Context) error) error {
	return resilience.Do(ctx, func(ctx context.Context) error {
		noteAttempt(ctx)
		return e.callOnce(ctx, fn)
	}, e.retry)
}

// callWrite runs a non-idempotent call once, with a deadline (no retry).
func (e *Edge) callWrite(ctx context.Context, fn func(context.Context) error) error {
	noteAttempt(ctx)
	return e.callOnce(ctx, fn)
}

// callAI runs an AI generation call exactly once (never retried on Unavailable:
// generation is slow and costly) under AI_CALL_TIMEOUT_SECONDS, not the read deadline.
func (e *Edge) callAI(ctx context.Context, fn func(context.Context) error) error {
	noteAttempt(ctx)
	d := e.aiTimeout
	if d <= 0 {
		d = e.callTimeout
	}
	c, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return fn(c)
}

func (e *Edge) callOnce(ctx context.Context, fn func(context.Context) error) error {
	c, cancel := context.WithTimeout(ctx, e.callTimeout)
	defer cancel()
	return fn(c)
}

func withPrincipal(ctx context.Context, p resolvedPrincipal) context.Context {
	ctx = auth.WithPrincipal(ctx, auth.Principal{ID: p.id, Type: auth.PrincipalType(p.ptype), Scopes: p.scopes})
	return context.WithValue(ctx, principalKey, p)
}

func principalFrom(ctx context.Context) (resolvedPrincipal, bool) {
	p, ok := ctx.Value(principalKey).(resolvedPrincipal)
	return p, ok
}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(reqctx.WithRequestID(ctx, id), requestIDKey, id)
}

func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// bearerCredential reports whether the header carries a Bearer credential and
// returns its token. An empty "Bearer" value counts as present-but-invalid.
// Other schemes (Basic, ...) are not ours to judge and read as "no credential".
func bearerCredential(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	scheme, rest, _ := strings.Cut(raw, " ")
	if !strings.EqualFold(scheme, "bearer") {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

func bearerToken(raw string) string {
	const prefix = "bearer "
	if len(raw) >= len(prefix) && strings.EqualFold(raw[:len(prefix)], prefix) {
		return strings.TrimSpace(raw[len(prefix):])
	}
	return ""
}

// maxRequestIDLen keeps the edge limit at 64, tighter than the SDK shape check.
const maxRequestIDLen = 64

// sanitizeRequestID returns the client's id when it is safe, else a fresh one.
func sanitizeRequestID(raw string) string {
	if len(raw) <= maxRequestIDLen && reqctx.ValidRequestID(raw) {
		return raw
	}
	return newRequestID()
}

// errInvalidIdempotencyKey is the fixed client-facing message for an
// Idempotency-Key header that is not a single printable-ASCII value.
const errInvalidIdempotencyKey = "invalid Idempotency-Key header: must be a single printable-ASCII value of at most 255 characters"

// outgoingWithIdempotencyKey is outgoing plus the Idempotency-Key header
// forwarded as `idempotency-key` metadata. A missing header adds nothing. A
// header that is repeated, empty-but-present, over-long or not printable ASCII
// is a client error (invalid_argument), never silently dropped: dropping it would
// disable duplicate-order protection. Used by CreateOrder only.
func (e *Edge) outgoingWithIdempotencyKey(ctx context.Context, header http.Header) (context.Context, error) {
	out := e.outgoing(ctx, header)
	vals := header.Values("Idempotency-Key")
	if len(vals) == 0 {
		return out, nil
	}
	if len(vals) > 1 || idempotency.Validate(strings.TrimSpace(vals[0])) != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New(errInvalidIdempotencyKey))
	}
	return metadata.AppendToOutgoingContext(out, idempotency.MDKey, strings.TrimSpace(vals[0])), nil
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req-unknown"
	}
	return hex.EncodeToString(b[:])
}

// Generic client-facing messages for server-side upstream failures. The raw
// upstream text (driver errors, hostnames, stack hints) never reaches a client.
const (
	msgInternalError      = "internal error"
	msgServiceUnavailable = "service unavailable"
	msgUpstreamTimeout    = "upstream timed out"
)

// upstreamError carries the original upstream error behind a sanitised Connect
// error. The logging interceptor records Original() with the request id.
type upstreamError struct {
	msg      string // generic, client-facing
	original error
}

func (e *upstreamError) Error() string   { return e.msg }
func (e *upstreamError) Original() error { return e.original }
func (e *upstreamError) Unwrap() error   { return e.original }

// sanitized builds a Connect error with a fixed message, keeping the original as
// a typed cause (never serialised to the client).
func sanitized(code connect.Code, msg string, original error) error {
	return connect.NewError(code, &upstreamError{msg: msg, original: original})
}

// logUpstreamError records the original (pre-sanitisation) upstream error with
// the request id so operators can correlate the generic client message. It is a
// no-op for errors that were not sanitised.
func logUpstreamError(logger *slog.Logger, procedure, rid string, err error) {
	var ue *upstreamError
	if errors.As(err, &ue) {
		logger.Error("edge.upstream_error",
			slog.String("method", procedure),
			slog.String("request_id", rid),
			slog.String("code", connect.CodeOf(err).String()),
			slog.String("original", ue.Original().Error()),
		)
	}
}

// toConnectErr maps an upstream gRPC status to the equivalent Connect error. It
// is the single sanitisation point: Internal/Unknown/DataLoss/non-status errors
// become "internal error" and Unavailable becomes "service unavailable" (codes
// kept); every client-meaningful code keeps the upstream message.
func toConnectErr(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return sanitized(connect.CodeInternal, msgInternalError, err)
	}
	var code connect.Code
	switch st.Code() {
	case codes.OK:
		return nil
	case codes.InvalidArgument:
		code = connect.CodeInvalidArgument
	case codes.NotFound:
		code = connect.CodeNotFound
	case codes.AlreadyExists:
		code = connect.CodeAlreadyExists
	case codes.PermissionDenied:
		code = connect.CodePermissionDenied
	case codes.Unauthenticated:
		code = connect.CodeUnauthenticated
	case codes.Unavailable:
		return sanitized(connect.CodeUnavailable, msgServiceUnavailable, err)
	case codes.DeadlineExceeded:
		// raised by the gateway's own gRPC client while it waits for an upstream
		// (e.g. a stopped service), so the text is resolver/LB internals
		return sanitized(connect.CodeDeadlineExceeded, msgUpstreamTimeout, err)
	case codes.FailedPrecondition:
		code = connect.CodeFailedPrecondition
	case codes.ResourceExhausted:
		code = connect.CodeResourceExhausted
	case codes.Aborted:
		code = connect.CodeAborted
	case codes.OutOfRange:
		code = connect.CodeOutOfRange
	case codes.Unimplemented:
		code = connect.CodeUnimplemented
	case codes.Canceled:
		code = connect.CodeCanceled
	case codes.DataLoss:
		return sanitized(connect.CodeDataLoss, msgInternalError, err)
	default: // Internal, Unknown and any future code
		return sanitized(connect.CodeInternal, msgInternalError, err)
	}
	return connect.NewError(code, errors.New(st.Message()))
}
