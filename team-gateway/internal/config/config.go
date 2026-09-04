// Package config assembles the gateway's flat Settings from per-capability
// struct groups (same reflection loader + .env.example drift gate as the other
// services). The gateway holds no business config — only where to listen and
// which upstream services to route to.
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Settings is the whole configuration surface, grouped by capability.
type Settings struct {
	Runtime       Runtime
	Server        Server
	Upstream      Upstream
	Auth          Auth
	Edge          Edge
	Events        Events
	Observability Observability
}

type Runtime struct {
	Env      string `env:"ENV" default:"local"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`
	LogJSON  bool   `env:"LOG_JSON" default:"true"`
}

// Server is the edge HTTP listener (serves Connect: gRPC + gRPC-web + JSON).
type Server struct {
	Host          string  `env:"HTTP_HOST" default:"0.0.0.0"`
	Port          int     `env:"HTTP_PORT" default:"8080"`
	ShutdownGrace float64 `env:"SHUTDOWN_GRACE_SECONDS" default:"10"`
}

// Upstream is the routing table: which gRPC service backs each contract.
type Upstream struct {
	SearchAddr       string `env:"UPSTREAM_SEARCH_ADDR" default:"localhost:50052"`
	ListingAddr      string `env:"UPSTREAM_LISTING_ADDR" default:"localhost:50051"`
	IdentityAddr     string `env:"UPSTREAM_IDENTITY_ADDR" default:"localhost:50053"`
	EngagementAddr   string `env:"UPSTREAM_ENGAGEMENT_ADDR" default:"localhost:50054"`
	OrderAddr        string `env:"UPSTREAM_ORDER_ADDR" default:"localhost:50055"`
	PaymentAddr      string `env:"UPSTREAM_PAYMENT_ADDR" default:"localhost:50056"`
	ChatAddr         string `env:"UPSTREAM_CHAT_ADDR" default:"localhost:50057"`
	AIAddr           string `env:"UPSTREAM_AI_ADDR" default:"localhost:50060"`
	RecAddr          string `env:"UPSTREAM_RECOMMENDATION_ADDR" default:"localhost:50060"`
	PromotionAddr    string `env:"UPSTREAM_PROMOTION_ADDR" default:"localhost:50061"`
	NotificationAddr string `env:"UPSTREAM_NOTIFICATION_ADDR" default:"localhost:50058"`
	// AnalyticsAddr backs the AnalyticsQueryService (seller-dashboard read-model).
	AnalyticsAddr string `env:"UPSTREAM_ANALYTICS_ADDR" default:"team-analytics-svc:50059"`
	// The four new standalone services (each owns its DB).
	ReferralAddr     string  `env:"UPSTREAM_REFERRAL_ADDR" default:"team-referral-svc:50062"`
	VerificationAddr string  `env:"UPSTREAM_VERIFICATION_ADDR" default:"team-verification-svc:50064"`
	SharingAddr      string  `env:"UPSTREAM_SHARING_ADDR" default:"team-sharing-svc:50065"`
	AuditAddr        string  `env:"UPSTREAM_AUDIT_ADDR" default:"team-audit-svc:50066"`
	DialTimeout      float64 `env:"DIAL_TIMEOUT_SECONDS" default:"5"`
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
	// PrometheusURL is where the Admin Cockpit handler runs its fixed server-side
	// PromQL set (GET /api/v1/query). Read-only, never exposed to the browser.
	// Empty/unreachable → the handler degrades to zeroed values (never random).
	PrometheusURL string `env:"PROMETHEUS_URL" default:"http://prometheus:9090"`
}

// Events configures the edge telemetry producer (ADR-0002) that emits browsing
// TrackingEvents on the analytics topic. When KafkaEnabled is false the producer
// is a no-op, so the collector still accepts beacons without a broker. This is
// pure edge telemetry (validate → stamp → produce), not business config (Rule 2).
type Events struct {
	KafkaEnabled   bool   `env:"KAFKA_ENABLED" default:"false"`
	Brokers        string `env:"KAFKA_BROKERS" default:"localhost:9092"` // comma-separated
	AnalyticsTopic string `env:"KAFKA_ANALYTICS_TOPIC" default:"analytics.events"`
}

type Observability struct {
	Enabled      bool   `env:"OTEL_ENABLED" default:"false"`
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:""`
	ServiceName  string `env:"OTEL_SERVICE_NAME" default:"team-gateway"`
}

// LoadSettings reads the environment into Settings, applies defaults, validates.
func LoadSettings() (*Settings, error) {
	s := &Settings{}
	if err := bindGroups(reflect.ValueOf(s).Elem()); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Settings) Validate() error {
	if s.Server.Port <= 0 || s.Server.Port > 65535 {
		return fmt.Errorf("HTTP_PORT out of range: %d", s.Server.Port)
	}
	if strings.TrimSpace(s.Upstream.SearchAddr) == "" || strings.TrimSpace(s.Upstream.ListingAddr) == "" {
		return errors.New("UPSTREAM_SEARCH_ADDR and UPSTREAM_LISTING_ADDR are required")
	}
	if strings.TrimSpace(s.Upstream.IdentityAddr) == "" {
		return errors.New("UPSTREAM_IDENTITY_ADDR is required")
	}
	if strings.TrimSpace(s.Auth.JWKSURL) == "" {
		return errors.New("JWKS_URL is required (team-identity's /.well-known/jwks.json)")
	}
	if s.Auth.JWKSCacheTTLSeconds <= 0 {
		return fmt.Errorf("JWKS_CACHE_TTL must be > 0: %d", s.Auth.JWKSCacheTTLSeconds)
	}
	if s.Server.ShutdownGrace < 0 {
		return fmt.Errorf("SHUTDOWN_GRACE_SECONDS must be >= 0: %v", s.Server.ShutdownGrace)
	}
	return nil
}

// PublicScopesList returns the scopes granted to anonymous callers.
func (s *Settings) PublicScopesList() []string {
	return splitCSV(s.Auth.PublicScopes)
}

// CORSOriginsList returns the allowed CORS origins.
func (s *Settings) CORSOriginsList() []string {
	return splitCSV(s.Edge.CORSOrigins)
}

// KafkaBrokers splits the comma-separated KAFKA_BROKERS into seed addresses.
func (s *Settings) KafkaBrokers() []string {
	return splitCSV(s.Events.Brokers)
}

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

func (s *Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Runtime.Env))
	return e == "prod" || e == "production"
}

// DeclaredEnvKeys returns every env key declared by Settings, in struct order.
func DeclaredEnvKeys() []string {
	var keys []string
	t := reflect.TypeOf(Settings{})
	for i := 0; i < t.NumField(); i++ {
		gt := t.Field(i).Type
		for j := 0; j < gt.NumField(); j++ {
			if k := gt.Field(j).Tag.Get("env"); k != "" {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

func bindGroups(v reflect.Value) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		group := v.Field(i)
		gt := group.Type()
		for j := 0; j < gt.NumField(); j++ {
			f := gt.Field(j)
			key := f.Tag.Get("env")
			if key == "" {
				continue
			}
			raw := f.Tag.Get("default")
			if val, ok := os.LookupEnv(key); ok {
				raw = val
			}
			if err := setField(group.Field(j), raw); err != nil {
				return fmt.Errorf("config %s: %w", key, err)
			}
		}
	}
	return nil
}

func setField(fv reflect.Value, raw string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Float32, reflect.Float64:
		x, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			return err
		}
		fv.SetFloat(x)
	default:
		return fmt.Errorf("unsupported config field kind %s", fv.Kind())
	}
	return nil
}
