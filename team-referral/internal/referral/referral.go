package referral

import (
	"context"
	"errors"
	"time"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/store"
)

var (
	ErrEmptyUser       = errors.New("user id is required")
	ErrEmptyCode       = errors.New("code is required")
	ErrCodeNotFound    = errors.New("referral code not found")
	ErrSelfReferral    = errors.New("cannot redeem your own referral code")
	ErrAlreadyRedeemed = errors.New("user has already redeemed a referral code")
	ErrNotEligible     = errors.New("account is not eligible to redeem a referral code")
	ErrReferrerCapped  = errors.New("referral code owner reached the lifetime cap")
)

type Service struct {
	repo store.ReferralRepository
	cfg  Config
	now  func() time.Time
}

func NewService(repo store.ReferralRepository, opts ...Option) *Service {
	s := newSettings(opts)
	return &Service{repo: repo, cfg: s.cfg, now: s.now}
}

func (s *Service) CreateCode(ctx context.Context, userID string) (string, error) {
	if userID == "" {
		return "", ErrEmptyUser
	}
	if existing, err := s.repo.GetCodeByUser(ctx, userID); err == nil {
		return existing, nil
	} else if !errors.Is(err, store.ErrCodeNotFound) {
		return "", err
	}
	return s.repo.CreateCode(ctx, userID, store.NewCode())
}

type MyReferral struct {
	Code         string
	InvitedCount int64
	RewardsTotal int64
	PendingXU    int64
	ReleasedXU   int64
}

func (s *Service) GetMyReferral(ctx context.Context, userID string) (*MyReferral, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	code, err := s.CreateCode(ctx, userID)
	if err != nil {
		return nil, err
	}
	invited, err := s.repo.CountReferralsByReferrer(ctx, userID)
	if err != nil {
		return nil, err
	}
	totals, err := s.repo.RewardTotals(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &MyReferral{
		Code:         code,
		InvitedCount: invited,
		RewardsTotal: totals.ReleasedXU,
		PendingXU:    totals.PendingXU,
		ReleasedXU:   totals.ReleasedXU,
	}, nil
}

func (s *Service) Redeem(ctx context.Context, refereeID, code string) error {
	if refereeID == "" {
		return ErrEmptyUser
	}
	if code == "" {
		return ErrEmptyCode
	}
	err := s.repo.Redeem(ctx, store.RedeemInput{
		RefereeID:     refereeID,
		Code:          code,
		At:            s.now(),
		MaxAccountAge: s.cfg.RedeemMaxAccountAge,
		LifetimeCap:   s.cfg.LifetimeCap,
	})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrCodeNotFound):
		return ErrCodeNotFound
	case errors.Is(err, store.ErrSelfReferral):
		return ErrSelfReferral
	case errors.Is(err, store.ErrAlreadyRedeemed):
		return ErrAlreadyRedeemed
	case errors.Is(err, store.ErrNotEligible), errors.Is(err, store.ErrNotRegistered):
		return ErrNotEligible
	case errors.Is(err, store.ErrReferrerCapped):
		return ErrReferrerCapped
	}
	return err
}

func (s *Service) ListRewards(ctx context.Context, userID string, page *referralv1.ListReferralRewardsRequest) ([]*referralv1.ReferralReward, string, error) {
	if userID == "" {
		return nil, "", ErrEmptyUser
	}
	limit, offset := decodePage(page)
	rewards, err := s.repo.ListRewards(ctx, userID, limit+1, offset)
	if err != nil {
		return nil, "", err
	}
	nextCursor := ""
	if len(rewards) > limit {
		rewards = rewards[:limit]
		nextCursor = encodeCursor(offset + limit)
	}
	return rewards, nextCursor, nil
}
