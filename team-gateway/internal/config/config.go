// Package config declares the gateway's Settings on the SDK loader. The gateway
// holds no business config: where to listen, which upstreams to route to, how
// to verify tokens and how to harden the edge.
package config

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

const serviceName = "team-gateway"

// Settings is the whole configuration surface, grouped by capability.
type Settings struct {
	sdkconfig.Runtime
	GRPC
	Kafka    sdkconfig.Kafka
	OTel     sdkconfig.OTel
	Mock     sdkconfig.MockData
	Upstream Upstream
	Auth     Auth
	Edge     Edge
	Events   Events
	Identity IdentityEvents
}

// Upstream is the routing table: which gRPC service backs each contract.
type Upstream struct {
	SearchAddr       string `env:"UPSTREAM_SEARCH_ADDR" default:"localhost:50052" keepempty:"true"`
	ListingAddr      string `env:"UPSTREAM_LISTING_ADDR" default:"localhost:50051" keepempty:"true"`
	IdentityAddr     string `env:"UPSTREAM_IDENTITY_ADDR" default:"localhost:50053" keepempty:"true"`
	EngagementAddr   string `env:"UPSTREAM_ENGAGEMENT_ADDR" default:"localhost:50054" keepempty:"true"`
	OrderAddr        string `env:"UPSTREAM_ORDER_ADDR" default:"localhost:50055" keepempty:"true"`
	PaymentAddr      string `env:"UPSTREAM_PAYMENT_ADDR" default:"localhost:50056" keepempty:"true"`
	ChatAddr         string `env:"UPSTREAM_CHAT_ADDR" default:"localhost:50057" keepempty:"true"`
	AIAddr           string `env:"UPSTREAM_AI_ADDR" default:"localhost:50060" keepempty:"true"`
	RecAddr          string `env:"UPSTREAM_RECOMMENDATION_ADDR" default:"localhost:50060" keepempty:"true"`
	PromotionAddr    string `env:"UPSTREAM_PROMOTION_ADDR" default:"localhost:50061" keepempty:"true"`
	NotificationAddr string `env:"UPSTREAM_NOTIFICATION_ADDR" default:"localhost:50058" keepempty:"true"`
	// AnalyticsAddr backs the AnalyticsQueryService (seller-dashboard read-model).
	AnalyticsAddr string `env:"UPSTREAM_ANALYTICS_ADDR" default:"team-analytics-svc:50059" keepempty:"true"`
	// The four new standalone services (each owns its DB).
	ReferralAddr     string  `env:"UPSTREAM_REFERRAL_ADDR" default:"team-referral-svc:50062" keepempty:"true"`
	VerificationAddr string  `env:"UPSTREAM_VERIFICATION_ADDR" default:"team-verification-svc:50064" keepempty:"true"`
	SharingAddr      string  `env:"UPSTREAM_SHARING_ADDR" default:"team-sharing-svc:50065" keepempty:"true"`
	AuditAddr        string  `env:"UPSTREAM_AUDIT_ADDR" default:"team-audit-svc:50066" keepempty:"true"`
	DialTimeout      float64 `env:"DIAL_TIMEOUT_SECONDS" default:"2"`
}

// Auth is the edge verification config (ADR-0006). The gateway holds no signing
// secret: it verifies the RS256 JWT team-identity signs against the public keys
// it fetches from identity's JWKS (JWKSURL) and caches for JWKSCacheTTLSeconds.
// Anonymous callers get PublicScopes.
type Auth struct {
	JWKSURL             string `env:"JWKS_URL" default:""`
	JWKSCacheTTLSeconds int    `env:"JWKS_CACHE_TTL" default:"300"`
	PublicScopes        string `env:"PUBLIC_SCOPES" default:"listing.read,search:read"` // comma-separated
}

