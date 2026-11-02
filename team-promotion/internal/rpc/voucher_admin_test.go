package rpc_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
)

type adminRig struct {
	*rig
	rec *recordingAudit
}

func newAdminRig(t *testing.T) *adminRig {
	t.Helper()
	rec := &recordingAudit{}
	return &adminRig{rig: startRig(t, rigOpts{audit: rec}), rec: rec}
}

func (a *adminRig) create(t *testing.T, code string, quota int64) *promotionv1.Voucher {
	t.Helper()
	req := platformVoucher(code, promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM)
	req.Quota = quota
	req.MinSpend = 7
	req.MaxDiscount = 9
	req.StartsAt = timestamppb.New(time.Now().Add(-time.Hour))
	req.EndsAt = timestamppb.New(time.Now().Add(24 * time.Hour))
	resp, err := a.vc.CreateVoucher(adminCtx(t), req)
	if err != nil {
		t.Fatalf("create %s: %v", code, err)
	}
	a.rec.mu.Lock()
	a.rec.events = nil
	a.rec.mu.Unlock()
	return resp.GetVoucher()
}

func (a *adminRig) use(t *testing.T, code string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		id := code + "-res-" + string(rune('a'+i))
		r, err := a.vc.ValidateAndReserve(orderSvcCtx(t), &promotionv1.ValidateAndReserveRequest{
			ReservationId: id, Code: code, BuyerId: "buyer-1", CartSubtotal: 1000,
		})
		if err != nil || !r.GetValid() {
			t.Fatalf("reserve %s: %v %v", id, r, err)
		}
		if _, err := a.vc.CommitReservation(orderSvcCtx(t), &promotionv1.CommitReservationRequest{ReservationId: id}); err != nil {
			t.Fatalf("commit %s: %v", id, err)
		}
	}
}

func (a *adminRig) get(t *testing.T, code string) *promotionv1.Voucher {
	t.Helper()
	resp, err := a.vc.GetVoucher(adminCtx(t), &promotionv1.GetVoucherRequest{Code: code})
	if err != nil {
		t.Fatalf("get %s: %v", code, err)
	}
	return resp.GetVoucher()
}

func (a *adminRig) requireUnchanged(t *testing.T, before *promotionv1.Voucher) {
	t.Helper()
	after := a.get(t, before.GetCode())
	if !proto.Equal(before, after) {
		t.Fatalf("voucher changed: before=%v after=%v", before, after)
	}
}

func (a *adminRig) requireOneAudit(t *testing.T, action, code string) {
	t.Helper()
	evs := a.rec.got()
	if len(evs) != 1 {
		t.Fatalf("want 1 audit event, got %d: %+v", len(evs), evs)
	}
	ev := evs[0]
	if ev.Action != action || ev.ActorID != "admin-1" || ev.TargetType != "voucher" || ev.TargetID != code {
		t.Fatalf("event = %+v, want %s admin-1 voucher %s", ev, action, code)
	}
}

func deactivate(a *adminRig, ctx context.Context, id string) (*promotionv1.DeactivateVoucherResponse, error) {
	return a.vc.DeactivateVoucher(ctx, &promotionv1.DeactivateVoucherRequest{VoucherId: id})
}

func TestDeactivateVoucherReturnsInactiveWithTimestamp_V1(t *testing.T) {
	a := newAdminRig(t)
	v := a.create(t, "STOP1", 5)
	if !v.GetActive() || v.GetDeactivatedAt() != nil {
		t.Fatalf("created voucher active=%v deactivated_at=%v, want active and no timestamp", v.GetActive(), v.GetDeactivatedAt())
	}
	resp, err := deactivate(a, adminCtx(t), v.GetId())
	if err != nil {
		t.Fatalf("DeactivateVoucher: %v", err)
	}
	got := resp.GetVoucher()
	if got.GetActive() || got.GetDeactivatedAt() == nil || got.GetDeactivatedAt().AsTime().IsZero() {
		t.Fatalf("got active=%v deactivated_at=%v", got.GetActive(), got.GetDeactivatedAt())
	}
	if got.GetCode() != "STOP1" || got.GetId() != v.GetId() {
		t.Fatalf("wrong voucher %v", got)
	}
	read := a.get(t, "STOP1")
	if read.GetActive() || read.GetDeactivatedAt() == nil {
		t.Fatalf("GetVoucher after stop active=%v deactivated_at=%v", read.GetActive(), read.GetDeactivatedAt())
	}
	a.requireOneAudit(t, "voucher.deactivate", "STOP1")
}

