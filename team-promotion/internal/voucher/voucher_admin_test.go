package voucher

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

var adminNow = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

type adminRepo struct {
	store.VoucherRepository
	log          *callLog
	mu           sync.Mutex
	beforeUpdate func()
}

func (r *adminRepo) Create(ctx context.Context, v store.Voucher, event func(store.Voucher) (outbox.Row, error)) (store.Voucher, error) {
	r.log.add("insert")
	return r.VoucherRepository.Create(ctx, v, event)
}

func (r *adminRepo) Deactivate(ctx context.Context, id string, at time.Time) (store.Voucher, error) {
	r.log.add("deactivate")
	return r.VoucherRepository.Deactivate(ctx, id, at)
}

func (r *adminRepo) UpdateEndsAtQuota(ctx context.Context, id string, endsAt time.Time, quota int64) (store.Voucher, error) {
	r.log.add("update")
	r.mu.Lock()
	hook := r.beforeUpdate
	r.mu.Unlock()
	if hook != nil {
		hook()
	}
	return r.VoucherRepository.UpdateEndsAtQuota(ctx, id, endsAt, quota)
}

type adminEnv struct {
	svc *Service
	mem *storefake.InMemoryVoucherRepository
	rep *adminRepo
	w   *recordingAuditWriter
	log *callLog
}

func newAdminEnv(t *testing.T) *adminEnv {
	t.Helper()
	log := &callLog{}
	w := &recordingAuditWriter{log: log}
	mem := storefake.NewInMemoryVoucherRepository()
	rep := &adminRepo{VoucherRepository: mem, log: log}
	events := &eventLog{}
	svc := New(rep, storefake.NewInMemoryReservationRepository(), auditx.NewGate(w, nil), events.build, nil)
	svc.nowFn = func() time.Time { return adminNow }
	return &adminEnv{svc: svc, mem: mem, rep: rep, w: w, log: log}
}

func (e *adminEnv) seed(t *testing.T, code string, quota, used int64) store.Voucher {
	t.Helper()
	v, err := e.svc.CreateVoucher(t.Context(), CreateVoucherParams{
		Code: code, Scope: 2, DiscountType: discountTypePercent, DiscountValue: 10,
		MinSpend: 5000, MaxDiscount: 3000, Quota: quota,
		StartsAt: adminNow.Add(-time.Hour), EndsAt: adminNow.Add(24 * time.Hour),
		AuditActorID: "admin-1",
	})
	if err != nil {
		t.Fatalf("seed CreateVoucher: %v", err)
	}
	if used > 0 {
		if err := e.mem.IncrementUsed(t.Context(), v.ID, used); err != nil {
			t.Fatalf("seed used: %v", err)
		}
	}
	return e.get(t, v.ID)
}

