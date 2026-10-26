package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

type Notification struct {
	DomainAddr   string `env:"UPSTREAM_DOMAIN_ADDR"`
	IdentityAddr string `env:"UPSTREAM_IDENTITY_ADDR"`

	LowStockThreshold int32 `env:"LOW_STOCK_THRESHOLD" default:"5"`

	ListingTopic         string `env:"LISTING_EVENTS_TOPIC" default:"listing.events"`
	ListingDLQTopic      string `env:"LISTING_EVENTS_DLQ_TOPIC" default:"listing.events.dlq"`
	ListingConsumerGroup string `env:"NOTIFICATION_LISTING_CONSUMER_GROUP" default:"team-notification.listing"`
	ChatTopic            string `env:"CHAT_EVENTS_TOPIC" default:"chat.events"`
	ChatDLQTopic         string `env:"CHAT_EVENTS_DLQ_TOPIC" default:"chat.events.dlq"`
	ChatConsumerGroup    string `env:"NOTIFICATION_CHAT_CONSUMER_GROUP" default:"team-notification.chat"`
	OrderTopic           string `env:"ORDER_EVENTS_TOPIC" default:"order.events"`
	OrderDLQTopic        string `env:"ORDER_EVENTS_DLQ_TOPIC" default:"order.events.dlq"`
	OrderConsumerGroup   string `env:"NOTIFICATION_ORDER_CONSUMER_GROUP" default:"team-notification.order"`
}

func (n Notification) Validate() error {
	var errs []error
	if n.DomainAddr == "" {
		errs = append(errs, errors.New("UPSTREAM_DOMAIN_ADDR is required"))
	}
	if n.IdentityAddr == "" {
		errs = append(errs, errors.New("UPSTREAM_IDENTITY_ADDR is required"))
	}
	return errors.Join(errs...)
}

type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres     sdkconfig.Postgres
	Kafka        sdkconfig.Kafka
	LogKeyFP     sdkconfig.LogKeyFP
	OTel         sdkconfig.OTel
	Notification Notification
}

func Load() (Settings, error) {
	if raw, ok := os.LookupEnv("LOW_STOCK_THRESHOLD"); ok && raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 32); err != nil || v < 0 {
			return Settings{}, fmt.Errorf("invalid LOW_STOCK_THRESHOLD %q: want integer in [0, 2147483647]", raw)
		}
	}
	return sdkconfig.Load[Settings]()
}
