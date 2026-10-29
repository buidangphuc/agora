// Command resync-commits is the one-off backfill of change
// port-order-inventory-correctness: it calls team-domain's CommitReservation for
// every local COMMITTED order_reservations row, so orders placed before
// team-order started committing are not restored by team-domain's TTL sweep. It
// is read-only locally and idempotent; safe to re-run.
//
// Usage (same env as the server: DATABASE_URL, UPSTREAM_DOMAIN_ADDR, ...):
//
//	resync-commits [-dry-run]
//
// Exit status: 0 all rows reconciled; 1 fatal error; 2 some rows need attention
// (released/unknown in team-domain, or transient failures) - see the log.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-order/internal/config"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/upstream"
)

func main() {
	code, err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resync-commits: %v\n", err)
	}
	os.Exit(code)
}

func run() (int, error) {
	dryRun := flag.Bool("dry-run", false, "count COMMITTED reservations without calling team-domain")
	flag.Parse()

	settings, err := config.Load()
	if err != nil {
		return 1, fmt.Errorf("load config: %w", err)
	}
	logger := logx.New(settings.Runtime, "resync-commits")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Open(ctx, settings.Postgres)
	if err != nil {
		return 1, fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()

	clients, err := upstream.DialAll(settings)
	if err != nil {
		return 1, fmt.Errorf("dial upstream: %w", err)
	}
	defer clients.Close()

	rep, err := order.ResyncCommittedReservations(ctx, store.NewPostgresSagaRepository(pool), clients.Listing, logger, *dryRun)
	logger.InfoContext(ctx, "resync finished",
		"dry_run", *dryRun,
		"scanned", rep.Scanned, "committed", rep.Committed,
		"released_in_domain", rep.Released, "not_found_in_domain", rep.NotFound,
		"failed", rep.Failed)
	if err != nil {
		return 1, err
	}
	if rep.NeedsAttention() {
		return 2, nil
	}
	return 0, nil
}