func (e *adminEnv) get(t *testing.T, id string) store.Voucher {
	t.Helper()
	v, err := e.mem.GetByID(t.Context(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	return v
}

func (e *adminEnv) actions(action string) []auditx.Event {
	var out []auditx.Event
	for _, ev := range e.w.got() {
		if ev.Action == action {
			out = append(out, ev)
		}
	}
	return out
}

func (e *adminEnv) adminWrites() int {
	return len(e.actions("voucher.deactivate")) + len(e.actions("voucher.update"))
}

func (e *adminEnv) failAudit() {
	e.w.mu.Lock()
	e.w.err = errors.New("audit down")
	e.w.mu.Unlock()
}

func ptr[T any](v T) *T { return &v }

func assertSameVoucher(t *testing.T, got, want store.Voucher) {
	t.Helper()
	if got != want {
		t.Fatalf("voucher changed:\n got  %+v\n want %+v", got, want)
	}
}

func assertMoneyFieldsEqual(t *testing.T, got, want store.Voucher) {
	t.Helper()
	if got.DiscountType != want.DiscountType || got.DiscountValue != want.DiscountValue ||
		got.MinSpend != want.MinSpend || got.MaxDiscount != want.MaxDiscount ||
		got.Scope != want.Scope || got.SellerID != want.SellerID {
		t.Fatalf("money fields changed:\n got  %+v\n want %+v", got, want)
	}
}

func TestDeactivateVoucher_V4_V1_stopReturnsInactive(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	got, err := e.svc.DeactivateVoucher(t.Context(), DeactivateVoucherParams{VoucherID: v.ID, AuditActorID: "admin-1"})
	if err != nil {
		t.Fatalf("DeactivateVoucher: %v", err)
	}
	if got.Active || got.DeactivatedAt.IsZero() {
		t.Fatalf("want inactive with deactivated_at, got %+v", got)
	}
	if stored := e.get(t, v.ID); stored.Active {
		t.Fatalf("stored voucher still active: %+v", stored)
	}
}

func TestDeactivateVoucher_V4_twiceKeepsOriginalDeactivatedAt(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	first, err := e.svc.DeactivateVoucher(t.Context(), DeactivateVoucherParams{VoucherID: v.ID, AuditActorID: "admin-1"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	e.svc.nowFn = func() time.Time { return adminNow.Add(time.Hour) }
	second, err := e.svc.DeactivateVoucher(t.Context(), DeactivateVoucherParams{VoucherID: v.ID, AuditActorID: "admin-1"})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Active || !second.DeactivatedAt.Equal(first.DeactivatedAt) {
		t.Fatalf("deactivated_at moved: first=%v second=%v", first.DeactivatedAt, second.DeactivatedAt)
	}
}

func TestDeactivateVoucher_V4a_twiceAuditedPerCall(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	p := DeactivateVoucherParams{VoucherID: v.ID, AuditActorID: "admin-1"}
	if _, err := e.svc.DeactivateVoucher(t.Context(), p); err != nil {
		t.Fatalf("first: %v", err)
	}
	if n := len(e.actions("voucher.deactivate")); n != 1 {
		t.Fatalf("after first call: %d deactivate entries, want 1", n)
	}
	if _, err := e.svc.DeactivateVoucher(t.Context(), p); err != nil {
		t.Fatalf("second: %v", err)
	}
	if n := len(e.actions("voucher.deactivate")); n != 2 {
		t.Fatalf("after second call: %d deactivate entries, want 2", n)
	}
}

func TestDeactivateVoucher_V5_V5a_unknownIsNotFoundWithoutAudit(t *testing.T) {
	e := newAdminEnv(t)
	e.seed(t, "SALE10", 100, 0)
	_, err := e.svc.DeactivateVoucher(t.Context(), DeactivateVoucherParams{VoucherID: "no-such-id", AuditActorID: "admin-1"})
	if !errors.Is(err, store.ErrVoucherNotFound) {
		t.Fatalf("err = %v, want ErrVoucherNotFound", err)
	}
	if n := e.adminWrites(); n != 0 {
		t.Fatalf("%d audit entries for a refused call, want 0", n)
	}
}

func TestDeactivateVoucher_V22_auditBeforeWriteWithActorAndCode(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	n0 := len(e.log.get())
	if _, err := e.svc.DeactivateVoucher(t.Context(), DeactivateVoucherParams{VoucherID: v.ID, AuditActorID: "admin-7"}); err != nil {
		t.Fatalf("DeactivateVoucher: %v", err)
	}
	got := e.log.get()[n0:]
	if len(got) != 2 || got[0] != "audit" || got[1] != "deactivate" {
		t.Fatalf("call order = %v, want [audit deactivate]", got)
	}
	evs := e.actions("voucher.deactivate")
	if len(evs) != 1 {
		t.Fatalf("want 1 entry, got %d", len(evs))
	}
	if evs[0].ActorID != "admin-7" || evs[0].TargetType != "voucher" || evs[0].TargetID != "SALE10" {
		t.Fatalf("event = %+v", evs[0])
	}
}

func TestDeactivateVoucher_V21_V21a_auditDownChangesNothing(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	before := e.get(t, v.ID)
	e.failAudit()
	_, err := e.svc.DeactivateVoucher(t.Context(), DeactivateVoucherParams{VoucherID: v.ID, AuditActorID: "admin-1"})
	assertAuditUnavailable(t, err)
	assertSameVoucher(t, e.get(t, v.ID), before)
}

func TestDeactivateVoucher_V21b_auditDownUnknownIdIsNotFound(t *testing.T) {
	e := newAdminEnv(t)
	e.seed(t, "SALE10", 100, 0)
	e.failAudit()
	n0 := len(e.w.got())
	_, err := e.svc.DeactivateVoucher(t.Context(), DeactivateVoucherParams{VoucherID: "no-such-id", AuditActorID: "admin-1"})
	if !errors.Is(err, store.ErrVoucherNotFound) {
		t.Fatalf("err = %v, want ErrVoucherNotFound", err)
	}
	if n := len(e.w.got()); n != n0 {
		t.Fatalf("audit writer called %d times for invalid input", n-n0)
	}
}

func TestUpdateVoucher_V8_V8a_V8b_raiseQuota(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 40)
	got, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, Quota: ptr(int64(150)), AuditActorID: "admin-1"})
	if err != nil {
		t.Fatalf("UpdateVoucher: %v", err)
	}
	if got.Quota != 150 {
		t.Fatalf("quota = %d, want 150", got.Quota)
	}
	assertMoneyFieldsEqual(t, got, v)
	if !got.EndsAt.Equal(v.EndsAt) || got.Used != 40 {
		t.Fatalf("ends_at/used changed: %+v vs %+v", got, v)
	}
	assertMoneyFieldsEqual(t, e.get(t, v.ID), v)
	if n := len(e.actions("voucher.update")); n != 1 {
		t.Fatalf("%d update entries, want 1", n)
	}
}

func TestUpdateVoucher_V9_V9a_moveEndDateReopensWindow(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	newEnd := v.EndsAt.Add(48 * time.Hour)
	got, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, EndsAt: &newEnd, AuditActorID: "admin-1"})
	if err != nil {
		t.Fatalf("UpdateVoucher: %v", err)
	}
	if !got.EndsAt.Equal(newEnd) || got.Quota != 100 {
		t.Fatalf("got %+v, want ends_at %v quota 100", got, newEnd)
	}
	e.svc.nowFn = func() time.Time { return v.EndsAt.Add(time.Hour) }
	res, err := e.svc.ValidateAndReserve(t.Context(), "r-1", "SALE10", "buyer", 100000, "")
	if err != nil {
		t.Fatalf("ValidateAndReserve: %v", err)
	}
	if !res.Valid {
		t.Fatalf("want valid after the old end date, got %+v", res)
	}
}

