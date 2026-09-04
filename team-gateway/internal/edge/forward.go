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
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-gateway/internal/token"
)

// Forwarded Principal metadata keys — built fresh from a verified token so a
// client cannot spoof them.
const (
	mdPrincipalID     = "x-principal-id"
	mdPrincipalType   = "x-principal-type"
	mdPrincipalScopes = "x-principal-scopes"
	mdRequestID       = "x-request-id"
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
}

// Edge holds the request-scoped machinery: auth resolution, upstream call
// timeout + retry, and the rate limiter.
type Edge struct {
	verifier     *token.Verifier
	publicScopes []string
	callTimeout  time.Duration
	retryMax     int
	limiter      *rateLimiter
}

// NewEdge builds the edge helper. The verifier checks RS256 tokens against
// team-identity's JWKS (ADR-0006); the edge holds no signing material.
func NewEdge(verifier *token.Verifier, publicScopes []string, callTimeout time.Duration, retryMax int, rps float64, burst int) *Edge {
	return &Edge{
		verifier:     verifier,
		publicScopes: publicScopes,
		callTimeout:  callTimeout,
		retryMax:     retryMax,
		limiter:      newRateLimiter(rps, burst),
	}
}

// resolve verifies the bearer JWT (if any) into a Principal; no/invalid token →
// anonymous with the configured public scopes.
func (e *Edge) resolve(header http.Header) resolvedPrincipal {
	p := resolvedPrincipal{id: "anonymous", ptype: "anonymous", scopes: e.publicScopes}
	if tok := bearerToken(header.Get("Authorization")); tok != "" {
		if claims, err := e.verifier.Verify(tok); err == nil {
			p.id = claims.Subject
			if claims.Type != "" {
				p.ptype = claims.Type
			}
			p.scopes = claims.Scopes
		}
	}
	return p
}

// outgoing builds the outbound gRPC context carrying the resolved Principal +
// request id as trusted metadata.
func (e *Edge) outgoing(ctx context.Context, header http.Header) context.Context {
	p, ok := principalFrom(ctx)
	if !ok {
		p = e.resolve(header)
	}
	rid := requestIDFrom(ctx)
	if rid == "" {
		if rid = strings.TrimSpace(header.Get("X-Request-Id")); rid == "" {
			rid = newRequestID()
		}
	}
	md := metadata.MD{}
	md.Set(mdPrincipalID, p.id)
	md.Set(mdPrincipalType, p.ptype)
	md.Set(mdPrincipalScopes, strings.Join(p.scopes, ","))
	md.Set(mdRequestID, rid)
	return metadata.NewOutgoingContext(ctx, md)
}

// callRead runs an idempotent read with a deadline, retrying on Unavailable.
func (e *Edge) callRead(ctx context.Context, fn func(context.Context) error) error {
	var err error
	for attempt := 0; attempt <= e.retryMax; attempt++ {
		err = e.callOnce(ctx, fn)
		if err == nil || status.Code(err) != codes.Unavailable {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 50 * time.Millisecond):
		}
	}
	return err
}

// callWrite runs a non-idempotent call once, with a deadline (no retry).
func (e *Edge) callWrite(ctx context.Context, fn func(context.Context) error) error {
	return e.callOnce(ctx, fn)
}

func (e *Edge) callOnce(ctx context.Context, fn func(context.Context) error) error {
	c, cancel := context.WithTimeout(ctx, e.callTimeout)
	defer cancel()
	return fn(c)
}

func withPrincipal(ctx context.Context, p resolvedPrincipal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func principalFrom(ctx context.Context) (resolvedPrincipal, bool) {
	p, ok := ctx.Value(principalKey).(resolvedPrincipal)
	return p, ok
}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func bearerToken(raw string) string {
	const prefix = "bearer "
	if len(raw) >= len(prefix) && strings.EqualFold(raw[:len(prefix)], prefix) {
		return strings.TrimSpace(raw[len(prefix):])
	}
	return ""
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req-unknown"
	}
	return hex.EncodeToString(b[:])
}

// toConnectErr maps an upstream gRPC status to the equivalent Connect error.
func toConnectErr(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return connect.NewError(connect.CodeInternal, err)
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
		code = connect.CodeUnavailable
	case codes.DeadlineExceeded:
		code = connect.CodeDeadlineExceeded
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
		code = connect.CodeDataLoss
	default:
		code = connect.CodeInternal
	}
	return connect.NewError(code, errors.New(st.Message()))
}
