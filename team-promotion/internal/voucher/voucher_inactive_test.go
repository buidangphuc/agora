package voucher

import (
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

type inactiveEnv struct {
	*adminEnv
	rrepo *storefake.InMemoryReservationRepository
}

func newInactiveEnv(t *testing.T) *inactiveEnv {
	t.Helper()
	log := &callLog{}
	w := &recordingAuditWriter{log: log}
	mem := storefake.NewInMemoryVoucherRepository()
	rep := &adminRepo{VoucherRepository: mem, log: log}
	rrepo := storefake.NewInMemoryReservationRepository()
	events := &eventLog{}
	svc := New(rep, rrepo, auditx.NewGate(w, nil), events.build, nil)
	svc.nowFn = func() time.Time { return adminNow }
	return &inactiveEnv{adminEnv: &adminEnv{svc: svc, mem: mem, rep: rep, w: w, log: log}, rrepo: rrepo}
}

func (e *inactiveEnv) stop(t *testing.T, id string) {
	t.Helper()
	if _, err := e.svc.DeactivateVoucher(t.Context(), DeactivateVoucherParams{VoucherID: id, AuditActorID: "admin-1"}); err != nil {
		t.Fatalf("DeactivateVoucher: %v", err)
	}
}

func TestValidateAndReserve_V1a_inactiveRefusedWithReason(t *testing.T) {
	e := newInactiveEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	e.stop(t, v.ID)
	res, err := e.svc.ValidateAndReserve(t.Context(), "resv-1", "SALE10", "buyer-1", 100000, "")
	if err != nil {
		t.Fatalf("ValidateAndReserve: %v", err)
	}
	if res.Valid || res.Reason != "inactive" || res.DiscountAmount != 0 {
		t.Fatalf("want valid=false reason=inactive, got %+v", res)
	}
}

func TestValidateAndReserve_V1b_inactiveCreatesNoHold(t *testing.T) {
	e := newInactiveEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	e.stop(t, v.ID)
	if _, err := e.svc.ValidateAndReserve(t.Context(), "resv-1", "SALE10", "buyer-1", 100000, ""); err != nil {
		t.Fatalf("ValidateAndReserve: %v", err)
	}
	assertNoHold(t, e.rrepo, "resv-1")
}

func TestValidateAndReserve_V1c_inactiveWinsOverExpiredAndOtherRules(t *testing.T) {
	tests := []struct {
		name     string
		subtotal int64
		seller   string
		stopAt   func(e *inactiveEnv, id string)
	}{
		{name: "expired", subtotal: 100000, stopAt: func(e *inactiveEnv, _ string) {
			e.svc.nowFn = func() time.Time { return adminNow.Add(48 * time.Hour) }
		}},
		{name: "below min spend", subtotal: 1},
		{name: "quota exhausted", subtotal: 100000, stopAt: func(e *inactiveEnv, id string) {
			if err := e.mem.IncrementUsed(t.Context(), id, 1); err != nil {
				t.Fatalf("IncrementUsed: %v", err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newInactiveEnv(t)
			quota := int64(100)
			if tc.name == "quota exhausted" {
				quota = 1
			}
			v := e.seed(t, "SALE10", quota, 0)
			e.stop(t, v.ID)
			if tc.stopAt != nil {
				tc.stopAt(e, v.ID)
			}
			res, err := e.svc.ValidateAndReserve(t.Context(), "resv-1", "SALE10", "buyer-1", tc.subtotal, tc.seller)
			if err != nil {
				t.Fatalf("ValidateAndReserve: %v", err)
			}
			if res.Valid || res.Reason != "inactive" {
				t.Fatalf("want reason inactive, got %+v", res)
			}
		})
	}
}

func TestValidateAndReserve_V1_activeVoucherStillReserves(t *testing.T) {
	e := newInactiveEnv(t)
	e.seed(t, "SALE10", 100, 0)
	res, err := e.svc.ValidateAndReserve(t.Context(), "resv-1", "SALE10", "buyer-1", 100000, "")
	if err != nil {
		t.Fatalf("ValidateAndReserve: %v", err)
	}
	if !res.Valid || res.Reason != "" {
		t.Fatalf("active voucher must reserve, got %+v", res)
	}
}

func TestCommitReservation_V2_existingHoldCommitsAfterStop(t *testing.T) {
	e := newInactiveEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	res, err := e.svc.ValidateAndReserve(t.Context(), "resv-1", "SALE10", "buyer-1", 100000, "")
	if err != nil || !res.Valid {
		t.Fatalf("reserve before stop: res=%+v err=%v", res, err)
	}
	e.stop(t, v.ID)
	ok, err := e.svc.CommitReservation(t.Context(), "resv-1")
	if err != nil || !ok {
		t.Fatalf("commit after stop: ok=%v err=%v", ok, err)
	}
	if got := e.get(t, v.ID); got.Used != 1 || got.Active {
		t.Fatalf("want used=1 and still inactive, got %+v", got)
	}
}

func TestReleaseReservation_V3_existingHoldReleasesAfterStop(t *testing.T) {
	e := newInactiveEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	res, err := e.svc.ValidateAndReserve(t.Context(), "resv-1", "SALE10", "buyer-1", 100000, "")
	if err != nil || !res.Valid {
		t.Fatalf("reserve before stop: res=%+v err=%v", res, err)
	}
	e.stop(t, v.ID)
	ok, err := e.svc.ReleaseReservation(t.Context(), "resv-1")
	if err != nil || !ok {
		t.Fatalf("release after stop: ok=%v err=%v", ok, err)
	}
	if got := e.get(t, v.ID); got.Used != 0 {
		t.Fatalf("want used unchanged 0, got %+v", got)
	}
}
