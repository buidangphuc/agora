package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
)

const (
	StatusAttributed = "ATTRIBUTED"
	StatusQualified  = "QUALIFIED"
	StatusRewarded   = "REWARDED"
	StatusCapped     = "CAPPED"

	RewardPending  = "PENDING"
	RewardReleased = "RELEASED"
	RewardVoid     = "VOID"

	ReasonFirstOrder = "invitee_first_order"

	OutboxTable = "referral_outbox_events"
)

var (
	ErrCodeNotFound     = errors.New("referral code not found")
	ErrAlreadyRedeemed  = errors.New("user has already redeemed a referral code")
	ErrReferralNotFound = errors.New("referral not found")
	ErrRewardNotFound   = errors.New("referral reward not found")
	ErrNotRegistered    = errors.New("no registration stored for user")
	ErrSelfReferral     = errors.New("cannot redeem your own referral code")
	ErrNotEligible      = errors.New("account is not eligible to redeem a referral code")
	ErrReferrerCapped   = errors.New("referral code owner reached the lifetime cap")
)

type Referral struct {
	ReferrerID   string
	RefereeID    string
	Code         string
	Status       string
	AttributedAt time.Time
}

type Reward struct {
	ID         string
	ReferrerID string
	RefereeID  string
	OrderID    string
	Reason     string
	Status     string
	AmountXU   int64
	CreatedAt  time.Time
	ReleaseAt  time.Time
	ReleasedAt *time.Time
}

type RewardTotals struct {
	PendingXU  int64
	ReleasedXU int64
}

type UserRegisteredInput struct {
	EventID      string
	UserID       string
	ReferralCode string
	RegisteredAt time.Time
	At           time.Time
	LifetimeCap  int
}

type OrderCompletedInput struct {
	EventID     string
	OrderID     string
	BuyerID     string
	CompletedAt time.Time
	ReleaseAt   time.Time
	At          time.Time
	RewardXU    int64
	LifetimeCap int
}

type ReturnRequestedInput struct {
	EventID string
	OrderID string
	At      time.Time
}

type ReleaseInput struct {
	At          time.Time
	DailyCap    int
	LifetimeCap int
	Location    *time.Location
}

type RedeemInput struct {
	RefereeID     string
	Code          string
	At            time.Time
	MaxAccountAge time.Duration
	LifetimeCap   int
}

type ReferralRepository interface {
	GetCodeByUser(ctx context.Context, userID string) (string, error)
	CreateCode(ctx context.Context, userID, code string) (string, error)
	OwnerOfCode(ctx context.Context, code string) (string, error)
	GetReferral(ctx context.Context, refereeID string) (Referral, error)
	RegisteredAt(ctx context.Context, userID string) (time.Time, error)
	HasCompletedOrder(ctx context.Context, buyerID string) (bool, error)
	CountReferralsByReferrer(ctx context.Context, referrerID string) (int64, error)
	RewardTotals(ctx context.Context, userID string) (RewardTotals, error)
	ListRewards(ctx context.Context, userID string, limit, offset int) ([]*referralv1.ReferralReward, error)
	GetRewardByOrder(ctx context.Context, orderID string) (Reward, error)
	ApplyUserRegistered(ctx context.Context, in UserRegisteredInput) error
	ApplyOrderCompleted(ctx context.Context, in OrderCompletedInput) error
	ApplyReturnRequested(ctx context.Context, in ReturnRequestedInput) error
	Redeem(ctx context.Context, in RedeemInput) error
	ReleaseDue(ctx context.Context, in ReleaseInput) (int, error)
	PurgeProcessedEvents(ctx context.Context, before time.Time) (int64, error)
}

func NewCode() string {
	return "REF" + uuid.NewString()[:8]
}

func NewRewardID() string { return "rwd_" + uuid.NewString() }

func RewardStatusProto(status string) referralv1.ReferralRewardStatus {
	switch status {
	case RewardPending:
		return referralv1.ReferralRewardStatus_REFERRAL_REWARD_STATUS_PENDING
	case RewardReleased:
		return referralv1.ReferralRewardStatus_REFERRAL_REWARD_STATUS_RELEASED
	case RewardVoid:
		return referralv1.ReferralRewardStatus_REFERRAL_REWARD_STATUS_VOID
	}
	return referralv1.ReferralRewardStatus_REFERRAL_REWARD_STATUS_UNSPECIFIED
}

func RejectReason(err error) (referralv1.ReferralRejectReason, bool) {
	switch {
	case errors.Is(err, ErrCodeNotFound):
		return referralv1.ReferralRejectReason_REFERRAL_REJECT_REASON_UNKNOWN_CODE, true
	case errors.Is(err, ErrSelfReferral):
		return referralv1.ReferralRejectReason_REFERRAL_REJECT_REASON_SELF_REFERRAL, true
	case errors.Is(err, ErrReferrerCapped):
		return referralv1.ReferralRejectReason_REFERRAL_REJECT_REASON_REFERRER_CAPPED, true
	}
	return referralv1.ReferralRejectReason_REFERRAL_REJECT_REASON_UNSPECIFIED, false
}

func DayBounds(at time.Time, loc *time.Location) (time.Time, time.Time) {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := at.In(loc).Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 0, 1)
}
