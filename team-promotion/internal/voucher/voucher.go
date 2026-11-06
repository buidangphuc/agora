package voucher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
)

// Discount type / scope enum values (mirror platform.promotion.v1 without importing
// the wire enum into the domain logic).
const (
	discountTypePercent int32 = 1
	discountTypeFixed   int32 = 2

	voucherScopeShop int32 = 1
)

const (
	defaultPageSize = 50

	maxPerUserLimit   = 1000
	myVouchersLimit   = 50
	defaultHoldTTL    = 15 * time.Minute
	ReasonNotEligible = "VOUCHER_NOT_ELIGIBLE"
	ReasonPerUser     = "VOUCHER_PER_USER_LIMIT"
)

const grpcCodeUnavailable int32 = 14

type orderUnavailable struct{}

func (orderUnavailable) Error() string { return "order service unavailable" }

func (orderUnavailable) GRPCStatus() *status.Status {
	return status.FromProto(&spb.Status{Code: grpcCodeUnavailable, Message: "order service unavailable"})
}

var ErrOrderUnavailable error = orderUnavailable{}

type OrderClient interface {
	HasCompletedOrder(ctx context.Context, buyerID string) (bool, error)
}

type Service struct {
	vouchers     store.VoucherRepository
	reservations store.ReservationRepository
	audit        *auditx.Gate
	event        func(context.Context, store.Voucher) (outbox.Row, error)
	logger       *slog.Logger
	nowFn        func() time.Time
	orders       OrderClient
	holdTTL      time.Duration
}

func New(
	vouchers store.VoucherRepository,
	reservations store.ReservationRepository,
	audit *auditx.Gate,
	event func(context.Context, store.Voucher) (outbox.Row, error),
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		vouchers:     vouchers,
		reservations: reservations,
		audit:        audit,
		event:        event,
		logger:       logger,
		nowFn:        time.Now,
		holdTTL:      defaultHoldTTL,
	}
}

func (s *Service) WithOrderClient(c OrderClient) *Service {
	s.orders = c
	return s
}

func (s *Service) WithReservationTTL(d time.Duration) *Service {
	if d > 0 {
		s.holdTTL = d
	}
	return s
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
	PerUserLimit  int32
	NewUserOnly   bool

	// AuditActorID is the verified admin id written as the audit actor_id.
	AuditActorID string

	// AuditAdmin marks a shop-scoped voucher created by an admin; it must be audited.
	AuditAdmin bool
}

// ErrInvalidVoucher is returned for malformed CreateVoucher input.
var ErrInvalidVoucher = errors.New("invalid voucher")

func checkPerUserLimit(n int32) error {
	if n < 0 || n > maxPerUserLimit {
		return fmt.Errorf("%w: per_user_limit must be between 0 and %d", ErrInvalidVoucher, maxPerUserLimit)
	}
	return nil
}

func addPerUserMeta(meta map[string]string, hasLimit bool, limit int32, newUserOnly bool) {
	if hasLimit {
		meta["per_user_limit"] = strconv.FormatInt(int64(limit), 10)
	}
	if newUserOnly {
		meta["new_user_only"] = "true"
	}
}

func (s *Service) CreateVoucher(ctx context.Context, p CreateVoucherParams) (store.Voucher, error) {
	if strings.TrimSpace(p.Code) == "" {
		return store.Voucher{}, ErrInvalidVoucher
	}
	if err := checkPerUserLimit(p.PerUserLimit); err != nil {
		return store.Voucher{}, err
	}
	if _, err := s.vouchers.GetByCode(ctx, strings.TrimSpace(p.Code)); err == nil {
		return store.Voucher{}, store.ErrDuplicateCode
	} else if !errors.Is(err, store.ErrVoucherNotFound) {
		return store.Voucher{}, err
	}
	v := store.Voucher{
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
		PerUserLimit:  p.PerUserLimit,
		NewUserOnly:   p.NewUserOnly,
		Kind:          store.VoucherKindStandard,
	}
	var created store.Voucher
	insert := func(ctx context.Context) error {
		var err error
		created, err = s.vouchers.Create(ctx, v, func(c store.Voucher) (outbox.Row, error) {
			return s.event(ctx, c)
		})
		return err
	}
	if p.Scope == voucherScopeShop && !p.AuditAdmin {
		if err := insert(ctx); err != nil {
			return store.Voucher{}, err
		}
		return created, nil
	}
	ev := auditx.Event{
		ActorID:    p.AuditActorID,
		Action:     "voucher.platform_create",
		TargetType: "voucher",
		TargetID:   v.Code,
		Metadata: map[string]string{
			"discount_type": promotionv1.DiscountType(p.DiscountType).String(),
			"value":         strconv.FormatInt(p.DiscountValue, 10),
		},
	}
	addPerUserMeta(ev.Metadata, p.PerUserLimit > 0, p.PerUserLimit, p.NewUserOnly)
	if p.Scope == voucherScopeShop {
		ev.Action = "voucher.admin_create"
		ev.Metadata["seller_id"] = p.SellerID
	}
	if err := s.audit.Do(ctx, ev, insert); err != nil {
		return store.Voucher{}, err
	}
	return created, nil
}

