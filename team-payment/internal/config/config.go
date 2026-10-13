// Package config holds team-payment's settings: the go-sdk groups it uses (Runtime,
// GRPC, Postgres, OTel, Kafka, Outbox) plus the service-specific groups below, bound
// from flat env keys with sdkconfig.Load. The `env`/`default` tags are the single source
// of truth for loading and for the .env.example drift test.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// Settings is the whole configuration surface. Runtime and GRPC are embedded SDK
// groups; the others are named because their field names collide.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
	Kafka    sdkconfig.Kafka
	Outbox   sdkconfig.Outbox
	Events   Events
	Upstream Upstream
	Mock     Mock
	Payout   Payout
}

// Events names the topics and the consumer group of the payment event flows. Brokers
// are in Kafka; the relayer and the settlement consumer are always on.
type Events struct {
	// PaymentTopic is where the outbox relayer produces PaymentSettled.
	PaymentTopic string `env:"PAYMENT_EVENTS_TOPIC" default:"payment.events"`
	// OrderTopic is the order.events topic the settlement consumer reads.
	OrderTopic string `env:"ORDER_EVENTS_TOPIC" default:"order.events"`
	// SettlementGroup is the settlement consumer group (a new group starts at the latest offset).
	SettlementGroup string `env:"PAYMENT_SETTLEMENT_CONSUMER_GROUP" default:"team-payment.settlement"`
	// SettlementDLQTopic parks poison records (not order.events.dlq, which is team-notification's).
	SettlementDLQTopic string `env:"PAYMENT_SETTLEMENT_DLQ_TOPIC" default:"order.events.payment-settlement.dlq"`
}

// Upstream holds the addresses of the services team-payment calls.
type Upstream struct {
	OrderAddr string `env:"UPSTREAM_ORDER_ADDR" default:"localhost:50055"`
}

// Mock holds development conveniences that must never be on in a shared environment.
type Mock struct {
	// MockPayments enables the ProcessMockPayment RPC (simulated settlement). Default
	// false: fail closed even when ENV is unset.
	MockPayments bool `env:"MOCK_PAYMENTS" default:"false"`
}

// Payout holds the refund hold-back on seller payouts (seller-payout-holdback, design D9).
// Money config fails closed: an invalid value refuses startup naming the key. sdkconfig.Load
// validates it like every other group.
type Payout struct {
	// HoldDays is the hold window in whole days, 0-3650; 0 disables the hold.
	HoldDays int `env:"PAYOUT_HOLD_DAYS" default:"7"`
	// HoldWindow, when non-empty, is a Go duration (0 to 3650 days) that overrides
	// HoldDays (the e2e overlay sets seconds).
	HoldWindow string `env:"PAYOUT_HOLD_WINDOW" default:""`
	// Window is the effective hold window, resolved by Load (not an env key).
	Window time.Duration
}

// maxHoldDays bounds both hold keys.
const maxHoldDays = 3650

// resolve validates both keys and returns the effective window.
func (p Payout) resolve() (time.Duration, error) {
	if p.HoldDays < 0 || p.HoldDays > maxHoldDays {
		return 0, fmt.Errorf("PAYOUT_HOLD_DAYS must be between 0 and %d, got %d", maxHoldDays, p.HoldDays)
	}
	raw := strings.TrimSpace(p.HoldWindow)
	if raw == "" {
		return time.Duration(p.HoldDays) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("PAYOUT_HOLD_WINDOW is not a Go duration (e.g. 168h, 20s): %q", p.HoldWindow)
	}
	if d < 0 || d > maxHoldDays*24*time.Hour {
		return 0, fmt.Errorf("PAYOUT_HOLD_WINDOW must be between 0 and %d days, got %s", maxHoldDays, d)
	}
	return d, nil
}

// Validate is called by sdkconfig.Load on the payout group.
func (p Payout) Validate() error {
	_, err := p.resolve()
	return err
}

// Load reads Settings from the environment, validates it and resolves the payout window.
func Load() (Settings, error) {
	s, err := sdkconfig.Load[Settings]()
	if err != nil {
		// A refused boot also names MOCK_PAYMENTS when that guard would refuse too; the
		// guard stays out of Validate because a valid Settings with mock in a strict ENV
		// still loads (app.RunWith refuses it).
		return s, errors.Join(err, s.RequireNoMockPayments())
	}
	s.Payout.Window, _ = s.Payout.resolve() // validated by Validate
	return s, nil
}

// Validate holds the one check no SDK group makes: the gRPC port range. sdkconfig.Load
// validates every group (ENV, GRPC_PORT, DATABASE_URL, PAYOUT_*) and joins all the errors with
// this one, so a refused boot names each offending key once.
func (s Settings) Validate() error {
	if s.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Port)
	}
	return nil
}

// IsProd reports whether this is a production environment (case-insensitive).
func (s Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Env))
	return e == "prod" || e == "production"
}

// RequireNoMockPayments is the boot guard against mock settlement in a shared
// environment: MOCK_PAYMENTS=true with a strict ENV (staging/stage/prod/production)
// refuses to start. Other environments always pass.
func (s Settings) RequireNoMockPayments() error {
	if !s.Mock.MockPayments || !s.IsStrict() {
		return nil
	}
	return fmt.Errorf(
		"refusing to start: MOCK_PAYMENTS=true is not allowed when ENV=%q (strict for ENV in %s); unset MOCK_PAYMENTS",
		s.Env, "staging, stage, prod, production")
}