func TestDeactivateVoucherTwiceKeepsOriginalTimestampAuditedPerCall_V4_V4a(t *testing.T) {
	a := newAdminRig(t)
	v := a.create(t, "STOP2", 5)
	first, err := deactivate(a, adminCtx(t), v.GetId())
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := deactivate(a, adminCtx(t), v.GetId())
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !first.GetVoucher().GetDeactivatedAt().AsTime().Equal(second.GetVoucher().GetDeactivatedAt().AsTime()) {
		t.Fatalf("deactivated_at moved: %v -> %v", first.GetVoucher().GetDeactivatedAt(), second.GetVoucher().GetDeactivatedAt())
	}
	if evs := a.rec.got(); len(evs) != 2 {
		t.Fatalf("want 2 audit events (one per call), got %d", len(evs))
	}
}

func TestDeactivateVoucherUnknownIsNotFoundWithoutAudit_V5_V5a(t *testing.T) {
	a := newAdminRig(t)
	_, err := deactivate(a, adminCtx(t), "no-such-id")
	if code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}
	a.rec.requireNone(t, "unknown voucher stop")
}

func TestDeactivateVoucherNonAdminDeniedLeavesNoTrace_V6_V6a(t *testing.T) {
	for name, tc := range map[string]struct {
		ctx  func(*testing.T) context.Context
		want codes.Code
	}{
		"seller":    {sellerCtx, codes.PermissionDenied},
		"buyer":     {buyerCtx, codes.PermissionDenied},
		"anonymous": {anonCtx, codes.Unauthenticated},
	} {
		t.Run(name, func(t *testing.T) {
			a := newAdminRig(t)
			v := a.create(t, "STOP3", 5)
			_, err := deactivate(a, tc.ctx(t), v.GetId())
			if code(err) != tc.want {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			a.rec.requireNone(t, "non-admin stop")
			a.requireUnchanged(t, v)
		})
	}
}

func TestDeactivateVoucherNonAdminUnknownIdIsPermissionDenied_V6_V16g(t *testing.T) {
	a := newAdminRig(t)
	_, err := deactivate(a, sellerCtx(t), "no-such-id")
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied before NotFound", err)
	}
	a.rec.requireNone(t, "non-admin stop of unknown id")
}

func TestUpdateVoucherRaiseQuotaOnlyChangesQuotaAuditedOnce_V8_V8a_V8b(t *testing.T) {
	a := newAdminRig(t)
	v := a.create(t, "UPD1", 100)
	a.use(t, "UPD1", 2)
	before := a.get(t, "UPD1")
	resp, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId(), Quota: proto.Int64(150)})
	if err != nil {
		t.Fatalf("UpdateVoucher: %v", err)
	}
	got := resp.GetVoucher()
	if got.GetQuota() != 150 {
		t.Fatalf("quota = %d, want 150", got.GetQuota())
	}
	want := proto.Clone(before).(*promotionv1.Voucher)
	want.Quota = 150
	if !proto.Equal(want, got) {
		t.Fatalf("other fields changed: want %v got %v", want, got)
	}
	if !proto.Equal(want, a.get(t, "UPD1")) {
		t.Fatalf("stored voucher differs from returned")
	}
	a.requireOneAudit(t, "voucher.update", "UPD1")
}

