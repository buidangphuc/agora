package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"

	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/token"
)

// jwksServer is the JWKS HTTP listener (ADR-0006): identity is otherwise
// gRPC-only. This tiny, unauthenticated, read-only server publishes the signer's
// RSA public key at /.well-known/jwks.json so the edge can verify RS256 tokens by
// kid. It runs alongside gRPC and is drained on shutdown. No auth, no business
// logic.
//
// It stays local rather than lifecycle.HTTP: that returns an error when its grace
// runs out, which would turn a slow JWKS shutdown into a non-zero exit of the
// whole service; here a failed shutdown is only logged and the exit code is
// unchanged.
func jwksServer(s config.Settings, signer *token.Signer, logger *slog.Logger) lifecycle.Component {
	return lifecycle.Func(func(ctx context.Context) error {
		addr := net.JoinHostPort(s.Host, strconv.Itoa(s.JWT.JWKSHTTPPort))
		srv := &http.Server{
			Addr:              addr,
			Handler:           token.JWKSHandler(token.PublicKey{Kid: signer.KID(), Key: signer.PublicKey()}),
			ReadHeaderTimeout: 5 * time.Second,
		}
		served := make(chan error, 1)
		go func() {
			logger.InfoContext(ctx, "identity JWKS HTTP server listening",
				"addr", addr, "path", token.JWKSPath, "kid", signer.KID())
			served <- srv.ListenAndServe()
		}()

		select {
		case err := <-served:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
		}
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			logger.WarnContext(ctx, "jwks server shutdown", "err", err)
		}
		return nil
	})
}