type DeactivateVoucherParams struct {
	VoucherID    string
	AuditActorID string
}

type UpdateVoucherParams struct {
	VoucherID    string
	EndsAt       *time.Time
	Quota        *int64
	PerUserLimit *int32
	NewUserOnly  *bool
	AuditActorID string
}

func (s *Service) DeactivateVoucher(ctx context.Context, p DeactivateVoucherParams) (store.Voucher, error) {
	cur, err := s.vouchers.GetByID(ctx, p.VoucherID)
	if err != nil {
		return store.Voucher{}, err
	}
	var out store.Voucher
	ev := auditx.Event{
		ActorID:    p.AuditActorID,
		Action:     "voucher.deactivate",
		TargetType: "voucher",
		TargetID:   cur.Code,
	}
	err = s.audit.Do(ctx, ev, func(ctx context.Context) error {
		var derr error
		out, derr = s.vouchers.Deactivate(ctx, cur.ID, s.nowFn())
		return derr
	})
	if err != nil {
		return store.Voucher{}, err
	}
	return out, nil
}

func (s *Service) UpdateVoucher(ctx context.Context, p UpdateVoucherParams) (store.Voucher, error) {
	cur, err := s.vouchers.GetByID(ctx, p.VoucherID)
	if err != nil {
		return store.Voucher{}, err
	}
	if !cur.Active {
		return store.Voucher{}, store.ErrVoucherInactive
	}
	var endsAt time.Time
	var quota int64
	changed := false
	if p.PerUserLimit != nil || p.NewUserOnly != nil {
		if cur.Kind != store.VoucherKindStandard {
			return store.Voucher{}, fmt.Errorf("%w: a new-user voucher cannot change its per-user rules", ErrInvalidVoucher)
		}
		if p.PerUserLimit != nil {
			if err := checkPerUserLimit(*p.PerUserLimit); err != nil {
				return store.Voucher{}, err
			}
		}
		changed = true
	}
	if p.EndsAt != nil {
		endsAt = *p.EndsAt
		if !endsAt.After(s.nowFn()) {
			return store.Voucher{}, fmt.Errorf("%w: ends_at must be in the future", ErrInvalidVoucher)
		}
		if !cur.StartsAt.IsZero() && !endsAt.After(cur.StartsAt) {
			return store.Voucher{}, fmt.Errorf("%w: ends_at must be after starts_at", ErrInvalidVoucher)
		}
		changed = true
	}
	if p.Quota != nil {
		q := *p.Quota
		if q <= 0 {
			return store.Voucher{}, fmt.Errorf("%w: quota must be positive", ErrInvalidVoucher)
		}
		if cur.Quota <= 0 {
			return store.Voucher{}, fmt.Errorf("%w: an unlimited voucher cannot get a quota", ErrInvalidVoucher)
		}
		if q < cur.Quota || q < cur.Used {
			return store.Voucher{}, fmt.Errorf("%w: quota cannot be lowered", ErrInvalidVoucher)
		}
		if q > cur.Quota {
			quota = q
			changed = true
		}
	}
	if !changed {
		return store.Voucher{}, fmt.Errorf("%w: nothing to change", ErrInvalidVoucher)
	}
	meta := map[string]string{}
	if p.EndsAt != nil {
		meta["ends_at"] = endsAt.UTC().Format(time.RFC3339)
	}
	if quota > 0 {
		meta["quota"] = strconv.FormatInt(quota, 10)
	}
	if p.PerUserLimit != nil {
		meta["per_user_limit"] = strconv.FormatInt(int64(*p.PerUserLimit), 10)
	}
	if p.NewUserOnly != nil {
		meta["new_user_only"] = strconv.FormatBool(*p.NewUserOnly)
	}
	var out store.Voucher
	ev := auditx.Event{
		ActorID:    p.AuditActorID,
		Action:     "voucher.update",
		TargetType: "voucher",
		TargetID:   cur.Code,
		Metadata:   meta,
	}
	err = s.audit.Do(ctx, ev, func(ctx context.Context) error {
		out = cur
		if p.EndsAt != nil || quota > 0 {
			var uerr error
			if out, uerr = s.vouchers.UpdateEndsAtQuota(ctx, cur.ID, endsAt, quota); uerr != nil {
				return uerr
			}
		}
		if p.PerUserLimit != nil || p.NewUserOnly != nil {
			var uerr error
			if out, uerr = s.vouchers.UpdatePerUser(ctx, cur.ID, p.PerUserLimit, p.NewUserOnly); uerr != nil {
				return uerr
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrVoucherQuota) {
			return store.Voucher{}, fmt.Errorf("%w: quota change refused", ErrInvalidVoucher)
		}
		return store.Voucher{}, err
	}
	return out, nil
}

// GetVoucher looks a voucher up by code.
func (s *Service) GetVoucher(ctx context.Context, code string) (store.Voucher, error) {
	return s.vouchers.GetByCode(ctx, code)
}

func (s *Service) ListMyVouchers(ctx context.Context, userID string) ([]store.Voucher, error) {
	return s.vouchers.ListByOwner(ctx, userID, s.nowFn(), myVouchersLimit)
}

// ListVouchers returns a page of vouchers for a seller (empty = platform-scoped).
// The cursor is a stringified offset; nextCursor is empty when the page is last.
func (s *Service) ListVouchers(ctx context.Context, sellerID, cursor string, pageSize int32) ([]store.Voucher, string, error) {
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
	Scope          int32
	SellerID       string
}

// ValidateAndReserve validates a voucher against a cart and places an idempotent
// hold keyed on reservationID. A retry with the same reservationID returns the
// original discount without decrementing quota or creating a second hold
// (ADR-0008). A rejection returns Valid=false with a human-readable reason and
// never creates a hold.
func (s *Service) ValidateAndReserve(ctx context.Context, reservationID, code, buyerID string, cartSubtotal int64, sellerID string) (ReserveResult, error) {
	if strings.TrimSpace(reservationID) == "" {
		return ReserveResult{Valid: false, Reason: "reservation_id is required"}, nil
	}

	// Idempotency: an existing hold for this reservation_id wins — return its
	// original discount regardless of the (possibly re-sent) request fields.
	existing, err := s.reservations.GetByReservationID(ctx, reservationID)
	if err == nil {
		var scope int32
		var owner string
		stored, gerr := s.vouchers.GetByID(ctx, existing.VoucherID)
		switch {
		case gerr == nil:
			scope, owner = stored.Scope, stored.SellerID
		case !errors.Is(gerr, store.ErrVoucherNotFound):
			return ReserveResult{}, gerr
		}
		if existing.Status == store.ReservationReleased {
			return ReserveResult{Valid: false, Reason: "reservation already released", Scope: scope, SellerID: owner}, nil
		}
		return ReserveResult{Valid: true, DiscountAmount: existing.DiscountAmount, VoucherID: existing.VoucherID, Scope: scope, SellerID: owner}, nil
	}
	if !errors.Is(err, store.ErrReservationNotFound) {
		return ReserveResult{}, err
	}

	if strings.TrimSpace(code) == "" {
		return ReserveResult{Valid: false, Reason: "voucher code is required"}, nil
	}

	voucher, err := s.vouchers.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, store.ErrVoucherNotFound) {
			return ReserveResult{Valid: false, Reason: "voucher code not found"}, nil
		}
		return ReserveResult{}, err
	}

	now := s.nowFn()
	refuse := func(reason string) (ReserveResult, error) {
		return ReserveResult{Valid: false, Reason: reason, Scope: voucher.Scope, SellerID: voucher.SellerID}, nil
	}
	if voucher.OwnerUserID != "" && voucher.OwnerUserID != buyerID {
		return refuse(ReasonNotEligible)
	}
	if reason, ok := validateWindow(voucher, cartSubtotal, now); !ok {
		return refuse(reason)
	}
	if voucher.PerUserLimit > 0 {
		n, err := s.reservations.CountActiveByBuyer(ctx, voucher.ID, buyerID, now)
		if err != nil {
			return ReserveResult{}, err
		}
		if n >= int(voucher.PerUserLimit) {
			return refuse(ReasonPerUser)
		}
	}
	if voucher.NewUserOnly {
		has, err := s.buyerHasCompletedOrder(ctx, buyerID)
		if err != nil {
			return ReserveResult{}, err
		}
		if has {
			return refuse(ReasonNotEligible)
		}
	}
	if reason, ok := validateRemaining(voucher, sellerID); !ok {
		return refuse(reason)
	}

	hold := store.Reservation{
		ReservationID:  reservationID,
		VoucherID:      voucher.ID,
		BuyerID:        buyerID,
		DiscountAmount: computeDiscount(voucher, cartSubtotal),
		Status:         store.ReservationReserved,
		ExpiresAt:      now.Add(s.holdTTL),
	}
	var stored store.Reservation
	if voucher.PerUserLimit > 0 {
		stored, _, err = s.reservations.CreateWithinLimit(ctx, hold, int(voucher.PerUserLimit), now)
		if errors.Is(err, store.ErrPerUserLimit) {
			return refuse(ReasonPerUser)
		}
	} else {
		stored, _, err = s.reservations.Create(ctx, hold)
	}
	if err != nil {
		return ReserveResult{}, err
	}
	return ReserveResult{Valid: true, DiscountAmount: stored.DiscountAmount, VoucherID: stored.VoucherID, Scope: voucher.Scope, SellerID: voucher.SellerID}, nil
}

