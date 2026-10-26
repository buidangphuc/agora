// Package fake holds the in-memory ReferralRepository used by tests only.
package fake

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/store"
)

type referral struct {
	referrerID string
	refereeID  string
	code       string
	createdAt  time.Time
}

type InMemoryReferralRepo struct {
	mu         sync.Mutex
	codeByUser map[string]string                       // user_id -> code
	userByCode map[string]string                       // code -> user_id
	referrals  []referral                              // ordered by creation
	rewards    map[string][]*referralv1.ReferralReward // user_id -> ledger (append order)
}

func NewInMemoryReferralRepo() *InMemoryReferralRepo {
	return &InMemoryReferralRepo{
		codeByUser: make(map[string]string),
		userByCode: make(map[string]string),
		rewards:    make(map[string][]*referralv1.ReferralReward),
	}
}

func (r *InMemoryReferralRepo) GetCodeByUser(_ context.Context, userID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	code, ok := r.codeByUser[userID]
	if !ok {
		return "", store.ErrCodeNotFound
	}
	return code, nil
}

func (r *InMemoryReferralRepo) CreateCode(_ context.Context, userID, code string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.codeByUser[userID]; ok {
		return existing, nil // idempotent
	}
	r.codeByUser[userID] = code
	r.userByCode[code] = userID
	return code, nil
}

func (r *InMemoryReferralRepo) OwnerOfCode(_ context.Context, code string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	userID, ok := r.userByCode[code]
	if !ok {
		return "", store.ErrCodeNotFound
	}
	return userID, nil
}

func (r *InMemoryReferralRepo) HasRedeemed(_ context.Context, refereeID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ref := range r.referrals {
		if ref.refereeID == refereeID {
			return true, nil
		}
	}
	return false, nil
}

func (r *InMemoryReferralRepo) CreateReferral(_ context.Context, referrerID, refereeID, code string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ref := range r.referrals {
		if ref.refereeID == refereeID {
			return store.ErrAlreadyRedeemed
		}
	}
	r.referrals = append(r.referrals, referral{
		referrerID: referrerID,
		refereeID:  refereeID,
		code:       code,
		createdAt:  time.Now(),
	})
	return nil
}

func (r *InMemoryReferralRepo) CountReferralsByReferrer(_ context.Context, referrerID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, ref := range r.referrals {
		if ref.referrerID == referrerID {
			n++
		}
	}
	return n, nil
}

func (r *InMemoryReferralRepo) AddReward(_ context.Context, userID string, amount int64, reason string) (*referralv1.ReferralReward, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rw := &referralv1.ReferralReward{
		Id:        "rwd_" + uuid.NewString()[:8],
		Amount:    amount,
		Reason:    reason,
		CreatedAt: timestamppb.New(time.Now()),
	}
	r.rewards[userID] = append(r.rewards[userID], rw)
	return cloneReward(rw), nil
}

func (r *InMemoryReferralRepo) ListRewards(_ context.Context, userID string, limit, offset int) ([]*referralv1.ReferralReward, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ledger := r.rewards[userID]
	// Newest-first, matching the Postgres ORDER BY created_at DESC.
	sorted := make([]*referralv1.ReferralReward, len(ledger))
	copy(sorted, ledger)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].GetCreatedAt().AsTime().After(sorted[j].GetCreatedAt().AsTime())
	})
	if offset >= len(sorted) {
		return nil, nil
	}
	end := offset + limit
	if end > len(sorted) {
		end = len(sorted)
	}
	page := sorted[offset:end]
	out := make([]*referralv1.ReferralReward, 0, len(page))
	for _, rw := range page {
		out = append(out, cloneReward(rw))
	}
	return out, nil
}

func (r *InMemoryReferralRepo) SumRewards(_ context.Context, userID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var total int64
	for _, rw := range r.rewards[userID] {
		total += rw.GetAmount()
	}
	return total, nil
}

func cloneReward(rw *referralv1.ReferralReward) *referralv1.ReferralReward {
	return &referralv1.ReferralReward{
		Id:        rw.GetId(),
		Amount:    rw.GetAmount(),
		Reason:    rw.GetReason(),
		CreatedAt: rw.GetCreatedAt(),
	}
}
