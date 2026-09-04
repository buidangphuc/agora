package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/buidangphuc/team-promotion/internal/bootstrap"
	"github.com/buidangphuc/team-promotion/internal/featureflags"
	"github.com/buidangphuc/team-promotion/internal/producer"
	"github.com/buidangphuc/team-promotion/internal/repository"
)

// Discount type / scope enum values (mirror platform.promotion.v1 without importing
// the wire enum into the domain logic).
const (
	discountTypePercent int32 = 1
	discountTypeFixed   int32 = 2

	voucherScopeShop int32 = 1
)

const defaultPageSize = 50

// VoucherService holds the voucher CRUD + redemption business logic.
type VoucherService struct {
	vouchers     repository.VoucherRepository
	reservations repository.ReservationRepository
	emitter      *producer.Emitter
	flags        featureflags.Evaluator
	logger       *slog.Logger
	nowFn        func() time.Time
}

// NewVoucherService wires the voucher + reservation repositories, the
// promotion.events producer (may be nil when Kafka is disabled) and the feature
// flag evaluator.
func NewVoucherService(
	vouchers repository.VoucherRepository,
	reservations repository.ReservationRepository,
	prod *bootstrap.EventProducer,
	flags featureflags.Evaluator,
	logger *slog.Logger,
) *VoucherService {
	if logger == nil {
		logger = slog.Default()
	}
	// Convert the concrete producer to the Publisher seam, mapping a nil *producer
	// to a nil interface so the emitter no-ops cleanly when Kafka is disabled.
	var pub producer.Publisher
	if prod != nil {
		pub = prod
	}
	return &VoucherService{
		vouchers:     vouchers,
		reservations: reservations,
		emitter:      producer.NewEmitter(pub, logger),
		flags:        flags,
		logger:       logger,
		nowFn:        time.Now,
	}
}

// CreateVoucherParams is the validated input for CreateVoucher. SellerID is set by
// the caller for shop-scoped vouchers (derived from the authenticated seller
// principal — CreateVoucherRequest carries no seller_id on the wire).
type CreateVoucherParams struct {
	Code          string
	Scope         int32
	SellerID      string
	DiscountType  int32
	DiscountValue int64
	MinSpend      int64
	MaxDiscount   int64
	Quota         int64
	StartsAt      time.Time
	EndsAt        time.Time
}

// ErrInvalidVoucher is returned for malformed CreateVoucher input.
var ErrInvalidVoucher = errors.New("invalid voucher")

// CreateVoucher persists a voucher and emits VoucherChanged on promotion.events.
func (s *VoucherService) CreateVoucher(ctx context.Context, p CreateVoucherParams) (repository.Voucher, error) {
	if strings.TrimSpace(p.Code) == "" {
		return repository.Voucher{}, ErrInvalidVoucher
	}
	v := repository.Voucher{
		Code:          strings.TrimSpace(p.Code),
		Scope:         p.Scope,
		SellerID:      p.SellerID,
		DiscountType:  p.DiscountType,
		DiscountValue: p.DiscountValue,
		MinSpend:      p.MinSpend,
		MaxDiscount:   p.MaxDiscount,
		Quota:         p.Quota,
		StartsAt:      p.StartsAt,
		EndsAt:        p.EndsAt,
	}
	created, err := s.vouchers.Create(ctx, v)
	if err != nil {
		return repository.Voucher{}, err
	}
	if err := s.emitter.EmitVoucherChanged(ctx, VoucherToProto(created)); err != nil {
		// Emission is best-effort: a Kafka blip must not fail the write. Log and move on.
		s.logger.Warn("emit VoucherChanged failed", slog.String("voucher_id", created.ID), slog.Any("err", err))
	}
	return created, nil
}

// GetVoucher looks a voucher up by code.
func (s *VoucherService) GetVoucher(ctx context.Context, code string) (repository.Voucher, error) {
	return s.vouchers.GetByCode(ctx, code)
}

// ListVouchers returns a page of vouchers for a seller (empty = platform-scoped).
// The cursor is a stringified offset; nextCursor is empty when the page is last.
func (s *VoucherService) ListVouchers(ctx context.Context, sellerID, cursor string, pageSize int32) ([]repository.Voucher, string, error) {
	limit := int(pageSize)
	if limit <= 0 || limit > 200 {
		limit = defaultPageSize
	}
	offset := 0
	if cursor != "" {
		if n, err := strconv.Atoi(cursor); err == nil && n >= 0 {
			offset = n
		}
	}
	// Fetch one extra to detect whether a further page exists.
	items, err := s.vouchers.List(ctx, sellerID, limit+1, offset)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = strconv.Itoa(offset + limit)
	}
	return items, next, nil
}

// ReserveResult is the outcome of ValidateAndReserve.
type ReserveResult struct {
	Valid          bool
	Reason         string
	DiscountAmount int64
	VoucherID      string
}

