// Command gateway is the edge: it serves the platform contracts over Connect
// (gRPC + gRPC-web + JSON) so browsers/native apps can reach them, and forwards
// every call to the upstream gRPC services. It holds no business logic and no
// DB (Rules 1–3): it only routes.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/buidangphuc/team-gateway/internal/config"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/events"
	"github.com/buidangphuc/team-gateway/internal/observability"
	"github.com/buidangphuc/team-gateway/internal/token"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway exited with error", slog.Any("err", err))
		os.Exit(1)
	}
}

func run() error {
	settings, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	logger := observability.NewLogger(settings)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observability.InitTracer(ctx, settings)
	if err != nil {
		return fmt.Errorf("init tracer: %w", err)
	}
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(sctx); err != nil {
			logger.Warn("tracer shutdown", slog.Any("err", err))
		}
	}()

	// Meter provider (ADR-0004): same swappable, OFF-by-default shape as the
	// tracer. When enabled, the otelgrpc client instrumentation on the upstream
	// dials emits per-service RED metrics through the same OTLP → collector path.
	shutdownMetrics, err := observability.InitMeter(ctx, settings)
	if err != nil {
		return fmt.Errorf("init meter: %w", err)
	}
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownMetrics(sctx); err != nil {
			logger.Warn("meter shutdown", slog.Any("err", err))
		}
	}()

	clients, err := upstream.Dial(
		settings.Upstream.SearchAddr,
		settings.Upstream.ListingAddr,
		settings.Upstream.IdentityAddr,
		settings.Upstream.EngagementAddr,
		settings.Upstream.OrderAddr,
		settings.Upstream.PaymentAddr,
		settings.Upstream.ChatAddr,
		settings.Upstream.AIAddr,
		settings.Upstream.RecAddr,
		settings.Upstream.PromotionAddr,
		settings.Upstream.NotificationAddr,
		settings.Upstream.AnalyticsAddr,
		settings.Upstream.ReferralAddr,
		settings.Upstream.VerificationAddr,
		settings.Upstream.SharingAddr,
		settings.Upstream.AuditAddr,
	)
	if err != nil {
		return fmt.Errorf("dial upstreams: %w", err)
	}
	defer clients.Close()

	// The edge verifies RS256 JWTs against team-identity's JWKS (ADR-0006),
	// resolves the Principal, rate-limits, and adds an upstream deadline + read
	// retry. The gateway holds no signing secret.
	verifier := token.NewVerifier(
		settings.Auth.JWKSURL,
		time.Duration(settings.Auth.JWKSCacheTTLSeconds)*time.Second,
	)
	// Warm the JWKS cache; do NOT hard-fail boot if identity is briefly
	// unreachable — anonymous read paths still work and the cache fills on first
	// use / TTL refresh.
	func() {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := verifier.Prime(pctx); err != nil {
			logger.Warn("JWKS prime failed at startup (will retry lazily)",
				slog.String("jwks_url", settings.Auth.JWKSURL), slog.Any("err", err))
		}
	}()
	e := edge.NewEdge(
		verifier,
		settings.PublicScopesList(),
		time.Duration(settings.Edge.CallTimeoutSecs*float64(time.Second)),
		settings.Edge.RetryMax,
		settings.Edge.RateLimitRPS,
		settings.Edge.RateLimitBurst,
	)

	// Edge telemetry producer (ADR-0002): a real Kafka producer when enabled,
	// else a no-op so beacons are accepted without a broker. Pure edge concern
	// (validate → stamp principal → produce); no business logic (Rule 2).
	analytics, err := newAnalyticsPublisher(settings, logger)
	if err != nil {
		return fmt.Errorf("analytics publisher: %w", err)
	}
	defer analytics.Close()

	// CORS lets browsers call the gateway directly (gRPC-web / Connect / JSON).
	corsMW := cors.New(cors.Options{
		AllowedOrigins:   settings.CORSOriginsList(),
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms", "Authorization", "X-Request-Id"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
	})

	// h2c lets gRPC clients speak HTTP/2 cleartext; the same handler serves
	// gRPC-web and JSON over HTTP/1.1 for browsers and native apps.
	handler := h2c.NewHandler(corsMW.Handler(edge.NewMux(clients, e, analytics, settings.Edge.PrometheusURL, logger)), &http2.Server{})
	addr := net.JoinHostPort(settings.Server.Host, strconv.Itoa(settings.Server.Port))
	srv := &http.Server{Addr: addr, Handler: handler}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("gateway listening (Connect: gRPC + gRPC-web + JSON)",
			slog.String("addr", addr),
			slog.String("upstream_search", settings.Upstream.SearchAddr),
			slog.String("upstream_listing", settings.Upstream.ListingAddr),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining")
		sctx, cancel := context.WithTimeout(context.Background(),
			time.Duration(settings.Server.ShutdownGrace*float64(time.Second)))
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			srv.Close()
		}
		return <-serveErr
	}
}

// newAnalyticsPublisher builds the edge telemetry producer: a Kafka producer
// when KAFKA_ENABLED=true, otherwise a no-op that accepts beacons and emits
// nothing (telemetry is best-effort and must never depend on a broker).
func newAnalyticsPublisher(settings *config.Settings, logger *slog.Logger) (events.AnalyticsPublisher, error) {
	if !settings.Events.KafkaEnabled {
		logger.Info("analytics producer disabled (KAFKA_ENABLED=false); /api/track is a no-op")
		return events.NoopPublisher{}, nil
	}
	pub, err := events.NewKafkaPublisher(settings.KafkaBrokers(), settings.Events.AnalyticsTopic)
	if err != nil {
		return nil, err
	}
	logger.Info("analytics producer enabled",
		slog.Any("brokers", settings.KafkaBrokers()),
		slog.String("topic", settings.Events.AnalyticsTopic),
	)
	return pub, nil
}