func TestUpdateVoucher_V9b_V9c_endsAtBoundaryAgainstNow(t *testing.T) {
	cases := []struct {
		name    string
		endsAt  time.Time
		wantErr bool
	}{
		{"before now", adminNow.Add(-time.Second), true},
		{"equal to now", adminNow, true},
		{"one second after now", adminNow.Add(time.Second), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newAdminEnv(t)
			v := e.seed(t, "SALE10", 100, 0)
			endsAt := tc.endsAt
			got, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, EndsAt: &endsAt, AuditActorID: "admin-1"})
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidVoucher) {
					t.Fatalf("err = %v, want ErrInvalidVoucher", err)
				}
				assertSameVoucher(t, e.get(t, v.ID), v)
				if n := e.adminWrites(); n != 0 {
					t.Fatalf("%d audit entries, want 0", n)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateVoucher: %v", err)
			}
			if !got.EndsAt.Equal(tc.endsAt) {
				t.Fatalf("ends_at = %v, want %v", got.EndsAt, tc.endsAt)
			}
		})
	}
}

func TestUpdateVoucher_V13_endsAtNotAfterStartsAtRefused(t *testing.T) {
	cases := map[string]time.Time{
		"equal to starts_at":  adminNow.Add(2 * time.Hour),
		"before starts_at":    adminNow.Add(time.Hour),
		"one ns before start": adminNow.Add(2*time.Hour - time.Nanosecond),
	}
	for name, endsAt := range cases {
		t.Run(name, func(t *testing.T) {
			e := newAdminEnv(t)
			v, err := e.svc.CreateVoucher(t.Context(), CreateVoucherParams{
				Code: "FUT", Scope: 2, DiscountType: discountTypePercent, DiscountValue: 10, Quota: 10,
				StartsAt: adminNow.Add(2 * time.Hour), EndsAt: adminNow.Add(72 * time.Hour), AuditActorID: "admin-1",
			})
			if err != nil {
				t.Fatalf("seed: %v", err)
			}
			ea := endsAt
			_, err = e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, EndsAt: &ea, AuditActorID: "admin-1"})
			if !errors.Is(err, ErrInvalidVoucher) {
				t.Fatalf("err = %v, want ErrInvalidVoucher", err)
			}
			assertSameVoucher(t, e.get(t, v.ID), v)
			if n := e.adminWrites(); n != 0 {
				t.Fatalf("%d audit entries, want 0", n)
			}
		})
	}
}

