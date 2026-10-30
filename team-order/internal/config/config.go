package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

const (
	DefaultReservationTTL           = 15 * time.Minute
	DefaultReservationSweepInterval = time.Minute
	DefaultPaymentDeadline          = 24 * time.Hour
	DefaultPaymentReminderBefore    = 2 * time.Hour
	DefaultLifecycleSweepInterval   = time.Minute
)

type Upstream struct {
	DomainAddr    string `env:"UPSTREAM_DOMAIN_ADDR"`
	IdentityAddr  string `env:"UPSTREAM_IDENTITY_ADDR"`
	PromotionAddr string `env:"UPSTREAM_PROMOTION_ADDR"`
}

func (u Upstream) ResolveFor(sdkconfig.Runtime) error {
	var errs []error
	for _, c := range []struct{ key, val string }{
		{"UPSTREAM_DOMAIN_ADDR", u.DomainAddr},
		{"UPSTREAM_IDENTITY_ADDR", u.IdentityAddr},
		{"UPSTREAM_PROMOTION_ADDR", u.PromotionAddr},
	} {
		if strings.TrimSpace(c.val) == "" {
			errs = append(errs, fmt.Errorf("%s is required", c.key))
		}
	}
	return errors.Join(errs...)
}

type Audit struct {
	Addr string `env:"UPSTREAM_AUDIT_ADDR"`
}

func (a Audit) ResolveFor(sdkconfig.Runtime) error {
	if strings.TrimSpace(a.Addr) == "" {
		return errors.New("UPSTREAM_AUDIT_ADDR is required")
	}
	return nil
}

type Events struct {
	PaymentTopic    string `env:"PAYMENT_EVENTS_TOPIC" default:"payment.events"`
	PaymentGroup    string `env:"ORDER_PAYMENT_CONSUMER_GROUP" default:"team-order.payment"`
	PaymentDLQ      string `env:"PAYMENT_EVENTS_DLQ_TOPIC" default:"payment.events.dlq"`
	OrderTopic      string `env:"ORDER_EVENTS_TOPIC" default:"order.events"`
	EngagementTopic string `env:"ENGAGEMENT_EVENTS_TOPIC" default:"engagement.events"`
	DisputeGroup    string `env:"ORDER_DISPUTE_CONSUMER_GROUP" default:"team-order.dispute"`
	EngagementDLQ   string `env:"ENGAGEMENT_EVENTS_DLQ_TOPIC" default:"engagement.events.order.dlq"`
}

type Reservation struct {
	TTL           string `env:"RESERVATION_TTL" default:"15m"`
	SweepInterval string `env:"RESERVATION_SWEEP_INTERVAL" default:"1m"`
}

type Lifecycle struct {
	PaymentDeadline       string `env:"PAYMENT_DEADLINE" default:"24h"`
	PaymentReminderBefore string `env:"PAYMENT_REMINDER_BEFORE" default:"2h"`
	SweepInterval         string `env:"LIFECYCLE_SWEEP_INTERVAL" default:"1m"`

	AutoCompleteAfter      string `env:"AUTO_COMPLETE_AFTER" default:"168h"`
	ReturnWindow           string `env:"RETURN_WINDOW" default:"360h"`
	ReturnAutoApproveAfter string `env:"RETURN_AUTO_APPROVE_AFTER" default:"72h"`
	ShipByAfter            string `env:"SHIP_BY_AFTER" default:"48h"`
}

func (l Lifecycle) Validate() error {
	var errs []error
	for _, c := range []struct{ key, raw string }{
		{"AUTO_COMPLETE_AFTER", l.AutoCompleteAfter},
		{"RETURN_WINDOW", l.ReturnWindow},
		{"RETURN_AUTO_APPROVE_AFTER", l.ReturnAutoApproveAfter},
		{"SHIP_BY_AFTER", l.ShipByAfter},
	} {
		d, err := time.ParseDuration(strings.TrimSpace(c.raw))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: invalid duration %q (want a positive Go duration such as 168h): %w", c.key, c.raw, err))
			continue
		}
		if d <= 0 {
			errs = append(errs, fmt.Errorf("%s: duration must be positive, got %q", c.key, c.raw))
		}
	}
	return errors.Join(errs...)
}

type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres    sdkconfig.Postgres
	OTel        sdkconfig.OTel
	Kafka       sdkconfig.Kafka
	Outbox      sdkconfig.Outbox
	Flags       sdkconfig.Flags
	LogKeyFP    sdkconfig.LogKeyFP
	Events      Events
	Upstream    Upstream
	Audit       Audit
	Reservation Reservation
	Lifecycle   Lifecycle
}

func Load() (Settings, error) {
	return sdkconfig.Load[Settings]()
}

func (s Settings) Validate() error {
	return errors.Join(s.Runtime.Validate(), s.GRPC.Validate(), s.Postgres.Validate())
}

func strictDuration(raw string) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

func (s *Settings) AutoCompleteAfter() time.Duration {
	return strictDuration(s.Lifecycle.AutoCompleteAfter)
}
func (s *Settings) ReturnWindow() time.Duration { return strictDuration(s.Lifecycle.ReturnWindow) }
func (s *Settings) ReturnAutoApproveAfter() time.Duration {
	return strictDuration(s.Lifecycle.ReturnAutoApproveAfter)
}
func (s *Settings) ShipByAfter() time.Duration { return strictDuration(s.Lifecycle.ShipByAfter) }

func (s *Settings) PaymentDeadline() (time.Duration, string) {
	return positiveDuration("PAYMENT_DEADLINE", s.Lifecycle.PaymentDeadline, DefaultPaymentDeadline)
}

func (s *Settings) PaymentReminderBefore() (time.Duration, string) {
	return positiveDuration("PAYMENT_REMINDER_BEFORE", s.Lifecycle.PaymentReminderBefore, DefaultPaymentReminderBefore)
}

func (s *Settings) LifecycleSweepInterval() (time.Duration, string) {
	return positiveDuration("LIFECYCLE_SWEEP_INTERVAL", s.Lifecycle.SweepInterval, DefaultLifecycleSweepInterval)
}

func (s *Settings) ReservationTTL() (time.Duration, string) {
	return positiveDuration("RESERVATION_TTL", s.Reservation.TTL, DefaultReservationTTL)
}

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
