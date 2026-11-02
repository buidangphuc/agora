package rpc_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
)

func (a *adminRig) auditDown(err error) {
	a.rec.mu.Lock()
	a.rec.err = err
	a.rec.events = nil
	a.rec.mu.Unlock()
}

func requireAuditUnavailableInfo(t *testing.T, err error) {
	t.Helper()
	requireAuditUnavailable(t, err)
	var found bool
	for _, d := range status.Convert(err).Details() {
		info, ok := d.(interface {
			GetDomain() string
			GetReason() string
		})
		if !ok {
			continue
		}
		found = true
		if info.GetDomain() != "promotion.platform" || info.GetReason() != "AUDIT_UNAVAILABLE" {
			t.Fatalf("ErrorInfo = %s/%s, want promotion.platform/AUDIT_UNAVAILABLE", info.GetDomain(), info.GetReason())
		}
	}
	if !found {
		t.Fatalf("no ErrorInfo detail on %v", err)
	}
}

func TestAdminVoucherWritesAuditDownUnavailableWithReason_V21_V21a(t *testing.T) {
	downs := map[string]error{
		"refused": errors.New("dial tcp 10.0.0.9:50066: connection refused"),
		"timeout": context.DeadlineExceeded,
	}
	writes := map[string]func(*testing.T, *adminRig, string) error{
		"create": func(t *testing.T, a *adminRig, _ string) error {
			_, err := a.vc.CreateVoucher(adminCtx(t), platformVoucher("V21-NEW", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM))
			return err
		},
		"deactivate": func(t *testing.T, a *adminRig, id string) error {
			_, err := deactivate(a, adminCtx(t), id)
			return err
		},
		"update quota": func(t *testing.T, a *adminRig, id string) error {
			_, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: id, Quota: proto.Int64(500)})
			return err
		},
	}
	for dn, down := range downs {
		for wn, write := range writes {
			t.Run(dn+"/"+wn, func(t *testing.T) {
				a := newAdminRig(t)
				v := a.create(t, "V21-OLD", 100)
				a.auditDown(down)
				err := write(t, a, v.GetId())
				requireAuditUnavailableInfo(t, err)
				a.requireUnchanged(t, v)
				if wn == "create" {
					if _, err := a.vc.GetVoucher(adminCtx(t), &promotionv1.GetVoucherRequest{Code: "V21-NEW"}); code(err) != codes.NotFound {
						t.Fatalf("voucher must not exist, GetVoucher = %v", code(err))
					}
				}
			})
		}
	}
}

func TestAdminVoucherWritesAuditDownInvalidInputKeepsOwnCode_V21b(t *testing.T) {
	cases := map[string]struct {
		want  codes.Code
		write func(*testing.T, *adminRig, string) error
	}{
		"update quota zero": {codes.InvalidArgument, func(t *testing.T, a *adminRig, id string) error {
			_, err := a.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: id, Quota: proto.Int64(0)})
			return err
		}},
		"create taken code": {codes.AlreadyExists, func(t *testing.T, a *adminRig, _ string) error {
			_, err := a.vc.CreateVoucher(adminCtx(t), platformVoucher("V21B-OLD", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM))
			return err
		}},
		"deactivate unknown id": {codes.NotFound, func(t *testing.T, a *adminRig, _ string) error {
			_, err := deactivate(a, adminCtx(t), "no-such-id")
			return err
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			a := newAdminRig(t)
			v := a.create(t, "V21B-OLD", 100)
			a.auditDown(errors.New("connection refused"))
			err := tc.write(t, a, v.GetId())
			st := status.Convert(err)
			if st.Code() != tc.want {
				t.Fatalf("got %v %q, want %v", st.Code(), st.Message(), tc.want)
			}
			if st.Message() == auditx.Message {
				t.Fatalf("audit message leaked into %v", tc.want)
			}
			for _, d := range st.Details() {
				if info, ok := d.(interface{ GetReason() string }); ok && info.GetReason() == "AUDIT_UNAVAILABLE" {
					t.Fatalf("AUDIT_UNAVAILABLE reason on %v", tc.want)
				}
			}
			a.rec.requireNone(t, name)
			a.requireUnchanged(t, v)
		})
	}
}
