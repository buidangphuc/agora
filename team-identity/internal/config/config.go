// Package config holds team-identity's settings: the go-sdk groups it uses
// (Runtime, GRPC, Postgres, OTel, Kafka, Outbox) plus the service-specific groups
// below, bound from flat env keys (GRPC_PORT, DATABASE_URL, ...) with
// sdkconfig.Load. The `env`/`default` struct tags are the single source of truth
// for both loading and the .env.example drift test (config_test.go).
package config

import (
	"errors"
	"fmt"
	"strings"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// Settings is the whole configuration surface. Runtime and GRPC are embedded SDK
// groups; the others are named because their field names collide (OTel and
// Postgres both had Enabled) or read better qualified.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres      sdkconfig.Postgres
	OTel          sdkconfig.OTel
	Kafka         sdkconfig.Kafka
	Outbox        sdkconfig.Outbox
	Events        Events
	JWT           JWT
	SeedAdmin     SeedAdmin
	PasswordReset PasswordReset
}

// Events names the topic identity publishes SessionRevoked on (ADR-0002) so the
// gateway can enforce revocations (ADR-0003 addendum). The Kafka producer and the
// outbox relayer are always on.
type Events struct {
	IdentityTopic string `env:"IDENTITY_EVENTS_TOPIC" default:"identity.events"`
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

// Load reads Settings from the environment, applies defaults and runs Validate.
// It is the single entry point used by cmd/server.
func Load() (Settings, error) {
	return sdkconfig.Load[Settings]()
}

// Validate enforces the service's own invariants and prod-safety. sdkconfig.Load
// validates every SDK group (Runtime, GRPC, Postgres, ...) by itself and then
// calls this.
func (s Settings) Validate() error {
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
	if s.PasswordReset.ExposeToken && s.IsStrict() {
		return fmt.Errorf("PASSWORD_RESET_EXPOSE_TOKEN must not be enabled when ENV is %q (staging or production)", s.Env)
	}
	if s.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Port)
	}
	if s.JWT.JWKSHTTPPort <= 0 || s.JWT.JWKSHTTPPort > 65535 {
		return fmt.Errorf("JWKS_HTTP_PORT out of range: %d", s.JWT.JWKSHTTPPort)
	}
	if s.JWT.TTLSeconds <= 0 {
		return fmt.Errorf("JWT_TTL_SECONDS must be > 0: %d", s.JWT.TTLSeconds)
	}
	return nil
}

// IsProd reports whether ENV names production (case-insensitive).
func (s Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Env))
	return e == "prod" || e == "production"
}