func TestUpdateVoucher_V10_V10a_V11_V16a_V16b_refusedQuotaWritesNothing(t *testing.T) {
	cases := []struct {
		name        string
		quota, used int64
		current     int64
		request     int64
	}{
		{"V10 below used", 100, 40, 100, 30},
		{"V11 lowered above used", 100, 40, 100, 80},
		{"V11 lowered by one", 100, 40, 100, 99},
		{"V16a quota zero", 100, 0, 100, 0},
		{"V16b unlimited cannot become finite", 0, 0, 0, 500},
		{"negative quota", 100, 0, 100, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newAdminEnv(t)
			v := e.seed(t, "SALE10", tc.current, tc.used)
			_, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, Quota: ptr(tc.request), AuditActorID: "admin-1"})
			if !errors.Is(err, ErrInvalidVoucher) {
				t.Fatalf("err = %v, want ErrInvalidVoucher", err)
			}
			assertSameVoucher(t, e.get(t, v.ID), v)
			if n := e.adminWrites(); n != 0 {
				t.Fatalf("%d audit entries, want 0", n)
			}
		})
	}
}

func TestUpdateVoucher_V12_V12b_V12c_fullyUsedSameQuotaWithEndDate(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 100)
	newEnd := v.EndsAt.Add(24 * time.Hour)
	got, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, Quota: ptr(int64(100)), EndsAt: &newEnd, AuditActorID: "admin-1"})
	if err != nil {
		t.Fatalf("UpdateVoucher: %v", err)
	}
	if got.Quota != 100 || !got.EndsAt.Equal(newEnd) {
		t.Fatalf("got %+v, want quota 100 ends_at %v", got, newEnd)
	}
	if n := len(e.actions("voucher.update")); n != 1 {
		t.Fatalf("%d update entries, want 1", n)
	}
}

func TestUpdateVoucher_V12a_V14_nothingToChange(t *testing.T) {
	cases := []struct {
		name string
		p    UpdateVoucherParams
	}{
		{"V12a quota equal to current", UpdateVoucherParams{Quota: ptr(int64(100))}},
		{"V14 neither field", UpdateVoucherParams{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newAdminEnv(t)
			v := e.seed(t, "SALE10", 100, 0)
			tc.p.VoucherID, tc.p.AuditActorID = v.ID, "admin-1"
			_, err := e.svc.UpdateVoucher(t.Context(), tc.p)
			if !errors.Is(err, ErrInvalidVoucher) {
				t.Fatalf("err = %v, want ErrInvalidVoucher", err)
			}
			if !strings.Contains(err.Error(), "nothing to change") {
				t.Fatalf("err = %q, want message containing %q", err.Error(), "nothing to change")
			}
			assertSameVoucher(t, e.get(t, v.ID), v)
			if n := e.adminWrites(); n != 0 {
				t.Fatalf("%d audit entries, want 0", n)
			}
		})
	}
}

func TestUpdateVoucher_V12d_quotaOneAboveCurrentOnFullyUsed(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 100)
	got, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, Quota: ptr(int64(101)), AuditActorID: "admin-1"})
	if err != nil {
		t.Fatalf("UpdateVoucher: %v", err)
	}
	if got.Quota != 101 {
		t.Fatalf("quota = %d, want 101", got.Quota)
	}
}

func TestUpdateVoucher_V15_inactiveIsFailedPrecondition(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	if _, err := e.svc.DeactivateVoucher(t.Context(), DeactivateVoucherParams{VoucherID: v.ID, AuditActorID: "admin-1"}); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	before := e.get(t, v.ID)
	updatesBefore := len(e.actions("voucher.update"))
	_, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, Quota: ptr(int64(150)), AuditActorID: "admin-1"})
	if !errors.Is(err, store.ErrVoucherInactive) {
		t.Fatalf("err = %v, want ErrVoucherInactive", err)
	}
	assertSameVoucher(t, e.get(t, v.ID), before)
	if n := len(e.actions("voucher.update")); n != updatesBefore {
		t.Fatalf("%d update audit entries for a refused call", n-updatesBefore)
	}
}

