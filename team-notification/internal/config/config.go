// Package config holds team-notification's settings: the SDK groups it uses plus
// its own upstream and consumer settings, bound from environment variables.
package config

import sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

// Upstream holds the addresses of the services that name a chat sender:
// team-domain (shop display name) and team-identity (user display name).
type Upstream struct {
	DomainAddr   string `env:"UPSTREAM_DOMAIN_ADDR" default:"localhost:50051"`
	IdentityAddr string `env:"UPSTREAM_IDENTITY_ADDR" default:"localhost:50053"`
}

// ListingEvents is the listing.events consumer: topic, dead-letter topic and
// consumer group.
type ListingEvents struct {
	Topic    string `env:"LISTING_EVENTS_TOPIC" default:"listing.events"`
	DLQTopic string `env:"LISTING_EVENTS_DLQ_TOPIC" default:"listing.events.dlq"`
	Group    string `env:"NOTIFICATION_LISTING_CONSUMER_GROUP" default:"team-notification.listing"`
}

// ChatEvents is the chat.events consumer.
type ChatEvents struct {
	Topic    string `env:"CHAT_EVENTS_TOPIC" default:"chat.events"`
	DLQTopic string `env:"CHAT_EVENTS_DLQ_TOPIC" default:"chat.events.dlq"`
	Group    string `env:"NOTIFICATION_CHAT_CONSUMER_GROUP" default:"team-notification.chat"`
}

// OrderEvents is the order.events consumer.
type OrderEvents struct {
	Topic    string `env:"ORDER_EVENTS_TOPIC" default:"order.events"`
	DLQTopic string `env:"ORDER_EVENTS_DLQ_TOPIC" default:"order.events.dlq"`
	Group    string `env:"NOTIFICATION_ORDER_CONSUMER_GROUP" default:"team-notification.order"`
}

// Settings is everything the service reads from the environment. Runtime and
// GRPC are embedded because nothing else collides with their fields; the other
// groups are named because several have an Enabled or Topic field.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
	Kafka    sdkconfig.Kafka
	Upstream Upstream
	Listing  ListingEvents
	Chat     ChatEvents
	Order    OrderEvents
}

// Load reads Settings from the environment and validates it.
func Load() (Settings, error) { return sdkconfig.Load[Settings]() }
