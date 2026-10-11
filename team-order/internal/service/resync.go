package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/upstream"
)

// ErrDomainNotUpgraded: team-domain answers UNIMPLEMENTED for CommitReservation;
// deploy it first.
var ErrDomainNotUpgraded = errors.New("team-domain does not implement CommitReservation; deploy it first")

const resyncPageSize = 200

// ResyncReport summarises one re-sync pass.
type ResyncReport struct {
	Scanned   int // local COMMITTED reservations visited
	Committed int // CommitReservation succeeded (newly or already committed)
	Released  int // FAILED_PRECONDITION: already released (swept) in team-domain; logged, skipped
	NotFound  int // NOT_FOUND: unknown to team-domain; logged, skipped
	Failed    int // other errors; safe to re-run
}

// NeedsAttention reports whether any row could not be reconciled.
func (r ResyncReport) NeedsAttention() bool { return r.Released+r.NotFound+r.Failed > 0 }

// ResyncCommittedReservations calls team-domain's CommitReservation for every
// reservation team-order already records as COMMITTED (orders placed before the
// commit call existed), so team-domain's TTL sweep stops restoring their stock.
// Cross-database backfill is not allowed (Rule 3), so it goes over gRPC, as the
// service-team-order principal. It writes nothing locally and CommitReservation is
// idempotent: a second run changes nothing. dryRun only counts the rows.
func ResyncCommittedReservations(ctx context.Context, repo repository.SagaRepository, domain upstream.DomainClient, logger *slog.Logger, dryRun bool) (ResyncReport, error) {
	if logger == nil {
		logger = slog.Default()
	}
	domain = upstream.NewServiceStockClient(domain)
	var rep ResyncReport
	after := ""
	for {
		page, err := repo.ListCommittedReservations(ctx, after, resyncPageSize)
		if err != nil {
			return rep, fmt.Errorf("list committed reservations: %w", err)
		}
		if len(page) == 0 {
			return rep, nil
		}
		for _, res := range page {
			after = res.ID
			rep.Scanned++
			if dryRun {
				continue
			}
			if err := ctx.Err(); err != nil {
				return rep, err
			}
			_, err := domain.CommitReservation(ctx, &listingv1.CommitReservationRequest{ReservationId: res.ID})
			switch status.Code(err) {
			case codes.OK:
				rep.Committed++
			case codes.Unimplemented:
				return rep, ErrDomainNotUpgraded
			case codes.FailedPrecondition:
				rep.Released++
				logger.WarnContext(ctx, "resync: reservation already released in team-domain; skipped",
					slog.String("reservation_id", res.ID), slog.String("order_id", res.OrderID),
					slog.String("listing_id", res.ListingID), slog.Int("quantity", int(res.Quantity)))
			case codes.NotFound:
				rep.NotFound++
				logger.WarnContext(ctx, "resync: reservation unknown to team-domain; skipped",
					slog.String("reservation_id", res.ID), slog.String("order_id", res.OrderID))
			default:
				rep.Failed++
				logger.ErrorContext(ctx, "resync: commit failed; re-run to retry",
					slog.String("reservation_id", res.ID), slog.Any("err", err))
			}
		}
	}
}
