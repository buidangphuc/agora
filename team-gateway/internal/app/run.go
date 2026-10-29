package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/rs/cors"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/mockdata"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/team-gateway/internal/config"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/events"
	"github.com/buidangphuc/team-gateway/internal/revocation"
	"github.com/buidangphuc/team-gateway/internal/token"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const (
	service       = "team-gateway"
	primeInterval = 5 * time.Second
	primeTimeout  = 5 * time.Second
)

func Run(ctx context.Context, s config.Settings) error {
	logger := logx.New(s.Runtime, service)
	slog.SetDefault(logger)

	var stack lifecycle.Stack
	fail := func(err error) error { return errors.Join(err, stack.Close(ctx)) }

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	stack.Push(shutdownOTel)

	mocks, err := mockdata.New(s.Mock)
	if err != nil {
		return fail(fmt.Errorf("mock data: %w", err))
	}
	mocks.LogActive(logger)

	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		return fail(fmt.Errorf("init health: %w", err))
	}

	verifier := token.NewVerifier(s.Auth.JWKSURL, time.Duration(s.Auth.JWKSCacheTTLSeconds)*time.Second)
	health.Register("jwks", verifier.Check)

	producer, err := kafkax.NewProducer(s.Kafka)
	if err != nil {
		return fail(fmt.Errorf("kafka producer: %w", err))
	}
	stack.Push(func(context.Context) error { producer.Close(); return nil })
	analytics := events.NewKafkaPublisher(producer, s.Events.AnalyticsTopic)

	clients, err := upstream.Dial(
		time.Duration(s.Upstream.DialTimeout*float64(time.Second)),
		[]upstream.Addr{
			{Name: "search", Address: s.Upstream.SearchAddr},
			{Name: "listing", Address: s.Upstream.ListingAddr},
			{Name: "identity", Address: s.Upstream.IdentityAddr},
			{Name: "engagement", Address: s.Upstream.EngagementAddr},
			{Name: "order", Address: s.Upstream.OrderAddr},
			{Name: "payment", Address: s.Upstream.PaymentAddr},
			{Name: "chat", Address: s.Upstream.ChatAddr},
			{Name: "ai", Address: s.Upstream.AIAddr},
			{Name: "recommendation", Address: s.Upstream.RecAddr},
			{Name: "promotion", Address: s.Upstream.PromotionAddr},
			{Name: "notification", Address: s.Upstream.NotificationAddr},
			{Name: "analytics", Address: s.Upstream.AnalyticsAddr},
			{Name: "referral", Address: s.Upstream.ReferralAddr},
			{Name: "verification", Address: s.Upstream.VerificationAddr},
			{Name: "sharing", Address: s.Upstream.SharingAddr},
			{Name: "audit", Address: s.Upstream.AuditAddr},
		},
	)
	if err != nil {
		return fail(fmt.Errorf("dial upstreams: %w", err))
	}
	stack.Push(func(context.Context) error { clients.Close(); return nil })

	e := edge.NewEdge(
		verifier,
		s.PublicScopesList(),
		time.Duration(s.Edge.CallTimeoutSecs*float64(time.Second)),
		s.Edge.RetryMax,
		s.Edge.RateLimitRPS,
		s.Edge.RateLimitBurst,
	).
		WithTrackLimit(s.Edge.TrackRateLimitRPS, s.Edge.TrackRateLimitBurst).
		WithTrackPublishTimeout(s.Events.TrackPublishTimeout).
		WithAICallTimeout(time.Duration(s.Edge.AICallTimeoutSecs * float64(time.Second))).
		WithStreamMaxBytes(s.Edge.StreamMaxRequestBytes).
		WithStreamRevocationCheck(time.Duration(s.Edge.StreamRevocationCheckSecs * float64(time.Second))).
		WithReflection(s.ReflectionEnabled)

	trusted, err := edge.ParseTrustedProxies(s.Edge.TrustedProxies, nil)
	if err != nil {
		return fail(err)
	}
	e.WithTrustedProxies(trusted)
	if s.Edge.TrustedProxies == "" {
		logger.Info("TRUSTED_PROXIES empty: X-Forwarded-For is ignored; client IP is the socket peer")
	}

	list := revocation.NewDenylist()
	consumer, err := revocation.NewConsumer(s.Kafka, s.Identity.Topic, list, logger)
	if err != nil {
		return fail(err)
	}
	e.WithRevocations(list)
	if err := revocation.RegisterMetrics(consumer); err != nil {
		logger.Warn("revocation metrics not registered", "err", err)
	}
	health.Register("revocation", func(context.Context) error {
		if !consumer.CaughtUp() {
			return errors.New("revocation: denylist has not caught up")
		}
		return nil
	})

	corsMW := cors.New(cors.Options{
		AllowedOrigins:   s.CORSOriginsList(),
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms", "Authorization", "X-Request-Id", "Idempotency-Key"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
	})
	edgeMux := edge.NewMux(clients, e, analytics, edge.CockpitConfig{
		PrometheusURL:  s.Edge.PrometheusURL,
		JaegerQueryURL: s.Edge.JaegerQueryURL,
		JaegerUIURL:    s.Edge.JaegerUIURL,
	}, logger)

	mux := http.NewServeMux()
	mux.Handle(healthHandler(health.Server()))
	mux.Handle("/", corsMW.Handler(edgeMux))

	srv := &http.Server{Addr: s.Addr(), Handler: mux, Protocols: new(http.Protocols)}
	srv.Protocols.SetHTTP1(true)
	srv.Protocols.SetUnencryptedHTTP2(true)

	opts := lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}
	logger.Info("gateway listening (Connect: gRPC + gRPC-web + JSON)", "addr", srv.Addr)
	return lifecycle.Run(ctx, opts,
		lifecycle.HTTP(srv, s.ShutdownGrace*4/5),
		lifecycle.Func(func(ctx context.Context) error { trusted.Run(ctx, 30*time.Second); <-ctx.Done(); return nil }),
		lifecycle.Func(consumer.Run),
		lifecycle.Func(health.Run),
		lifecycle.Func(func(ctx context.Context) error { return primeJWKS(ctx, verifier, logger) }),
	)
}

func primeJWKS(ctx context.Context, v *token.Verifier, logger *slog.Logger) error {
	t := time.NewTicker(primeInterval)
	defer t.Stop()
	for {
		pctx, cancel := context.WithTimeout(ctx, primeTimeout)
		err := v.Prime(pctx)
		cancel()
		if err == nil {
			<-ctx.Done()
			return nil
		}
		logger.Warn("JWKS prime failed (retrying in background)", "err", err)
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}