func (s *Service) buyerHasCompletedOrder(ctx context.Context, buyerID string) (bool, error) {
	if s.orders == nil {
		return false, ErrOrderUnavailable
	}
	has, err := s.orders.HasCompletedOrder(ctx, buyerID)
	if err != nil {
		return false, ErrOrderUnavailable
	}
	return has, nil
}

func validateWindow(v store.Voucher, subtotal int64, now time.Time) (string, bool) {
	if !v.Active {
		return "inactive", false
	}
	if !v.StartsAt.IsZero() && now.Before(v.StartsAt) {
		return "voucher not yet active", false
	}
	if !v.EndsAt.IsZero() && !now.Before(v.EndsAt) {
		return "voucher expired", false
	}
	if subtotal < v.MinSpend {
		return "order subtotal below minimum spend", false
	}
	return "", true
}

func validateRemaining(v store.Voucher, sellerID string) (string, bool) {
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
func computeDiscount(v store.Voucher, subtotal int64) int64 {
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
func (s *Service) CommitReservation(ctx context.Context, reservationID string) (bool, error) {
	res, err := s.reservations.GetByReservationID(ctx, reservationID)
	if err != nil {
		if errors.Is(err, store.ErrReservationNotFound) {
			return false, nil
		}
		return false, err
	}
	switch res.Status {
	case store.ReservationCommitted:
		return true, nil // idempotent: already finalized
	case store.ReservationReleased:
		return false, nil // cannot commit a freed hold
	}
	if err := s.reservations.UpdateStatus(ctx, reservationID, store.ReservationCommitted); err != nil {
		return false, err
	}
	if err := s.vouchers.IncrementUsed(ctx, res.VoucherID, 1); err != nil {
		return false, err
	}
	return true, nil
}

// ReleaseReservation frees a hold. Idempotent — releasing an already-released hold
// still reports released=true. A committed hold cannot be released (returns false).
func (s *Service) ReleaseReservation(ctx context.Context, reservationID string) (bool, error) {
	res, err := s.reservations.GetByReservationID(ctx, reservationID)
	if err != nil {
		if errors.Is(err, store.ErrReservationNotFound) {
			return false, nil
		}
		return false, err
	}
	switch res.Status {
	case store.ReservationReleased:
		return true, nil // idempotent
	case store.ReservationCommitted:
		return false, nil // finalized holds are never released
	}
	if err := s.reservations.UpdateStatus(ctx, reservationID, store.ReservationReleased); err != nil {
		return false, err
	}
	return true, nil
}
