package voucher

import (
	"context"
	"errors"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

type DeactivateVoucherParams struct {
	VoucherID    string
	AuditActorID string
}

type UpdateVoucherParams struct {
	VoucherID    string
	EndsAt       *time.Time
	Quota        *int64
	AuditActorID string
}

var errTestStub = errors.New("test stub: not implemented")

func (s *Service) DeactivateVoucher(ctx context.Context, p DeactivateVoucherParams) (store.Voucher, error) {
	return store.Voucher{}, errTestStub
}

func (s *Service) UpdateVoucher(ctx context.Context, p UpdateVoucherParams) (store.Voucher, error) {
	return store.Voucher{}, errTestStub
}
