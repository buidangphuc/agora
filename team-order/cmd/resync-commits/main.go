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
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-order/internal/bootstrap"
	"github.com/buidangphuc/team-order/internal/config"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
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

	settings, err := config.LoadSettings()
	if err != nil {
		return 1, fmt.Errorf("load config: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil)).With(slog.String("cmd", "resync-commits"))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	res, err := bootstrap.InitResources(ctx, settings, logger)
	if err != nil {
		return 1, fmt.Errorf("init resources: %w", err)
	}
	defer func() { _ = bootstrap.CloseResources(context.Background(), res) }()
	if res.Pool == nil {
		return 1, errors.New("database is disabled; there is no durable order_reservations table to re-sync")
	}

	clients, err := upstream.Dial(settings.Upstream.DomainAddr, settings.Upstream.IdentityAddr)
	if err != nil {
		return 1, fmt.Errorf("dial upstream: %w", err)
	}
	defer clients.Close()

	rep, err := service.ResyncCommittedReservations(ctx, repository.NewPostgresSagaRepository(res.Pool), clients.Listing, logger, *dryRun)
	logger.Info("resync finished",
		slog.Bool("dry_run", *dryRun),
		slog.Int("scanned", rep.Scanned), slog.Int("committed", rep.Committed),
		slog.Int("released_in_domain", rep.Released), slog.Int("not_found_in_domain", rep.NotFound),
		slog.Int("failed", rep.Failed))
	if err != nil {
		return 1, err
	}
	if rep.NeedsAttention() {
		return 2, nil
	}
	return 0, nil
}
