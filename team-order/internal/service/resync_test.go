package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

// Orders placed before the upgrade: team-order has COMMITTED reservations whose
// team-domain twins are still active (one was already swept).
func preUpgradeState(t *testing.T, n int) (*repository.InMemorySagaRepository, *upstreamtest.Domain, []string) {
	t.Helper()
	ctx := context.Background()
	sagas := repository.NewInMemorySagaRepository()
	domain := upstreamtest.NewDomain(map[string]int32{"lst_1": 1000})
	var ids []string
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("res-%03d", i)
		r := repository.Reservation{ID: id, SagaID: "s", OrderID: fmt.Sprintf("ord-%d", i), ListingID: "lst_1", Quantity: 1,
			Status: repository.ReservationStatusCommitted, ExpiresAt: time.Now()}
		if _, err := sagas.CreateReservation(ctx, r); err != nil {
			t.Fatal(err)
		}
		if _, err := domain.ReserveStock(ctx, reserveReq(r)); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	// A RELEASED local row is not re-synced.
	_, _ = sagas.CreateReservation(ctx, repository.Reservation{ID: "res-released", SagaID: "s", ListingID: "lst_1", Quantity: 1,
		Status: repository.ReservationStatusReleased, ExpiresAt: time.Now()})
	return sagas, domain, ids
}

func TestResyncCommittedReservations_IdempotentSecondRunChangesNothing(t *testing.T) {
	ctx := context.Background()
	sagas, domain, ids := preUpgradeState(t, 450) // > one page
	// One of them was already swept by team-domain before the resync ran.
	domain.ReleaseStock(ctx, releaseReq(ids[7]))
	stockBefore := domain.Stock("lst_1")

	rep, err := service.ResyncCommittedReservations(ctx, sagas, domain, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 450 || rep.Committed != 449 || rep.Released != 1 || rep.Failed != 0 || !rep.NeedsAttention() {
		t.Fatalf("first run: %+v", rep)
	}
	for i, id := range ids {
		want := upstreamtest.StateCommitted
		if i == 7 {
			want = upstreamtest.StateReleased
		}
		if got := domain.State(id); got != want {
			t.Fatalf("%s: %q want %q", id, got, want)
		}
	}
	if domain.Sweep() != 0 || domain.Stock("lst_1") != stockBefore {
		t.Fatal("after the resync team-domain's sweep must restore nothing")
	}

	rep2, err := service.ResyncCommittedReservations(ctx, sagas, domain, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep2 != rep {
		t.Fatalf("second run must see the same picture: %+v vs %+v", rep2, rep)
	}
	if domain.Stock("lst_1") != stockBefore {
		t.Fatal("the second run must change nothing")
	}
	local, _ := sagas.ListCommittedReservations(ctx, "", 1000)
	if len(local) != 450 {
		t.Fatalf("the resync must not write locally: %d committed rows", len(local))
	}
}

func TestResyncCommittedReservations_DryRunAndOldDomain(t *testing.T) {
	ctx := context.Background()
	sagas, domain, _ := preUpgradeState(t, 3)
	rep, err := service.ResyncCommittedReservations(ctx, sagas, domain, nil, true)
	if err != nil || rep.Scanned != 3 || domain.Calls.Commit != 0 {
		t.Fatalf("dry run: %+v %v commits=%d", rep, err, domain.Calls.Commit)
	}
	domain.CommitErr = func(string) error { return status.Error(codes.Unimplemented, "old") }
	if _, err := service.ResyncCommittedReservations(ctx, sagas, domain, nil, false); !errors.Is(err, service.ErrDomainNotUpgraded) {
		t.Fatalf("want ErrDomainNotUpgraded, got %v", err)
	}
}
