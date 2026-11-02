package rpc_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
)

type duplicateOnCreate struct {
	store.VoucherRepository
}

func (duplicateOnCreate) Create(context.Context, store.Voucher, func(store.Voucher) (outbox.Row, error)) (store.Voucher, error) {
	return store.Voucher{}, fmt.Errorf("insert voucher: %w", store.ErrDuplicateCode)
}

func assertDuplicateCodeStatus(t *testing.T, err error, value string) {
	t.Helper()
	st := status.Convert(err)
	if st.Code() != codes.AlreadyExists {
		t.Fatalf("got %v %q, want AlreadyExists", st.Code(), st.Message())
	}
	if !strings.Contains(st.Message(), "code") {
		t.Fatalf("message %q does not name the code field", st.Message())
	}
	for _, leak := range []string{value, "vouchers_code", "23505", "constraint"} {
		if strings.Contains(st.Message(), leak) {
			t.Fatalf("message leaks %q: %q", leak, st.Message())
		}
	}
}

func TestCreateVoucherTypedDuplicateMapsToAlreadyExistsNamingCode_V18_V18c(t *testing.T) {
	r := startRig(t, rigOpts{vouchers: duplicateOnCreate{store.VoucherRepository(nil)}})
	_, err := r.vc.CreateVoucher(adminCtx(t), platformVoucher("RACE-LOSER", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM))
	assertDuplicateCodeStatus(t, err, "RACE-LOSER")
}

func TestCreateVoucherTakenCodeNamesCodeField_V18(t *testing.T) {
	pool, ob := newPostgres(t)
	r := startRig(t, rigOpts{
		vouchers:     store.NewPostgresVoucherRepository(pool, ob),
		reservations: store.NewPostgresReservationRepository(pool),
		voucherEvent: realVoucherEvent,
	})
	req := platformVoucher("DUP-NAMED", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM)
	if _, err := r.vc.CreateVoucher(adminCtx(t), req); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := r.vc.CreateVoucher(adminCtx(t), req)
	assertDuplicateCodeStatus(t, err, "DUP-NAMED")
}