// ValidateAndReserve validates a voucher against a cart and places an idempotent
// hold keyed on reservationID. A retry with the same reservationID returns the
// original discount without decrementing quota or creating a second hold
// (ADR-0008). A rejection returns Valid=false with a human-readable reason and
// never creates a hold.
func (s *VoucherService) ValidateAndReserve(ctx context.Context, reservationID, code, buyerID string, cartSubtotal int64, sellerID string) (ReserveResult, error) {
	if strings.TrimSpace(reservationID) == "" {
		return ReserveResult{Valid: false, Reason: "reservation_id is required"}, nil
	}

	// Idempotency: an existing hold for this reservation_id wins — return its
	// original discount regardless of the (possibly re-sent) request fields.
	existing, err := s.reservations.GetByReservationID(ctx, reservationID)
	if err == nil {
		if existing.Status == repository.ReservationReleased {
			return ReserveResult{Valid: false, Reason: "reservation already released"}, nil
		}
		return ReserveResult{Valid: true, DiscountAmount: existing.DiscountAmount, VoucherID: existing.VoucherID}, nil
	}
	if !errors.Is(err, repository.ErrReservationNotFound) {
		return ReserveResult{}, err
	}

	if strings.TrimSpace(code) == "" {
		return ReserveResult{Valid: false, Reason: "voucher code is required"}, nil
	}

	voucher, err := s.vouchers.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrVoucherNotFound) {
			return ReserveResult{Valid: false, Reason: "voucher code not found"}, nil
		}
		return ReserveResult{}, err
	}

	if reason, ok := s.validate(voucher, cartSubtotal, sellerID); !ok {
		return ReserveResult{Valid: false, Reason: reason}, nil
	}

	discount := computeDiscount(voucher, cartSubtotal)

	// Place the hold. Create is idempotent on reservation_id, so a race that
	// inserts concurrently still yields a single hold with a single discount.
	stored, _, err := s.reservations.Create(ctx, repository.Reservation{
		ReservationID:  reservationID,
		VoucherID:      voucher.ID,
		BuyerID:        buyerID,
		DiscountAmount: discount,
		Status:         repository.ReservationReserved,
	})
	if err != nil {
		return ReserveResult{}, err
	}
	return ReserveResult{Valid: true, DiscountAmount: stored.DiscountAmount, VoucherID: stored.VoucherID}, nil
}

// validate applies the rejection rules; returns (reason, false) on the first miss.
func (s *VoucherService) validate(v repository.Voucher, subtotal int64, sellerID string) (string, bool) {
	now := s.nowFn()
	if !v.StartsAt.IsZero() && now.Before(v.StartsAt) {
		return "voucher not yet active", false
	}
	if !v.EndsAt.IsZero() && !now.Before(v.EndsAt) {
		return "voucher expired", false
	}
	if subtotal < v.MinSpend {
		return "order subtotal below minimum spend", false
	}
	if v.Quota > 0 && v.Used >= v.Quota {
		return "voucher quota exhausted", false
	}
	if v.Scope == voucherScopeShop && v.SellerID != "" && v.SellerID != sellerID {
		return "voucher not valid for this seller", false
	}
	return "", true
}

// computeDiscount applies the discount math, clamped to never exceed the subtotal.
//
//	PERCENT → min(subtotal*value/100, max_discount>0 ? max_discount : ∞)
//	FIXED   → min(value, subtotal)
func computeDiscount(v repository.Voucher, subtotal int64) int64 {
	var d int64
	switch v.DiscountType {
	case discountTypePercent:
		d = subtotal * v.DiscountValue / 100
		if v.MaxDiscount > 0 && d > v.MaxDiscount {
			d = v.MaxDiscount
		}
	case discountTypeFixed:
		d = v.DiscountValue
	default:
		d = 0
	}
	if d > subtotal {
		d = subtotal
	}
	if d < 0 {
		d = 0
	}
	return d
}

// CommitReservation finalizes a hold: the redemption is counted (voucher.used++)
// exactly once. Idempotent — a second commit of an already-committed hold is a
// no-op that still reports committed=true. Returns false if the hold was released
// or never existed.
func (s *VoucherService) CommitReservation(ctx context.Context, reservationID string) (bool, error) {
	res, err := s.reservations.GetByReservationID(ctx, reservationID)
	if err != nil {
		if errors.Is(err, repository.ErrReservationNotFound) {
			return false, nil
		}
		return false, err
	}
	switch res.Status {
	case repository.ReservationCommitted:
		return true, nil // idempotent: already finalized
	case repository.ReservationReleased:
		return false, nil // cannot commit a freed hold
	}
	if err := s.reservations.UpdateStatus(ctx, reservationID, repository.ReservationCommitted); err != nil {
		return false, err
	}
	if err := s.vouchers.IncrementUsed(ctx, res.VoucherID, 1); err != nil {
		return false, err
	}
	return true, nil
}

// ReleaseReservation frees a hold. Idempotent — releasing an already-released hold
// still reports released=true. A committed hold cannot be released (returns false).
func (s *VoucherService) ReleaseReservation(ctx context.Context, reservationID string) (bool, error) {
	res, err := s.reservations.GetByReservationID(ctx, reservationID)
	if err != nil {
		if errors.Is(err, repository.ErrReservationNotFound) {
			return false, nil
		}
		return false, err
	}
	switch res.Status {
	case repository.ReservationReleased:
		return true, nil // idempotent
	case repository.ReservationCommitted:
		return false, nil // finalized holds are never released
	}
	if err := s.reservations.UpdateStatus(ctx, reservationID, repository.ReservationReleased); err != nil {
		return false, err
	}
	return true, nil
}
