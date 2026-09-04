package bootstrap

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/health"

	"github.com/buidangphuc/team-domain/internal/events"
	"github.com/buidangphuc/team-domain/internal/repository"
)

// Resources is the central bag of opened handles, mirroring team-ai's
// ApplicationResources. CloseResources tears these down in reverse order.
type Resources struct {
	// Pool is this service's OWN Postgres connection pool (nil if DB disabled).
	Pool *pgxpool.Pool
	// Health is the gRPC health server; its "" (overall) status tracks the DB.
	Health *health.Server
	// Publisher emits ListingChanged events (KafkaPublisher or NoopPublisher).
	// It is now the relayer's produce path, not the request path.
	Publisher events.ListingPublisher
	// Outbox is the transactional-outbox store over the pool (nil if DB disabled).
	// main.go wires it into the service so writes record their event atomically.
	Outbox *repository.OutboxStore

	// addons holds the addons that actually opened, for reverse-order teardown.
	addons []Addon
	// stopHealth cancels the background DB health monitor.
	stopHealth context.CancelFunc
	// stopRelayer cancels the background outbox relayer; relayerDone signals it
	// has drained and returned.
	stopRelayer context.CancelFunc
	relayerDone chan struct{}
	// stopSweeper cancels the background reservation sweeper; sweeperDone signals
	// it has returned (AD3 domain side).
	stopSweeper context.CancelFunc
	sweeperDone chan struct{}
}
