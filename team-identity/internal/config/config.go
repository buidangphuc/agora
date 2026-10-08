// Package config assembles team-identity's flat Settings from per-capability
// struct groups (same reflection loader + .env.example drift gate as the other
// Go services).
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Settings struct {
	Runtime       Runtime
	Server        Server
	Database      Database
	Events        Events
	Outbox        Outbox
	JWT           JWT
	Observability Observability
	SeedAdmin     SeedAdmin
	PasswordReset PasswordReset
}

type Runtime struct {
	Env      string `env:"ENV" default:"local"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`
	LogJSON  bool   `env:"LOG_JSON" default:"true"`
}

type Server struct {
	Host              string  `env:"GRPC_HOST" default:"0.0.0.0"`
	Port              int     `env:"GRPC_PORT" default:"50053"`
	ReflectionEnabled bool    `env:"GRPC_REFLECTION_ENABLED" default:"true"`
	ShutdownGrace     float64 `env:"SHUTDOWN_GRACE_SECONDS" default:"10"`
}

// Database is this service's OWN Postgres (identity_db). Rule 3.
type Database struct {
	Enabled  bool   `env:"DATABASE_ENABLED" default:"true"`
	URL      string `env:"DATABASE_URL" default:""`
	MaxConns int32  `env:"DB_MAX_CONNS" default:"10"`
}

// Events configures the Kafka producer behind the outbox relayer (ADR-0002):
// identity publishes SessionRevoked on IdentityTopic so the gateway can enforce
// revocations (ADR-0003 addendum). With KafkaEnabled=false nothing is produced.
type Events struct {
	KafkaEnabled  bool   `env:"KAFKA_ENABLED" default:"false"`
	Brokers       string `env:"KAFKA_BROKERS" default:"localhost:9092"` // comma-separated
	IdentityTopic string `env:"IDENTITY_EVENTS_TOPIC" default:"identity.events"`
}

// Outbox configures the transactional-outbox relayer (same names as team-domain).
// Revokes always record an outbox row in the same DB transaction; the relayer only
// runs when OUTBOX_ENABLED and KAFKA_ENABLED are both true.
type Outbox struct {
	Enabled          bool   `env:"OUTBOX_ENABLED" default:"true"`
	PollInterval     string `env:"OUTBOX_POLL_INTERVAL" default:"1s"` // Go duration, e.g. 1s, 500ms
	BatchSize        int    `env:"OUTBOX_BATCH_SIZE" default:"100"`
	ClaimLockSeconds int    `env:"OUTBOX_CLAIM_LOCK_SECONDS" default:"60"`
	MaxAttempts      int    `env:"OUTBOX_MAX_ATTEMPTS" default:"10"`
}

// JWT holds the RSA signing material identity mints RS256 tokens with, plus the
// port of the small HTTP listener that publishes the matching public key(s) as a
// JWKS the edge fetches (ADR-0006). The private key never leaves this service.
type JWT struct {
	PrivateKey   string `env:"JWT_PRIVATE_KEY" default:""` // PEM-encoded RSA private key
	KID          string `env:"JWT_KID" default:""`         // key id stamped in each token header
	JWKSHTTPPort int    `env:"JWKS_HTTP_PORT" default:"50063"`
	TTLSeconds   int    `env:"JWT_TTL_SECONDS" default:"3600"`
}

// MinSeedAdminPasswordLen is the shortest SEED_ADMIN_PASSWORD accepted.
const MinSeedAdminPasswordLen = 12

// SeedAdmin opts in to creating a first admin account at startup. It is OFF by
// default and the password has no default: the service never ships a built-in
// credential. Deployment manifests must not enable it (see platform-gitops check).
type SeedAdmin struct {
	Enabled  bool   `env:"SEED_ADMIN_ENABLED" default:"false"`
	Username string `env:"SEED_ADMIN_USERNAME" default:"admin"`
	Password string `env:"SEED_ADMIN_PASSWORD" default:""`
}

// PasswordReset controls RequestPasswordReset. There is no out-of-band delivery
// (email/SMS) yet, so the raw reset token is NEVER returned in the RPC response
// unless ExposeToken is on. Dev/e2e only: with it on, anyone who can call the RPC
// can take over any account. Refused when ENV is prod.
type PasswordReset struct {
	ExposeToken bool `env:"PASSWORD_RESET_EXPOSE_TOKEN" default:"false"`
}

type Observability struct {
	Enabled      bool   `env:"OTEL_ENABLED" default:"false"`
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:""`
	ServiceName  string `env:"OTEL_SERVICE_NAME" default:"team-identity"`
}

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
	if s.Database.Enabled && strings.TrimSpace(s.Database.URL) == "" {
		return errors.New("DATABASE_URL is required when DATABASE_ENABLED=true")
	}
	if strings.TrimSpace(s.JWT.PrivateKey) == "" {
		return errors.New("JWT_PRIVATE_KEY is required (PEM-encoded RSA private key)")
	}
	if strings.TrimSpace(s.JWT.KID) == "" {
		return errors.New("JWT_KID is required")
	}
	if s.SeedAdmin.Enabled {
		if strings.TrimSpace(s.SeedAdmin.Username) == "" {
			return errors.New("SEED_ADMIN_USERNAME must not be empty when SEED_ADMIN_ENABLED=true")
		}
		if len(s.SeedAdmin.Password) < MinSeedAdminPasswordLen {
			return fmt.Errorf("SEED_ADMIN_PASSWORD is required and must be at least %d characters when SEED_ADMIN_ENABLED=true", MinSeedAdminPasswordLen)
		}
	}
	if s.PasswordReset.ExposeToken && s.IsProd() {
		return errors.New("PASSWORD_RESET_EXPOSE_TOKEN must not be enabled when ENV is prod")
	}
	if s.Server.Port <= 0 || s.Server.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Server.Port)
	}
	if s.JWT.JWKSHTTPPort <= 0 || s.JWT.JWKSHTTPPort > 65535 {
		return fmt.Errorf("JWKS_HTTP_PORT out of range: %d", s.JWT.JWKSHTTPPort)
	}
	if s.JWT.TTLSeconds <= 0 {
		return fmt.Errorf("JWT_TTL_SECONDS must be > 0: %d", s.JWT.TTLSeconds)
	}
	return nil
}

// KafkaBrokers splits the comma-separated KAFKA_BROKERS into seed addresses.
func (s *Settings) KafkaBrokers() []string {
	parts := strings.Split(s.Events.Brokers, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// OutboxPollInterval parses OUTBOX_POLL_INTERVAL, defaulting to 1s when unset or
// unparseable.
func (s *Settings) OutboxPollInterval() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(s.Outbox.PollInterval))
	if err != nil || d <= 0 {
		return time.Second
	}
	return d
}

// hardenedSecretEnvs are the deployed environments that must not run on the
// committed development signing key; anything else ("local", "test", unknown) is
// non-strict.
var hardenedSecretEnvs = []string{"staging", "stage", "prod", "production"}

// RequiresHardenedSecrets reports whether ENV names a deployed environment
// (staging / production) that must not run on development secrets.
func (s *Settings) RequiresHardenedSecrets() bool {
	e := strings.ToLower(strings.TrimSpace(s.Runtime.Env))
	for _, strict := range hardenedSecretEnvs {
		if e == strict {
			return true
		}
	}
	return false
}

func (s *Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Runtime.Env))
	return e == "prod" || e == "production"
}

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
