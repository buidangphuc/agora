package config

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Runtime holds settings every service shares. ENV has no default: a service
// that forgets it must fail at load, not run as local.
type Runtime struct {
	Env      string `env:"ENV,required"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`
	LogJSON  bool   `env:"LOG_JSON" default:"true"`
}

// Validate rejects an empty ENV. An empty ENV= counts as set, so "required"
// alone lets it through.
func (r Runtime) Validate() error {
	if strings.TrimSpace(r.Env) == "" {
		return errors.New("ENV is required (local, test, staging or prod)")
	}
	return nil
}

// IsLocal reports whether the service runs in a developer or test environment:
// true only for an explicit ENV of "local" or "test".
func (r Runtime) IsLocal() bool { return r.Env == "local" || r.Env == "test" }

// IsStrict reports whether the service runs in a deployed environment where
// silent fallbacks are not allowed: staging, stage, prod or production, matched
// case-insensitively.
func (r Runtime) IsStrict() bool {
	switch strings.ToLower(strings.TrimSpace(r.Env)) {
	case "staging", "stage", "prod", "production":
		return true
	}
	return false
}

// GRPC holds the gRPC listener settings.
type GRPC struct {
	Host string `env:"GRPC_HOST" default:"0.0.0.0"`
	Port int    `env:"GRPC_PORT,required"`
	// ShutdownGrace is the graceful shutdown deadline, a Go duration such as "10s".
	ShutdownGrace time.Duration `env:"SHUTDOWN_GRACE" default:"10s"`
	// DrainDelay is how long to keep serving after health reports NOT_SERVING,
	// so probes and load balancers notice before the server stops.
	DrainDelay time.Duration `env:"SHUTDOWN_DRAIN_DELAY" default:"0s"`
}

// Validate rejects a missing or non-positive port. An empty GRPC_PORT= parses
// as 0, so the "required" tag alone lets it through. A Settings type embedding
// GRPC gets this hook promoted; one embedding several groups delegates to it.
func (g GRPC) Validate() error {
	if g.Port <= 0 {
		return errors.New("GRPC_PORT must be a positive port number")
	}
	return nil
}

// Addr returns the host:port the server listens on.
func (g GRPC) Addr() string { return net.JoinHostPort(g.Host, strconv.Itoa(g.Port)) }

// Postgres holds the database settings.
type Postgres struct {
	URL      string `env:"DATABASE_URL"`
	MaxConns int32  `env:"DB_MAX_CONNS" default:"10"`
}

// Validate rejects an empty URL: the database is always on, with no enable flag.
func (p Postgres) Validate() error {
	if p.URL == "" {
		return errors.New("DATABASE_URL is required")
	}
	return nil
}

// OTel holds the telemetry settings.
type OTel struct {
	Enabled     bool   `env:"OTEL_ENABLED" default:"false"`
	Endpoint    string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	ServiceName string `env:"OTEL_SERVICE_NAME"`
}

// Kafka holds the broker settings. Kafka has no enable flag: a service that
// imports kafkax needs it. KAFKA_BROKERS has no tag default: Load fills
// localhost:9092 for a local ENV and rejects an unset or empty value when
// Runtime.IsStrict (see Load).
type Kafka struct {
	Brokers string `env:"KAFKA_BROKERS"`
}

// ResolveFor is called by Load with the service's Runtime. In a strict
// environment it rejects an empty Brokers; otherwise it fills the local default.
func (k *Kafka) ResolveFor(rt Runtime) error {
	if k.Brokers != "" {
		return nil
	}
	if rt.IsStrict() {
		return errors.New("KAFKA_BROKERS is required when ENV is staging or prod")
	}
	k.Brokers = defaultKafkaBrokers
	return nil
}

// BrokerList splits Brokers on commas, trimming blanks and dropping empty entries.
func (k Kafka) BrokerList() []string {
	var out []string
	for b := range strings.SplitSeq(k.Brokers, ",") {
		if b = strings.TrimSpace(b); b != "" {
			out = append(out, b)
		}
	}
	return out
}

// Outbox holds the outbox relayer settings.
type Outbox struct {
	PollInterval time.Duration `env:"OUTBOX_POLL_INTERVAL" default:"1s"`
	BatchSize    int           `env:"OUTBOX_BATCH_SIZE" default:"100"`
	// ClaimLock is how long a claimed row stays leased to one relayer.
	ClaimLock time.Duration `env:"OUTBOX_CLAIM_LOCK" default:"60s"`
}

// Flags holds the feature-flag settings. There is no on/off switch: a service
// always evaluates through Flipt, and an unreachable Flipt makes every
// evaluation return the default written in code.
type Flags struct {
	FliptAddr string `env:"FLIPT_ADDR" default:"localhost:9000"`
	// EvalTimeout bounds one flag evaluation.
	EvalTimeout time.Duration `env:"FEATURE_FLAGS_EVAL_TIMEOUT" default:"500ms"`
}

const defaultKafkaBrokers = "localhost:9092"

// PositiveDuration parses raw, a Go duration such as "20s" or "15m", for the
// variable key. An empty (or blank) raw value returns def; an unparsable or
// non-positive value returns an error naming key.
func PositiveDuration(key, raw string, def time.Duration) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid %s %q: want a positive Go duration such as 20s or 15m", key, raw)
	}
	return d, nil
}

// MockData holds the MOCK_DATA adapter list: a comma-separated set of adapter
// names that serve fixture data instead of calling the real dependency.
type MockData struct {
	Adapters string `env:"MOCK_DATA"`
}

// ResolveFor refuses a non-empty MOCK_DATA when the environment is strict.
func (m *MockData) ResolveFor(rt Runtime) error {
	if rt.IsStrict() && strings.TrimSpace(m.Adapters) != "" {
		return errors.New("MOCK_DATA must be empty when ENV is staging or prod")
	}
	return nil
}

// Audit holds the audit-service address. Addr has no tag default: in a strict
// environment an empty value is rejected, locally it stays empty.
type Audit struct {
	Addr string `env:"UPSTREAM_AUDIT_ADDR"`
}

// ResolveFor rejects an empty Addr when the environment is strict.
func (a *Audit) ResolveFor(rt Runtime) error {
	if rt.IsStrict() && strings.TrimSpace(a.Addr) == "" {
		return errors.New("UPSTREAM_AUDIT_ADDR is required when ENV is staging or prod")
	}
	return nil
}

const localLogKeyFPDefault = "local-dev-log-key-fp-secret"

// LogKeyFP holds the secret that keys the record-key fingerprints the consume
// loops log instead of raw keys. Secret has no tag default: Load fills a
// documented local default for a local ENV and rejects an unset or empty value
// when Runtime.IsStrict.
type LogKeyFP struct {
	Secret string `env:"LOG_KEY_FP_SECRET"`
}

// ResolveFor rejects an empty Secret when the environment is strict and fills
// the local default otherwise.
func (l *LogKeyFP) ResolveFor(rt Runtime) error {
	if strings.TrimSpace(l.Secret) != "" {
		return nil
	}
	if rt.IsStrict() {
		return errors.New("LOG_KEY_FP_SECRET is required when ENV is staging or prod")
	}
	l.Secret = localLogKeyFPDefault
	return nil
}