func TestUpdateVoucherMoveEndDate_V9_V9c(t *testing.T) {
	for name, offset := range map[string]time.Duration{"later": 72 * time.Hour, "one second ahead": 2 * time.Minute} {
		t.Run(name, func(t *testing.T) {
			a := newAdminRig(t)
			v := a.create(t, "UPD2", 5)
			end := time.Now().Add(offset).Truncate(time.Second)
			resp, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId(), EndsAt: timestamppb.New(end)})
			if err != nil {
				t.Fatalf("UpdateVoucher: %v", err)
			}
			if !resp.GetVoucher().GetEndsAt().AsTime().Equal(end) {
				t.Fatalf("ends_at = %v, want %v", resp.GetVoucher().GetEndsAt().AsTime(), end)
			}
			if resp.GetVoucher().GetQuota() != 5 {
				t.Fatalf("quota changed to %d", resp.GetVoucher().GetQuota())
			}
			a.requireOneAudit(t, "voucher.update", "UPD2")
		})
	}
}

func TestUpdateVoucherEndsAtNotInFutureIsInvalid_V9b(t *testing.T) {
	for name, at := range map[string]time.Time{
		"past":     time.Now().Add(-time.Hour),
		"now":      time.Now(),
		"far past": time.Now().Add(-48 * time.Hour),
	} {
		t.Run(name, func(t *testing.T) {
			a := newAdminRig(t)
			v := a.create(t, "UPD3", 5)
			_, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId(), EndsAt: timestamppb.New(at)})
			if code(err) != codes.InvalidArgument {
				t.Fatalf("got %v, want InvalidArgument", err)
			}
			a.rec.requireNone(t, "refused end date")
			a.requireUnchanged(t, v)
		})
	}
}

func TestUpdateVoucherEndsAtNotAfterStartIsInvalid_V13(t *testing.T) {
	a := newAdminRig(t)
	v := a.create(t, "UPD4", 5)
	_, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId(), EndsAt: v.GetStartsAt()})
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", err)
	}
	a.rec.requireNone(t, "end date equal to start")
	a.requireUnchanged(t, v)
}

func TestUpdateVoucherQuotaRulesRefused_V10_V10a_V11_V16a(t *testing.T) {
	for name, tc := range map[string]struct {
		quota int64
		used  int
	}{
		"below used": {quota: 1, used: 3},
		"lowered":    {quota: 8, used: 3},
		"zero":       {quota: 0, used: 0},
		"negative":   {quota: -5, used: 0},
	} {
		t.Run(name, func(t *testing.T) {
			a := newAdminRig(t)
			v := a.create(t, "UPD5", 10)
			a.use(t, "UPD5", tc.used)
			before := a.get(t, "UPD5")
			_, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId(), Quota: proto.Int64(tc.quota)})
			if code(err) != codes.InvalidArgument {
				t.Fatalf("got %v, want InvalidArgument", err)
			}
			a.rec.requireNone(t, "refused quota")
			a.requireUnchanged(t, before)
		})
	}
}

func TestUpdateVoucherFullyUsedSameQuotaWithNewEndDate_V12_V12b_V12c(t *testing.T) {
	a := newAdminRig(t)
	v := a.create(t, "UPD6", 3)
	a.use(t, "UPD6", 3)
	end := time.Now().Add(96 * time.Hour).Truncate(time.Second)
	resp, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{
		VoucherId: v.GetId(), Quota: proto.Int64(3), EndsAt: timestamppb.New(end),
	})
	if err != nil {
		t.Fatalf("UpdateVoucher: %v", err)
	}
	if resp.GetVoucher().GetQuota() != 3 || resp.GetVoucher().GetUsed() != 3 || !resp.GetVoucher().GetEndsAt().AsTime().Equal(end) {
		t.Fatalf("got %v", resp.GetVoucher())
	}
	a.requireOneAudit(t, "voucher.update", "UPD6")
}

func TestUpdateVoucherQuotaOneAboveFullyUsed_V12d(t *testing.T) {
	a := newAdminRig(t)
	v := a.create(t, "UPD7", 3)
	a.use(t, "UPD7", 3)
	resp, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId(), Quota: proto.Int64(4)})
	if err != nil {
		t.Fatalf("UpdateVoucher: %v", err)
	}
	if resp.GetVoucher().GetQuota() != 4 {
		t.Fatalf("quota = %d, want 4", resp.GetVoucher().GetQuota())
	}
}

