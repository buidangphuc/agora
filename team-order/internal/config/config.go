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
	Upstream      Upstream
	Observability Observability
	FeatureFlags  FeatureFlags
	Kafka         Kafka
	Outbox        Outbox
	Reservation   Reservation
}

type Runtime struct {
	Env      string `env:"ENV" default:"local"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`
	LogJSON  bool   `env:"LOG_JSON" default:"true"`
}

type Server struct {
	Host              string  `env:"GRPC_HOST" default:"0.0.0.0"`
	Port              int     `env:"GRPC_PORT" default:"50055"`
	ReflectionEnabled bool    `env:"GRPC_REFLECTION_ENABLED" default:"true"`
	ShutdownGrace     float64 `env:"SHUTDOWN_GRACE_SECONDS" default:"10"`
}

type Database struct {
	Enabled  bool   `env:"DATABASE_ENABLED" default:"true"`
	URL      string `env:"DATABASE_URL" default:""`
	MaxConns int32  `env:"DB_MAX_CONNS" default:"10"`
}

type Upstream struct {
	DomainAddr    string `env:"UPSTREAM_DOMAIN_ADDR" default:"localhost:50051"`
	IdentityAddr  string `env:"UPSTREAM_IDENTITY_ADDR" default:"localhost:50053"`
	PromotionAddr string `env:"UPSTREAM_PROMOTION_ADDR" default:""`
}

type Observability struct {
	Enabled      bool   `env:"OTEL_ENABLED" default:"false"`
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:""`
	ServiceName  string `env:"OTEL_SERVICE_NAME" default:"team-order"`
}

// FeatureFlags configures the OpenFeature + Flipt provider used to evaluate
// flags (e.g. the checkout kill-switch). FliptAddr is the Flipt gRPC endpoint;
// evaluation is in-process against a streamed in-memory snapshot.
type FeatureFlags struct {
	Enabled       bool   `env:"FEATURE_FLAGS_ENABLED" default:"true"`
	FliptAddr     string `env:"FLIPT_ADDR" default:"localhost:9000"`
	EvalTimeoutMS int    `env:"FEATURE_FLAGS_EVAL_TIMEOUT_MS" default:"500"`
}

// Kafka wires the payment.events consumer and the order.events producer. With
// KAFKA_ENABLED=false neither runs: outbox rows are still recorded with each PAID
// transition but never relayed.
type Kafka struct {
	Enabled       bool   `env:"KAFKA_ENABLED" default:"false"`
	Brokers       string `env:"KAFKA_BROKERS" default:"localhost:9092"` // comma-separated
	ConsumerGroup string `env:"ORDER_PAYMENT_CONSUMER_GROUP" default:"team-order.payment"`
	PaymentTopic  string `env:"PAYMENT_EVENTS_TOPIC" default:"payment.events"`
	PaymentDLQ    string `env:"PAYMENT_EVENTS_DLQ_TOPIC" default:"payment.events.dlq"`
	OrderTopic    string `env:"ORDER_EVENTS_TOPIC" default:"order.events"`
}

// Outbox tunes the transactional-outbox relayer that publishes order.events
// (ADR-0002, ADR-0013). Names match team-domain's Outbox group. The relayer runs
// only when OUTBOX_ENABLED and KAFKA_ENABLED are both true and Postgres is on.
type Outbox struct {
	Enabled          bool   `env:"OUTBOX_ENABLED" default:"true"`
	PollInterval     string `env:"OUTBOX_POLL_INTERVAL" default:"1s"` // Go duration
	BatchSize        int    `env:"OUTBOX_BATCH_SIZE" default:"100"`
	ClaimLockSeconds int    `env:"OUTBOX_CLAIM_LOCK_SECONDS" default:"60"`
	MaxAttempts      int    `env:"OUTBOX_MAX_ATTEMPTS" default:"10"`
}

// Reservation tunes the stock-reservation lifetime and the reservation sweeper.
// Both are Go duration strings ("20s", "1m", "15m"); read them through
// ReservationTTL and ReservationSweepInterval, which never fail: an unusable value
// falls back to the default with a warning so a typo cannot stop the service.
type Reservation struct {
	TTL           string `env:"RESERVATION_TTL" default:"15m"`
	SweepInterval string `env:"RESERVATION_SWEEP_INTERVAL" default:"1m"`
}

// DefaultReservationTTL is used when RESERVATION_TTL is empty or unusable. It
// matches team-domain's own default.
const DefaultReservationTTL = 15 * time.Minute

// DefaultReservationSweepInterval is used when RESERVATION_SWEEP_INTERVAL is empty
// or unusable.
const DefaultReservationSweepInterval = time.Minute

// ReservationTTL parses RESERVATION_TTL, the lifetime of a checkout's stock hold
// before the sweep may reclaim it (and of an unfinished checkout attempt). An
// empty, unparsable, zero or negative value yields the default and a non-empty
// warning naming the variable for the caller to log.
func (s *Settings) ReservationTTL() (time.Duration, string) {
	return positiveDuration("RESERVATION_TTL", s.Reservation.TTL, DefaultReservationTTL)
}

// ReservationSweepInterval parses RESERVATION_SWEEP_INTERVAL, how often the
// reservation sweeper runs. Same fallback rules as ReservationTTL.
func (s *Settings) ReservationSweepInterval() (time.Duration, string) {
	return positiveDuration("RESERVATION_SWEEP_INTERVAL", s.Reservation.SweepInterval, DefaultReservationSweepInterval)
}

func positiveDuration(key, raw string, def time.Duration) (time.Duration, string) {
	raw = strings.TrimSpace(raw)
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return def, fmt.Sprintf("invalid %s %q (want a positive Go duration such as 20s or 15m); using default %s", key, raw, def)
	}
	return d, ""
}

// KafkaBrokers splits the comma-separated KAFKA_BROKERS into seed addresses.
func (s *Settings) KafkaBrokers() []string {
	parts := strings.Split(s.Kafka.Brokers, ",")
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
	if s.Server.Port <= 0 || s.Server.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Server.Port)
	}
	return nil
}

func (s *Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Runtime.Env))
	return e == "prod" || e == "production"
}

// durableStorageEnvs are the ENV values (trimmed, lowercase) in which the service
// must never run on in-memory repositories. Anything else (local, test, unknown)
// is non-strict, matching the "local" default.
var durableStorageEnvs = []string{"staging", "stage", "prod", "production"}

// RequiresDurableStorage reports whether ENV names a staging/production environment.
func (s *Settings) RequiresDurableStorage() bool {
	e := strings.ToLower(strings.TrimSpace(s.Runtime.Env))
	for _, strict := range durableStorageEnvs {
		if e == strict {
			return true
		}
	}
	return false
}

// RequireDurableStorage is the boot guard against the silent in-memory fallback:
// in a strict ENV it fails when the database is disabled or no pool was obtained,
// so a mis-set flag refuses to boot instead of serving orders that vanish on
// restart. Other environments always pass.
func (s *Settings) RequireDurableStorage(dbAvailable bool) error {
	if !s.RequiresDurableStorage() {
		return nil
	}
	if s.Database.Enabled && dbAvailable {
		return nil
	}
	return fmt.Errorf(
		"refusing to start with in-memory storage: ENV=%q requires a database (strict for ENV in %s) but DATABASE_ENABLED=%t and the database pool available=%t; set DATABASE_ENABLED=true with a reachable DATABASE_URL",
		s.Runtime.Env, strings.Join(durableStorageEnvs, ", "), s.Database.Enabled, dbAvailable)
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
