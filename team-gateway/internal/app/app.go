// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them. Read top to bottom to see what runs.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c" //nolint:staticcheck // SA1019: see the call below

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"

	"github.com/buidangphuc/team-gateway/internal/config"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/events"
	"github.com/buidangphuc/team-gateway/internal/revocation"
	"github.com/buidangphuc/team-gateway/internal/token"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const service = "team-gateway"

const (
	// trustedProxyRefresh is how often hostnames in TRUSTED_PROXIES are re-resolved.
	trustedProxyRefresh = 30 * time.Second
	// jwksPrimeTimeout bounds the JWKS fetch at boot.
	jwksPrimeTimeout = 5 * time.Second
	// shutdownHeadroom is added to SHUTDOWN_GRACE_SECONDS for lifecycle, so the
	// HTTP server's own drain deadline (and its forced Close) fires first and a slow
	// drain ends as before: closed connections, exit code 0.
	shutdownHeadroom = 2 * time.Second
)

// Options are the seams tests use; Run passes the zero value.
type Options struct {
	// Wrap decorates the edge handler, for example to add a slow route.
	Wrap func(http.Handler) http.Handler
	// LogOut receives the service log. Default: os.Stdout.
	LogOut io.Writer
}

// Run wires the gateway and runs it until ctx ends (main cancels it on
// SIGINT/SIGTERM) or a component fails. Build order: logger, telemetry,
// upstream clients, verifier, edge, analytics publisher, revocation consumer,
// HTTP server. Everything opened goes on the stack; closers run in reverse order
// on shutdown and on an early failure.
//
// Shutdown order: the HTTP server stops accepting and drains in-flight requests
// (up to SHUTDOWN_GRACE_SECONDS), then the revocation consumer and the
// trusted-proxy refresher stop, then the analytics publisher, the upstream
// connections and the telemetry providers close.
func Run(ctx context.Context, s config.Settings) error {
	return run(ctx, s, Options{})
}