// Edge holds the cross-cutting edge hardening knobs: rate limiting, upstream
// call timeout + read retry, and CORS.
type Edge struct {
	RateLimitRPS    float64 `env:"RATE_LIMIT_RPS" default:"20"`
	RateLimitBurst  int     `env:"RATE_LIMIT_BURST" default:"40"`
	CallTimeoutSecs float64 `env:"CALL_TIMEOUT_SECONDS" default:"5"`
	RetryMax        int     `env:"RETRY_MAX" default:"2"`
	CORSOrigins     string  `env:"CORS_ORIGINS" default:"http://localhost:3000"` // comma-separated
	// TrackRateLimitRPS/Burst bound POST /api/track per visitor (user id, else
	// client IP) in a bucket separate from the RPC one: beacons are bursty.
	TrackRateLimitRPS   float64 `env:"TRACK_RATE_LIMIT_RPS" default:"5"`
	TrackRateLimitBurst int     `env:"TRACK_RATE_LIMIT_BURST" default:"20"`
	// AICallTimeoutSecs is the single-attempt deadline of the AI generation RPCs
	// (MagicListing, ChatCopilot, SummarizeReviews, ShoppingAssistant); never retried.
	AICallTimeoutSecs float64 `env:"AI_CALL_TIMEOUT_SECONDS" default:"30"`
	// StreamMaxRequestBytes caps one StreamChat request message.
	StreamMaxRequestBytes int `env:"STREAM_MAX_REQUEST_BYTES" default:"16384"`
	// StreamRevocationCheckSecs is how often an open stream re-checks its session
	// against the in-memory revocation denylist; a revoked session ends the stream
	// with unauthenticated within this interval. (Token expiry ends it on time.)
	StreamRevocationCheckSecs float64 `env:"STREAM_REVOCATION_CHECK_SECONDS" default:"5"`
	// PrometheusURL is where the Admin Cockpit handler runs its fixed server-side
	// PromQL set (GET /api/v1/query). Read-only, never exposed to the browser.
	// Empty/unreachable → the handler degrades to zeroed values (never random).
	PrometheusURL string `env:"PROMETHEUS_URL" default:"http://prometheus:9090" keepempty:"true"`
	// JaegerQueryURL is the Jaeger query API the cockpit asks for recent
	// team-gateway traces (fixed query, 2 s timeout). Server-side only. Empty or
	// unreachable → recent_traces is empty.
	JaegerQueryURL string `env:"JAEGER_QUERY_URL" default:"http://jaeger:16686" keepempty:"true"`
	// JaegerUIURL is the browser-facing Jaeger UI base used to build trace links.
	JaegerUIURL string `env:"JAEGER_UI_URL" default:"http://localhost:16686"`
	// TrustedProxies lists the peers whose X-Forwarded-For the edge believes when
	// computing the client IP forwarded to services as x-client-ip: comma-separated
	// CIDRs, bare IPs or hostnames (resolved periodically, e.g. the frontend's
	// service name). Empty (default) trusts nobody: the socket peer address is used.
	TrustedProxies string `env:"TRUSTED_PROXIES" default:""`
}

// GRPC is the edge listener. It mirrors sdkconfig.GRPC but leaves GRPC_PORT to
// Validate, so an unset port is reported together with, and after, the
// environment-dependent refusals (MOCK_DATA, reflection).
type GRPC struct {
	Host              string        `env:"GRPC_HOST" default:"0.0.0.0"`
	Port              int           `env:"GRPC_PORT"`
	ShutdownGrace     time.Duration `env:"SHUTDOWN_GRACE" default:"10s"`
	DrainDelay        time.Duration `env:"SHUTDOWN_DRAIN_DELAY" default:"0s"`
	ReflectionEnabled bool          `env:"GRPC_REFLECTION_ENABLED" default:"false"`
}

func (g *GRPC) ResolveFor(rt sdkconfig.Runtime) error {
	if g.ReflectionEnabled && !rt.IsLocal() {
		return errors.New("GRPC_REFLECTION_ENABLED is only allowed when ENV is local or test")
	}
	return nil
}

func (g GRPC) Validate() error {
	var errs []error
	if g.Port <= 0 || g.Port > 65535 {
		errs = append(errs, fmt.Errorf("GRPC_PORT is required and must be a port number: %d", g.Port))
	}
	if g.ShutdownGrace < 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_GRACE must be >= 0: %v", g.ShutdownGrace))
	}
	if g.DrainDelay < 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_DRAIN_DELAY must be >= 0: %v", g.DrainDelay))
	}
	return errors.Join(errs...)
}

