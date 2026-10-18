package bootstrap

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/health"

	"github.com/buidangphuc/team-identity/internal/events"
	"github.com/buidangphuc/team-identity/internal/repository"
	"github.com/buidangphuc/team-identity/internal/upstream"
)

// Resources is the central bag of opened handles for team-identity.
type Resources struct {
	Pool   *pgxpool.Pool
	Health *health.Server
	// Publisher is the relayer's produce path (KafkaPublisher or NoopPublisher).
	Publisher events.Publisher
	// Outbox is the transactional-outbox store over the pool.
	Outbox *repository.OutboxStore
	// Audit is the team-audit client; nil when UPSTREAM_AUDIT_ADDR is empty.
	Audit *upstream.AuditClient

	stopHealth context.CancelFunc
	// stopRelayer cancels the background outbox relayer; relayerDone signals it
	// has drained and returned.
	stopRelayer context.CancelFunc
	relayerDone chan struct{}
	// stopSweeper cancels the session retention sweeper; sweeperDone signals it returned.
	stopSweeper context.CancelFunc
	sweeperDone chan struct{}
}
