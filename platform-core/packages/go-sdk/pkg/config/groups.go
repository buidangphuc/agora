package config

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Runtime holds settings every service shares.
type Runtime struct {
	Env      string `env:"ENV" default:"local"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`
	LogJSON  bool   `env:"LOG_JSON" default:"true"`
}

// IsLocal reports whether the service runs in a developer or test environment.
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
	Host                 string  `env:"GRPC_HOST" default:"0.0.0.0"`
	Port                 int     `env:"GRPC_PORT,required"`
	ShutdownGraceSeconds float64 `env:"SHUTDOWN_GRACE_SECONDS" default:"10"`
	// DrainDelaySeconds is how long to keep serving after health reports
	// NOT_SERVING, so probes and load balancers notice before the server stops.
	DrainDelaySeconds float64 `env:"SHUTDOWN_DRAIN_DELAY_SECONDS" default:"0"`
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

// ShutdownGrace returns the graceful shutdown deadline.
func (g GRPC) ShutdownGrace() time.Duration {
	return time.Duration(g.ShutdownGraceSeconds * float64(time.Second))
}

// DrainDelay returns the pause between draining health and stopping components.
func (g GRPC) DrainDelay() time.Duration {
	return time.Duration(g.DrainDelaySeconds * float64(time.Second))
}

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
// imports kafkax needs it.
type Kafka struct {
	Brokers string `env:"KAFKA_BROKERS" default:"localhost:9092"`
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
	PollInterval     time.Duration `env:"OUTBOX_POLL_INTERVAL" default:"1s"`
	BatchSize        int           `env:"OUTBOX_BATCH_SIZE" default:"100"`
	ClaimLockSeconds int           `env:"OUTBOX_CLAIM_LOCK_SECONDS" default:"60"`
	MaxAttempts      int           `env:"OUTBOX_MAX_ATTEMPTS" default:"10"`
}

// Flags holds the feature-flag settings. Enabled stays a flag because some
// environments run without Flipt.
type Flags struct {
	Enabled       bool   `env:"FEATURE_FLAGS_ENABLED" default:"true"`
	FliptAddr     string `env:"FLIPT_ADDR" default:"localhost:9000"`
	EvalTimeoutMS int    `env:"FEATURE_FLAGS_EVAL_TIMEOUT_MS" default:"500"`
}

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
