package edge

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
)

// defaultStreamRevocationCheck is how often an open stream re-checks its session
// against the revocation denylist (STREAM_REVOCATION_CHECK_SECONDS).
const defaultStreamRevocationCheck = 5 * time.Second

var (
	errStreamExpired = errors.New("token expired while the stream was open")
	errStreamRevoked = errors.New("session revoked while the stream was open")
)

// WithStreamRevocationCheck sets how often an open stream polls the revocation
// denylist (an in-memory read). Zero or negative keeps the default.
func (e *Edge) WithStreamRevocationCheck(d time.Duration) *Edge {
	e.streamRevocationEvery = d
	return e
}

// streamLifetime ends a server stream when the bearer token that opened it
// expires or its session is revoked. The edge authenticates a stream only at
// open; this keeps the stream from outliving the credential.
type streamLifetime struct {
	cancel context.CancelFunc
	done   chan struct{}
	cause  atomic.Pointer[error]
}

func (l *streamLifetime) stop() {
	l.cancel()
	<-l.done
}

// ended reports whether the lifetime watcher (not the client or the upstream)
// ended the stream.
func (l *streamLifetime) ended() bool { return l.cause.Load() != nil }

// watchStream returns a context cancelled when the stream's credential stops
// being valid. Callers must call stop when the handler returns. A principal with
// neither `exp` nor a revocable `sid` (anonymous) needs no watcher.
func (e *Edge) watchStream(ctx context.Context, p resolvedPrincipal) (context.Context, *streamLifetime) {
	ctx, cancel := context.WithCancel(ctx)
	l := &streamLifetime{cancel: cancel, done: make(chan struct{})}
	revocable := p.sid != "" && e.revocations != nil
	if p.exp.IsZero() && !revocable {
		close(l.done)
		return ctx, l
	}
	every := e.streamRevocationEvery
	if every <= 0 {
		every = defaultStreamRevocationCheck
	}
	go func() {
		defer close(l.done)
		var expiry <-chan time.Time
		if !p.exp.IsZero() {
			t := time.NewTimer(time.Until(p.exp))
			defer t.Stop()
			expiry = t.C
		}
		var tick <-chan time.Time
		if revocable {
			t := time.NewTicker(every)
			defer t.Stop()
			tick = t.C
			if e.revocations.Revoked(p.sid) {
				l.end(errStreamRevoked)
				return
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-expiry:
				l.end(errStreamExpired)
				return
			case <-tick:
				if e.revocations.Revoked(p.sid) {
					l.end(errStreamRevoked)
					return
				}
			}
		}
	}()
	return ctx, l
}

// reason is the short cause the watcher ended the stream with.
func (l *streamLifetime) reason() string {
	if c := l.cause.Load(); c != nil && errors.Is(*c, errStreamRevoked) {
		return "session_revoked"
	}
	return "token_expired"
}

func (l *streamLifetime) end(cause error) {
	l.cause.Store(&cause)
	l.cancel()
}

// endedError is the error a stream ends with once its credential stopped being
// valid: the same code and challenge as a bad token at open.
func endedError() error {
	cerr := connect.NewError(connect.CodeUnauthenticated, errInvalidToken)
	cerr.Meta().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	return cerr
}