func run(ctx context.Context, s config.Settings, o Options) error {
	out := o.LogOut
	if out == nil {
		out = os.Stdout
	}
	logger := logx.NewTo(out, s.Runtime, service)
	slog.SetDefault(logger)

	var stack lifecycle.Stack
	fail := func(err error) error { return errors.Join(err, stack.Close(ctx)) }

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	stack.Push(shutdownOTel)

	clients, err := upstream.Dial(
		time.Duration(s.Upstream.DialTimeout*float64(time.Second)),
		s.Upstream.SearchAddr,
		s.Upstream.ListingAddr,
		s.Upstream.IdentityAddr,
		s.Upstream.EngagementAddr,
		s.Upstream.OrderAddr,
		s.Upstream.PaymentAddr,
		s.Upstream.ChatAddr,
		s.Upstream.AIAddr,
		s.Upstream.RecAddr,
		s.Upstream.PromotionAddr,
		s.Upstream.NotificationAddr,
		s.Upstream.AnalyticsAddr,
		s.Upstream.ReferralAddr,
		s.Upstream.VerificationAddr,
		s.Upstream.SharingAddr,
		s.Upstream.AuditAddr,
	)
	if err != nil {
		return fail(fmt.Errorf("dial upstreams: %w", err))
	}
	stack.Push(func(context.Context) error { clients.Close(); return nil })

	// The edge verifies RS256 JWTs against team-identity's JWKS (ADR-0006),
	// resolves the Principal, rate-limits, and adds an upstream deadline + read
	// retry. The gateway holds no signing secret.
	verifier := token.NewVerifier(
		s.Auth.JWKSURL,
		time.Duration(s.Auth.JWKSCacheTTLSeconds)*time.Second,
	)
	// Warm the JWKS cache; do NOT hard-fail boot if identity is briefly
	// unreachable: anonymous read paths still work and the cache fills on first
	// use / TTL refresh.
	primeJWKS(ctx, verifier, s.Auth.JWKSURL, logger)

	e := edge.NewEdge(
		verifier,
		s.PublicScopesList(),
		time.Duration(s.Edge.CallTimeoutSecs*float64(time.Second)),
		s.Edge.RetryMax,
		s.Edge.RateLimitRPS,
		s.Edge.RateLimitBurst,
	).
		WithTrackLimit(s.Edge.TrackRateLimitRPS, s.Edge.TrackRateLimitBurst).
		WithAICallTimeout(time.Duration(s.Edge.AICallTimeoutSecs * float64(time.Second))).
		WithStreamMaxBytes(s.Edge.StreamMaxRequestBytes).
		WithStreamRevocationCheck(time.Duration(s.Edge.StreamRevocationCheckSecs * float64(time.Second))).
		WithReflection(s.Edge.ReflectionEnabled)

	// Trusted client context: x-client-ip is the socket peer, or the X-Forwarded-For
	// client only when the peer is listed in TRUSTED_PROXIES (ADR-0003 addendum).
	trusted, err := edge.ParseTrustedProxies(s.Edge.TrustedProxies, nil)
	if err != nil {
		return fail(err)
	}
	e.WithTrustedProxies(trusted)
	if s.Edge.TrustedProxies == "" {
		logger.InfoContext(ctx, "TRUSTED_PROXIES empty: X-Forwarded-For is ignored; client IP is the socket peer")
	}

	// Edge telemetry producer (ADR-0002): wraps a beacon in an EventEnvelope and
	// produces it. Pure edge concern (validate, stamp principal, produce); no
	// business logic (Rule 2). The Kafka client connects lazily, so a broker that
	// is down never stops the gateway starting.
	analytics, err := events.NewKafkaPublisher(s.Kafka, s.Events.AnalyticsTopic)
	if err != nil {
		return fail(fmt.Errorf("analytics publisher: %w", err))
	}
	stack.Push(func(context.Context) error { analytics.Close(); return nil })
	logger.InfoContext(ctx, "analytics producer started",
		"brokers", s.BrokerList(), "topic", s.Events.AnalyticsTopic)

	// Session revocation (ADR-0003 addendum): identity.events -> in-memory denylist
	// of revoked session ids, checked by the edge after local JWT verification.
	// Best-effort by design: the consumer runs in the background and Kafka being
	// down never stops the gateway starting (revocations just are not enforced,
	// logged, and visible on gateway_revocation_consumer_up).
	list := revocation.NewDenylist()
	consumer := revocation.NewConsumer(s.Kafka, s.Identity.Topic, list, logger)
	e.WithRevocations(list)
	if err := revocation.RegisterMetrics(consumer); err != nil {
		logger.WarnContext(ctx, "revocation metrics not registered", "err", err)
	}

	// CORS lets browsers call the gateway directly (gRPC-web / Connect / JSON).
	corsMW := cors.New(cors.Options{
		AllowedOrigins:   s.CORSOriginsList(),
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms", "Authorization", "X-Request-Id", "Idempotency-Key"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
	})
	var mux http.Handler = edge.NewMux(clients, e, analytics, edge.CockpitConfig{
		PrometheusURL:  s.Edge.PrometheusURL,
		JaegerQueryURL: s.Edge.JaegerQueryURL,
		JaegerUIURL:    s.Edge.JaegerUIURL,
	}, logger)
	if o.Wrap != nil {
		mux = o.Wrap(mux)
	}
	// h2c lets gRPC clients speak HTTP/2 cleartext; the same handler serves
	// gRPC-web and JSON over HTTP/1.1 for browsers and native apps.
	addr := net.JoinHostPort(s.Server.Host, strconv.Itoa(s.Server.Port))
	//nolint:staticcheck // SA1019: h2c keeps HTTP/2 cleartext (prior knowledge and Upgrade) exactly as before; http.Protocols has no Upgrade path.
	handler := h2c.NewHandler(corsMW.Handler(mux), &http2.Server{})
	//nolint:gosec // G112: no ReadHeaderTimeout, as before the migration; adding one changes edge behaviour for slow clients and long streams.
	srv := &http.Server{Addr: addr, Handler: handler}

	grace := time.Duration(s.Server.ShutdownGrace * float64(time.Second))
	logger.InfoContext(ctx, "gateway listening (Connect: gRPC + gRPC-web + JSON)",
		"addr", addr,
		"upstream_search", s.Upstream.SearchAddr,
		"upstream_listing", s.Upstream.ListingAddr,
	)

	// The HTTP server is the primary component: the revocation consumer and the
	// trusted-proxy refresher keep running until it has drained its requests.
	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: grace + shutdownHeadroom,
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, lifecycle.StopAfter(
		serve(srv, grace, logger),
		lifecycle.Func(func(ctx context.Context) error { consumer.Run(ctx); return nil }),
		lifecycle.Func(func(ctx context.Context) error { trusted.Run(ctx, trustedProxyRefresh); return nil }),
	))
}

// primeJWKS fetches the signing keys once so the first request does not pay for it.
func primeJWKS(ctx context.Context, v *token.Verifier, url string, logger *slog.Logger) {
	pctx, cancel := context.WithTimeout(ctx, jwksPrimeTimeout)
	defer cancel()
	if err := v.Prime(pctx); err != nil {
		logger.WarnContext(ctx, "JWKS prime failed at startup (will retry lazily)", "jwks_url", url, "err", err)
	}
}

// serve runs srv until ctx ends, then stops accepting connections and drains
// in-flight requests for up to grace before closing what is left.
func serve(srv *http.Server, grace time.Duration, logger *slog.Logger) lifecycle.Component {
	return lifecycle.Func(func(ctx context.Context) error {
		errCh := make(chan error, 1)
		go func() { errCh <- srv.ListenAndServe() }()

		select {
		case err := <-errCh:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
		}
		logger.InfoContext(ctx, "shutdown signal received, draining")
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), grace)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			_ = srv.Close()
		}
		if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
}