func TestUpdateVoucher_V15a_unknownIsNotFoundWithoutAudit(t *testing.T) {
	e := newAdminEnv(t)
	e.seed(t, "SALE10", 100, 0)
	_, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: "no-such-id", Quota: ptr(int64(150)), AuditActorID: "admin-1"})
	if !errors.Is(err, store.ErrVoucherNotFound) {
		t.Fatalf("err = %v, want ErrVoucherNotFound", err)
	}
	if n := e.adminWrites(); n != 0 {
		t.Fatalf("%d audit entries, want 0", n)
	}
}

func TestUpdateVoucher_V16_moneyFieldsNeverAltered(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 10)
	newEnd := v.EndsAt.Add(time.Hour)
	got, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, Quota: ptr(int64(200)), EndsAt: &newEnd, AuditActorID: "admin-1"})
	if err != nil {
		t.Fatalf("UpdateVoucher: %v", err)
	}
	assertMoneyFieldsEqual(t, got, v)
	assertMoneyFieldsEqual(t, e.get(t, v.ID), v)
	if got.ID != v.ID || got.Code != v.Code || !got.StartsAt.Equal(v.StartsAt) || got.Used != v.Used || !got.Active {
		t.Fatalf("identity fields changed: %+v vs %+v", got, v)
	}
}

func TestUpdateVoucher_V16c_unlimitedEndDateCanMove(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 0, 0)
	newEnd := v.EndsAt.Add(24 * time.Hour)
	got, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, EndsAt: &newEnd, AuditActorID: "admin-1"})
	if err != nil {
		t.Fatalf("UpdateVoucher: %v", err)
	}
	if got.Quota != 0 || !got.EndsAt.Equal(newEnd) {
		t.Fatalf("got %+v, want quota 0 ends_at %v", got, newEnd)
	}
}

func TestUpdateVoucher_V16d_V16e_lostRaceIsInvalidKeepsAuditAndNeverBelowUsed(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 40)
	e.rep.beforeUpdate = func() {
		if err := e.mem.IncrementUsed(context.Background(), v.ID, 50); err != nil {
			t.Errorf("concurrent commit: %v", err)
		}
	}
	_, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, Quota: ptr(int64(80)), AuditActorID: "admin-1"})
	if !errors.Is(err, ErrInvalidVoucher) {
		t.Fatalf("err = %v, want ErrInvalidVoucher", err)
	}
	stored := e.get(t, v.ID)
	if stored.Quota < stored.Used || stored.Quota != 100 {
		t.Fatalf("stored quota=%d used=%d, want quota 100 and >= used", stored.Quota, stored.Used)
	}
	if n := len(e.actions("voucher.update")); n != 1 {
		t.Fatalf("%d update audit entries, want the attempt recorded once", n)
	}
}

func TestUpdateVoucher_V16f_deactivateBetweenValidationAndUpdateIsFailedPrecondition(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	e.rep.beforeUpdate = func() {
		if _, err := e.mem.Deactivate(context.Background(), v.ID, adminNow); err != nil {
			t.Errorf("concurrent deactivate: %v", err)
		}
	}
	_, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, Quota: ptr(int64(150)), AuditActorID: "admin-1"})
	if !errors.Is(err, store.ErrVoucherInactive) {
		t.Fatalf("err = %v, want ErrVoucherInactive", err)
	}
	if stored := e.get(t, v.ID); stored.Quota != 100 {
		t.Fatalf("quota = %d, want 100 unchanged", stored.Quota)
	}
}

func TestUpdateVoucher_V16g_errorPrecedence(t *testing.T) {
	t.Run("not found beats invalid argument", func(t *testing.T) {
		e := newAdminEnv(t)
		_, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: "no-such-id", Quota: ptr(int64(0)), AuditActorID: "admin-1"})
		if !errors.Is(err, store.ErrVoucherNotFound) {
			t.Fatalf("err = %v, want ErrVoucherNotFound", err)
		}
	})
	t.Run("inactive beats invalid argument", func(t *testing.T) {
		e := newAdminEnv(t)
		v := e.seed(t, "SALE10", 100, 0)
		if _, err := e.svc.DeactivateVoucher(t.Context(), DeactivateVoucherParams{VoucherID: v.ID, AuditActorID: "admin-1"}); err != nil {
			t.Fatalf("deactivate: %v", err)
		}
		_, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, Quota: ptr(int64(0)), AuditActorID: "admin-1"})
		if !errors.Is(err, store.ErrVoucherInactive) {
			t.Fatalf("err = %v, want ErrVoucherInactive", err)
		}
	})
	t.Run("inactive beats nothing to change", func(t *testing.T) {
		e := newAdminEnv(t)
		v := e.seed(t, "SALE10", 100, 0)
		if _, err := e.svc.DeactivateVoucher(t.Context(), DeactivateVoucherParams{VoucherID: v.ID, AuditActorID: "admin-1"}); err != nil {
			t.Fatalf("deactivate: %v", err)
		}
		_, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, AuditActorID: "admin-1"})
		if !errors.Is(err, store.ErrVoucherInactive) {
			t.Fatalf("err = %v, want ErrVoucherInactive", err)
		}
	})
}

