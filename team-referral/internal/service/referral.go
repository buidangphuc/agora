// Package service holds team-referral's application logic. ReferralService owns
// the referral program: minting per-user codes, redeeming someone else's code
// (attributing the referee to the referrer and emitting a mock coin-credit
// reward intent), and reading a user's reward ledger. It validates input and
// delegates persistence to the repository; identity always comes from the
// handler (resolved from the principal), never guessed here.
package service

import (
	"context"
	"errors"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/repository"
)

// RewardOnReferral is the mock coin credit (minor units) granted to a referrer
// each time one of their codes is redeemed. Rewards here are pure accounting
// intent — no real money moves (see 00-spec / AGENTS.md §7).
const RewardOnReferral int64 = 10000

// ReasonInviteeJoined is the ledger reason recorded when a referral is redeemed.
const ReasonInviteeJoined = "invitee_joined"

// Validation / domain errors surfaced to the handler, which maps them to gRPC
// status codes.
var (
	ErrEmptyUser       = errors.New("user id is required")
	ErrEmptyCode       = errors.New("code is required")
	ErrCodeNotFound    = errors.New("referral code not found")
	ErrSelfReferral    = errors.New("cannot redeem your own referral code")
	ErrAlreadyRedeemed = errors.New("user has already redeemed a referral code")
)

// ReferralService is the referral-program use-case boundary.
type ReferralService struct {
	repo repository.ReferralRepository
}

func NewReferralService(repo repository.ReferralRepository) *ReferralService {
	return &ReferralService{repo: repo}
}

// CreateCode mints (or returns the existing) referral code for the caller.
// Idempotent: a user always owns exactly one code.
func (s *ReferralService) CreateCode(ctx context.Context, userID string) (string, error) {
	if userID == "" {
		return "", ErrEmptyUser
	}
	if existing, err := s.repo.GetCodeByUser(ctx, userID); err == nil {
		return existing, nil
	} else if !errors.Is(err, repository.ErrCodeNotFound) {
		return "", err
	}
	return s.repo.CreateCode(ctx, userID, repository.NewCode())
}

// MyReferral is the caller's referral summary: their code plus lifetime stats.
type MyReferral struct {
	Code         string
	InvitedCount int64
	RewardsTotal int64 // minor units
}

// GetMyReferral returns the caller's code (minting one on first read so the
// summary is always populated) with their invited count and total rewards.
func (s *ReferralService) GetMyReferral(ctx context.Context, userID string) (*MyReferral, error) {
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
	total, err := s.repo.SumRewards(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &MyReferral{Code: code, InvitedCount: invited, RewardsTotal: total}, nil
}

// Redeem attributes refereeID to the owner of code and emits a mock reward
// intent to that referrer. It rejects unknown codes, self-referral, and a
// referee who has already redeemed any code (a user is referred at most once).
func (s *ReferralService) Redeem(ctx context.Context, refereeID, code string) error {
	if refereeID == "" {
		return ErrEmptyUser
	}
	if code == "" {
		return ErrEmptyCode
	}
	referrerID, err := s.repo.OwnerOfCode(ctx, code)
	if errors.Is(err, repository.ErrCodeNotFound) {
		return ErrCodeNotFound
	}
	if err != nil {
		return err
	}
	if referrerID == refereeID {
		return ErrSelfReferral
	}
	if err := s.repo.CreateReferral(ctx, referrerID, refereeID, code); err != nil {
		if errors.Is(err, repository.ErrAlreadyRedeemed) {
			return ErrAlreadyRedeemed
		}
		return err
	}
	// Mock coin credit: record the reward intent on the referrer's ledger. The
	// actual crediting (wallet movement) is out of scope for this service.
	if _, err := s.repo.AddReward(ctx, referrerID, RewardOnReferral, ReasonInviteeJoined); err != nil {
		return err
	}
	return nil
}

// ListRewards returns a page of the caller's reward ledger, newest-first, plus
// the next-page cursor (empty when the page is the last). `page` may be nil.
func (s *ReferralService) ListRewards(ctx context.Context, userID string, page *referralv1.ListReferralRewardsRequest) ([]*referralv1.ReferralReward, string, error) {
	if userID == "" {
		return nil, "", ErrEmptyUser
	}
	limit, offset := decodePage(page)
	// Fetch one extra row to detect whether a further page exists.
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
