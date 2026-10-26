package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

const maxHoldDays = 3650

type Upstream struct {
	OrderAddr string `env:"UPSTREAM_ORDER_ADDR"`
	AuditAddr string `env:"UPSTREAM_AUDIT_ADDR"`
}

func (u Upstream) ResolveFor(sdkconfig.Runtime) error {
	var errs []error
	if strings.TrimSpace(u.OrderAddr) == "" {
		errs = append(errs, errors.New("UPSTREAM_ORDER_ADDR is required"))
	}
	if strings.TrimSpace(u.AuditAddr) == "" {
		errs = append(errs, errors.New("UPSTREAM_AUDIT_ADDR is required"))
	}
	return errors.Join(errs...)
}

type Events struct {
	PaymentTopic  string `env:"PAYMENT_EVENTS_TOPIC" default:"payment.events"`
	OrderTopic    string `env:"ORDER_EVENTS_TOPIC" default:"order.events"`
	ConsumerGroup string `env:"PAYMENT_SETTLEMENT_CONSUMER_GROUP" default:"team-payment.settlement"`
	DLQTopic      string `env:"PAYMENT_SETTLEMENT_DLQ_TOPIC" default:"order.events.payment-settlement.dlq"`
}

type Payout struct {
	HoldDays   int    `env:"PAYOUT_HOLD_DAYS" default:"7"`
	HoldWindow string `env:"PAYOUT_HOLD_WINDOW"`
	Window     time.Duration
}

func (p *Payout) ResolveFor(sdkconfig.Runtime) error {
	if p.HoldDays < 0 || p.HoldDays > maxHoldDays {
		return fmt.Errorf("PAYOUT_HOLD_DAYS must be between 0 and %d, got %d", maxHoldDays, p.HoldDays)
	}
	raw := strings.TrimSpace(p.HoldWindow)
	if raw == "" {
		p.Window = time.Duration(p.HoldDays) * 24 * time.Hour
		return nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("PAYOUT_HOLD_WINDOW is not a Go duration (e.g. 168h, 20s): %q", p.HoldWindow)
	}
	if d < 0 || d > maxHoldDays*24*time.Hour {
		return fmt.Errorf("PAYOUT_HOLD_WINDOW must be between 0 and %d days, got %s", maxHoldDays, d)
	}
	p.Window = d
	return nil
}

type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
	Kafka    sdkconfig.Kafka
	Outbox   sdkconfig.Outbox
	LogKeyFP sdkconfig.LogKeyFP
	MockData sdkconfig.MockData
	Events   Events
	Upstream Upstream
	Payout   Payout
}

func Load() (Settings, error) {
	rt := sdkconfig.Runtime{Env: os.Getenv("ENV")}
	mock := sdkconfig.MockData{Adapters: os.Getenv("MOCK_DATA")}
	if err := mock.ResolveFor(rt); err != nil {
		return Settings{}, err
	}
	return sdkconfig.Load[Settings]()
}