func TestUpdateVoucher_V22_auditBeforeWriteWithActorAndCode(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	n0 := len(e.log.get())
	if _, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, Quota: ptr(int64(150)), AuditActorID: "admin-7"}); err != nil {
		t.Fatalf("UpdateVoucher: %v", err)
	}
	got := e.log.get()[n0:]
	if len(got) != 2 || got[0] != "audit" || got[1] != "update" {
		t.Fatalf("call order = %v, want [audit update]", got)
	}
	evs := e.actions("voucher.update")
	if len(evs) != 1 {
		t.Fatalf("want 1 entry, got %d", len(evs))
	}
	if evs[0].ActorID != "admin-7" || evs[0].TargetType != "voucher" || evs[0].TargetID != "SALE10" {
		t.Fatalf("event = %+v", evs[0])
	}
}

func TestUpdateVoucher_V21_V21a_auditDownChangesNothing(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	before := e.get(t, v.ID)
	e.failAudit()
	_, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, Quota: ptr(int64(150)), AuditActorID: "admin-1"})
	assertAuditUnavailable(t, err)
	assertSameVoucher(t, e.get(t, v.ID), before)
}

func TestUpdateVoucher_V21b_auditDownInvalidInputKeepsItsOwnError(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	before := e.get(t, v.ID)
	e.failAudit()
	n0 := len(e.w.got())
	_, err := e.svc.UpdateVoucher(t.Context(), UpdateVoucherParams{VoucherID: v.ID, Quota: ptr(int64(0)), AuditActorID: "admin-1"})
	if !errors.Is(err, ErrInvalidVoucher) {
		t.Fatalf("err = %v, want ErrInvalidVoucher", err)
	}
	if n := len(e.w.got()); n != n0 {
		t.Fatalf("audit writer called %d times for invalid input", n-n0)
	}
	assertSameVoucher(t, e.get(t, v.ID), before)
}

func TestCreateVoucher_V18_V18a_duplicateCodeRefusedWithoutAudit(t *testing.T) {
	for name, code := range map[string]string{"exact": "SALE10", "padded": "  SALE10  "} {
		t.Run(name, func(t *testing.T) {
			e := newAdminEnv(t)
			v := e.seed(t, "SALE10", 100, 0)
			creates := len(e.actions("voucher.platform_create"))
			p := platformParams(code)
			_, err := e.svc.CreateVoucher(t.Context(), p)
			if !errors.Is(err, store.ErrDuplicateCode) {
				t.Fatalf("err = %v, want ErrDuplicateCode", err)
			}
			if n := len(e.actions("voucher.platform_create")); n != creates {
				t.Fatalf("%d create audit entries for a duplicate, want none", n-creates)
			}
			assertSameVoucher(t, e.get(t, v.ID), v)
		})
	}
}

func TestCreateVoucher_V18a_V21b_auditDownDuplicateIsAlreadyExists(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seed(t, "SALE10", 100, 0)
	e.failAudit()
	n0 := len(e.w.got())
	_, err := e.svc.CreateVoucher(t.Context(), platformParams("SALE10"))
	if !errors.Is(err, store.ErrDuplicateCode) {
		t.Fatalf("err = %v, want ErrDuplicateCode", err)
	}
	if n := len(e.w.got()); n != n0 {
		t.Fatalf("audit writer called %d times for a duplicate", n-n0)
	}
	assertSameVoucher(t, e.get(t, v.ID), v)
}

func TestCreateVoucher_V21_auditDownNewCodeCreatesNothing(t *testing.T) {
	e := newAdminEnv(t)
	e.failAudit()
	_, err := e.svc.CreateVoucher(t.Context(), platformParams("NEWCODE"))
	assertAuditUnavailable(t, err)
	assertVoucherAbsent(t, e.mem, "NEWCODE")
}