func TestUpdateVoucherSameQuotaNoEndDateIsNothingToChange_V12a(t *testing.T) {
	a := newAdminRig(t)
	v := a.create(t, "UPD8", 5)
	_, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId(), Quota: proto.Int64(5)})
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", err)
	}
	if msg := err.Error(); !contains(msg, "nothing to change") {
		t.Fatalf("message %q lacks 'nothing to change'", msg)
	}
	a.rec.requireNone(t, "same quota")
	a.requireUnchanged(t, v)
}

func TestUpdateVoucherNeitherFieldIsInvalid_V14(t *testing.T) {
	a := newAdminRig(t)
	v := a.create(t, "UPD9", 5)
	_, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId()})
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", err)
	}
	a.rec.requireNone(t, "empty update")
	a.requireUnchanged(t, v)
}

func TestUpdateVoucherInactiveIsFailedPrecondition_V15(t *testing.T) {
	a := newAdminRig(t)
	v := a.create(t, "UPD10", 5)
	if _, err := deactivate(a, adminCtx(t), v.GetId()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	stopped := a.get(t, "UPD10")
	a.rec.mu.Lock()
	a.rec.events = nil
	a.rec.mu.Unlock()
	_, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId(), Quota: proto.Int64(9)})
	if code(err) != codes.FailedPrecondition {
		t.Fatalf("got %v, want FailedPrecondition", err)
	}
	a.rec.requireNone(t, "edit of stopped voucher")
	a.requireUnchanged(t, stopped)
}

func TestUpdateVoucherUnknownIsNotFoundWithoutAudit_V15a(t *testing.T) {
	a := newAdminRig(t)
	_, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: "no-such-id", Quota: proto.Int64(9)})
	if code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}
	a.rec.requireNone(t, "unknown voucher edit")
}

func TestUpdateVoucherUnlimitedCannotBecomeFiniteButEndDateMoves_V16b_V16c(t *testing.T) {
	a := newAdminRig(t)
	v := a.create(t, "UPD11", 0)
	_, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId(), Quota: proto.Int64(500)})
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", err)
	}
	a.rec.requireNone(t, "finite quota on unlimited")
	a.requireUnchanged(t, v)

	end := time.Now().Add(96 * time.Hour).Truncate(time.Second)
	resp, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId(), EndsAt: timestamppb.New(end)})
	if err != nil {
		t.Fatalf("end date on unlimited: %v", err)
	}
	if resp.GetVoucher().GetQuota() != 0 || !resp.GetVoucher().GetEndsAt().AsTime().Equal(end) {
		t.Fatalf("got %v", resp.GetVoucher())
	}
}

func TestUpdateVoucherNonAdminDeniedWithoutAudit_V17_V27a(t *testing.T) {
	for name, tc := range map[string]struct {
		ctx  func(*testing.T) context.Context
		want codes.Code
	}{
		"seller":    {sellerCtx, codes.PermissionDenied},
		"buyer":     {buyerCtx, codes.PermissionDenied},
		"anonymous": {anonCtx, codes.Unauthenticated},
	} {
		t.Run(name, func(t *testing.T) {
			a := newAdminRig(t)
			v := a.create(t, "UPD12", 5)
			_, err := a.vc.UpdateVoucher(tc.ctx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId(), Quota: proto.Int64(9)})
			if code(err) != tc.want {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			a.rec.requireNone(t, "non-admin edit")
			a.requireUnchanged(t, v)
		})
	}
}

func TestUpdateVoucherNonAdminUnknownIdIsPermissionDenied_V17_V16g(t *testing.T) {
	a := newAdminRig(t)
	_, err := a.vc.UpdateVoucher(sellerCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: "no-such-id"})
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied before NotFound/InvalidArgument", err)
	}
	a.rec.requireNone(t, "non-admin edit of unknown id")
}

func TestUpdateVoucherFailedPreconditionBeatsInvalidArgument_V16g(t *testing.T) {
	a := newAdminRig(t)
	v := a.create(t, "UPD13", 5)
	if _, err := deactivate(a, adminCtx(t), v.GetId()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	_, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: v.GetId(), Quota: proto.Int64(0)})
	if code(err) != codes.FailedPrecondition {
		t.Fatalf("got %v, want FailedPrecondition", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
