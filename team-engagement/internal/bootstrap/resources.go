package bootstrap

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/health"
)

// Resources is the central bag of opened handles for team-identity.
type Resources struct {
	Pool   *pgxpool.Pool
	Health *health.Server

	stopHealth context.CancelFunc
}