// Addr returns the host:port the edge listens on.
func (g GRPC) Addr() string { return net.JoinHostPort(g.Host, strconv.Itoa(g.Port)) }

// Events configures the edge telemetry producer (ADR-0002) that emits browsing
// TrackingEvents on the analytics topic.
type Events struct {
	AnalyticsTopic      string        `env:"KAFKA_ANALYTICS_TOPIC" default:"analytics.events"`
	TrackPublishTimeout time.Duration `env:"TRACK_PUBLISH_TIMEOUT" default:"2s"`
}

// IdentityEvents configures the revocation consumer (ADR-0003 addendum).
type IdentityEvents struct {
	Topic string `env:"IDENTITY_EVENTS_TOPIC" default:"identity.events"`
}

// Load reads the environment into Settings, applies defaults and validates.
func Load() (Settings, error) {
	s, err := sdkconfig.Load[Settings]()
	if err != nil {
		return s, err
	}
	if s.OTel.ServiceName == "" {
		s.OTel.ServiceName = serviceName
	}
	return s, nil
}

// Validate checks the gateway-specific bounds.
func (s Settings) Validate() error {
	var errs []error
	for _, v := range []interface{ Validate() error }{s.Runtime, s.GRPC, s.Upstream, s.Auth, s.Edge, s.Events} {
		if err := v.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (u Upstream) Validate() error {
	var errs []error
	for key, addr := range map[string]string{
		"UPSTREAM_SEARCH_ADDR": u.SearchAddr, "UPSTREAM_LISTING_ADDR": u.ListingAddr,
		"UPSTREAM_IDENTITY_ADDR": u.IdentityAddr, "UPSTREAM_ENGAGEMENT_ADDR": u.EngagementAddr,
		"UPSTREAM_ORDER_ADDR": u.OrderAddr, "UPSTREAM_PAYMENT_ADDR": u.PaymentAddr,
		"UPSTREAM_CHAT_ADDR": u.ChatAddr, "UPSTREAM_AI_ADDR": u.AIAddr,
		"UPSTREAM_RECOMMENDATION_ADDR": u.RecAddr, "UPSTREAM_PROMOTION_ADDR": u.PromotionAddr,
		"UPSTREAM_NOTIFICATION_ADDR": u.NotificationAddr, "UPSTREAM_ANALYTICS_ADDR": u.AnalyticsAddr,
		"UPSTREAM_REFERRAL_ADDR": u.ReferralAddr, "UPSTREAM_VERIFICATION_ADDR": u.VerificationAddr,
		"UPSTREAM_SHARING_ADDR": u.SharingAddr, "UPSTREAM_AUDIT_ADDR": u.AuditAddr,
	} {
		if strings.TrimSpace(addr) == "" {
			errs = append(errs, fmt.Errorf("%s is required", key))
		}
	}
	return errors.Join(errs...)
}

func (a Auth) Validate() error {
	var errs []error
	if strings.TrimSpace(a.JWKSURL) == "" {
		errs = append(errs, errors.New("JWKS_URL is required (team-identity's /.well-known/jwks.json)"))
	}
	if a.JWKSCacheTTLSeconds <= 0 {
		errs = append(errs, fmt.Errorf("JWKS_CACHE_TTL must be > 0: %d", a.JWKSCacheTTLSeconds))
	}
	return errors.Join(errs...)
}

func (e Edge) Validate() error {
	if e.RetryMax < 0 {
		return fmt.Errorf("RETRY_MAX must be >= 0: %d", e.RetryMax)
	}
	return nil
}

func (e Events) Validate() error {
	if e.TrackPublishTimeout <= 0 {
		return fmt.Errorf("TRACK_PUBLISH_TIMEOUT must be a positive duration: %s", e.TrackPublishTimeout)
	}
	return nil
}

// PublicScopesList returns the scopes granted to anonymous callers.
func (s Settings) PublicScopesList() []string { return splitCSV(s.Auth.PublicScopes) }

// CORSOriginsList returns the allowed CORS origins.
func (s Settings) CORSOriginsList() []string { return splitCSV(s.Edge.CORSOrigins) }

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
