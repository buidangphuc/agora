package fake

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/events"
	"github.com/buidangphuc/team-referral/internal/store"
)

type InMemoryReferralRepo struct {
	mu         sync.Mutex
	codeByUser map[string]string
	userByCode map[string]string
	accounts   map[string]time.Time
	buyers     map[string]time.Time
	processed  map[string]time.Time
	referrals  map[string]*store.Referral
	rewards    []*store.Reward
	outbox     []outbox.Row
}

func NewInMemoryReferralRepo() *InMemoryReferralRepo {
	return &InMemoryReferralRepo{
		codeByUser: make(map[string]string),
		userByCode: make(map[string]string),
		accounts:   make(map[string]time.Time),
		buyers:     make(map[string]time.Time),
		processed:  make(map[string]time.Time),
		referrals:  make(map[string]*store.Referral),
	}
}

func (r *InMemoryReferralRepo) OutboxRows() []outbox.Row {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]outbox.Row(nil), r.outbox...)
}

func (r *InMemoryReferralRepo) enqueue(row outbox.Row, err error) error {
	if err != nil {
		return err
	}
	for _, existing := range r.outbox {
		if existing.EventID == row.EventID {
			return nil
		}
	}
	r.outbox = append(r.outbox, row)
	return nil
}

func (r *InMemoryReferralRepo) markProcessed(eventID string, at time.Time) bool {
	if _, seen := r.processed[eventID]; seen {
		return false
	}
	r.processed[eventID] = at
	return true
}

func (r *InMemoryReferralRepo) live(userID string) int {
	n := 0
	for _, rw := range r.rewards {
		if rw.ReferrerID == userID && rw.Status != store.RewardVoid {
			n++
		}
	}
	return n
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
		return existing, nil
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

func (r *InMemoryReferralRepo) GetReferral(_ context.Context, refereeID string) (store.Referral, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ref, ok := r.referrals[refereeID]
	if !ok {
		return store.Referral{}, store.ErrReferralNotFound
	}
	return *ref, nil
}

func (r *InMemoryReferralRepo) RegisteredAt(_ context.Context, userID string) (time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	at, ok := r.accounts[userID]
	if !ok {
		return time.Time{}, store.ErrNotRegistered
	}
	return at, nil
}

func (r *InMemoryReferralRepo) HasCompletedOrder(_ context.Context, buyerID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.buyers[buyerID]
	return ok, nil
}

func (r *InMemoryReferralRepo) CountReferralsByReferrer(_ context.Context, referrerID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, ref := range r.referrals {
		if ref.ReferrerID == referrerID {
			n++
		}
	}
	return n, nil
}

func (r *InMemoryReferralRepo) RewardTotals(_ context.Context, userID string) (store.RewardTotals, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var t store.RewardTotals
	for _, rw := range r.rewards {
		if rw.ReferrerID != userID {
			continue
		}
		switch rw.Status {
		case store.RewardPending:
			t.PendingXU += rw.AmountXU
		case store.RewardReleased:
			t.ReleasedXU += rw.AmountXU
		}
	}
	return t, nil
}

func (r *InMemoryReferralRepo) ListRewards(_ context.Context, userID string, limit, offset int) ([]*referralv1.ReferralReward, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var mine []*store.Reward
	for _, rw := range r.rewards {
		if rw.ReferrerID == userID && rw.Status != store.RewardVoid {
			mine = append(mine, rw)
		}
	}
	for i, j := 0, len(mine)-1; i < j; i, j = i+1, j-1 {
		mine[i], mine[j] = mine[j], mine[i]
	}
	sort.SliceStable(mine, func(i, j int) bool { return mine[i].CreatedAt.After(mine[j].CreatedAt) })
	if offset >= len(mine) {
		return nil, nil
	}
	end := min(offset+limit, len(mine))
	out := make([]*referralv1.ReferralReward, 0, end-offset)
	for _, rw := range mine[offset:end] {
		out = append(out, &referralv1.ReferralReward{
			Id:        rw.ID,
			Amount:    rw.AmountXU,
			Reason:    rw.Reason,
			CreatedAt: timestamppb.New(rw.CreatedAt),
			Status:    store.RewardStatusProto(rw.Status),
			ReleaseAt: timestamppb.New(rw.ReleaseAt),
		})
	}
	return out, nil
}

func (r *InMemoryReferralRepo) GetRewardByOrder(_ context.Context, orderID string) (store.Reward, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rw := range r.rewards {
		if rw.OrderID == orderID {
			return cloneReward(rw), nil
		}
	}
	return store.Reward{}, store.ErrRewardNotFound
}

func (r *InMemoryReferralRepo) ApplyUserRegistered(_ context.Context, in store.UserRegisteredInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.markProcessed(in.EventID, in.At) {
		return nil
	}
	r.accounts[in.UserID] = in.RegisteredAt
	if in.ReferralCode == "" {
		return nil
	}
	if _, ok := r.referrals[in.UserID]; ok {
		return nil
	}
	owner, ok := r.userByCode[in.ReferralCode]
	var verdict error
	switch {
	case !ok:
		verdict = store.ErrCodeNotFound
	case owner == in.UserID:
		verdict = store.ErrSelfReferral
	case r.live(owner) >= in.LifetimeCap:
		verdict = store.ErrReferrerCapped
	}
	if verdict != nil {
		reason, _ := store.RejectReason(verdict)
		return r.enqueue(events.Rejected(in.UserID, reason, in.At))
	}
	r.addReferral(owner, in.UserID, in.ReferralCode, in.At)
	return r.enqueue(events.Attributed(in.UserID, owner, in.At))
}

func (r *InMemoryReferralRepo) addReferral(referrer, referee, code string, at time.Time) {
	r.referrals[referee] = &store.Referral{
		ReferrerID: referrer, RefereeID: referee, Code: code, Status: store.StatusAttributed, AttributedAt: at,
	}
}

func (r *InMemoryReferralRepo) Redeem(_ context.Context, in store.RedeemInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	owner, ok := r.userByCode[in.Code]
	if !ok {
		return store.ErrCodeNotFound
	}
	if owner == in.RefereeID {
		return store.ErrSelfReferral
	}
	if _, ok := r.referrals[in.RefereeID]; ok {
		return store.ErrAlreadyRedeemed
	}
	registeredAt, ok := r.accounts[in.RefereeID]
	if !ok || in.At.Sub(registeredAt) >= in.MaxAccountAge {
		return store.ErrNotEligible
	}
	if _, ordered := r.buyers[in.RefereeID]; ordered {
		return store.ErrNotEligible
	}
	if r.live(owner) >= in.LifetimeCap {
		return store.ErrReferrerCapped
	}
	r.addReferral(owner, in.RefereeID, in.Code, in.At)
	return r.enqueue(events.Attributed(in.RefereeID, owner, in.At))
}

func (r *InMemoryReferralRepo) ApplyOrderCompleted(_ context.Context, in store.OrderCompletedInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.markProcessed(in.EventID, in.At) {
		return nil
	}
	if first, ok := r.buyers[in.BuyerID]; !ok || in.CompletedAt.Before(first) {
		r.buyers[in.BuyerID] = in.CompletedAt
	}
	ref, ok := r.referrals[in.BuyerID]
	if !ok || ref.Status != store.StatusAttributed {
		return nil
	}
	if r.live(ref.ReferrerID) >= in.LifetimeCap {
		ref.Status = store.StatusCapped
		return nil
	}
	for _, rw := range r.rewards {
		if rw.OrderID == in.OrderID {
			return nil
		}
	}
	r.rewards = append(r.rewards, &store.Reward{
		ID:         store.NewRewardID(),
		ReferrerID: ref.ReferrerID,
		RefereeID:  in.BuyerID,
		OrderID:    in.OrderID,
		Reason:     store.ReasonFirstOrder,
		Status:     store.RewardPending,
		AmountXU:   in.RewardXU,
		CreatedAt:  in.At,
		ReleaseAt:  in.ReleaseAt,
	})
	ref.Status = store.StatusQualified
	return nil
}

func (r *InMemoryReferralRepo) ApplyReturnRequested(_ context.Context, in store.ReturnRequestedInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.markProcessed(in.EventID, in.At) {
		return nil
	}
	for _, rw := range r.rewards {
		if rw.OrderID != in.OrderID || rw.Status != store.RewardPending {
			continue
		}
		rw.Status = store.RewardVoid
		if ref, ok := r.referrals[rw.RefereeID]; ok && ref.Status == store.StatusQualified {
			ref.Status = store.StatusAttributed
		}
	}
	return nil
}

func (r *InMemoryReferralRepo) ReleaseDue(_ context.Context, in store.ReleaseInput) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var due []*store.Reward
	for _, rw := range r.rewards {
		if rw.Status == store.RewardPending && !rw.ReleaseAt.After(in.At) {
			due = append(due, rw)
		}
	}
	sort.SliceStable(due, func(i, j int) bool { return due[i].ReleaseAt.Before(due[j].ReleaseAt) })
	start, end := store.DayBounds(in.At, in.Location)
	released := 0
	var errs []error
	for _, rw := range due {
		today, total := 0, 0
		for _, other := range r.rewards {
			if other.ReferrerID != rw.ReferrerID || other.Status != store.RewardReleased || other.ReleasedAt == nil {
				continue
			}
			total++
			if !other.ReleasedAt.Before(start) && other.ReleasedAt.Before(end) {
				today++
			}
		}
		if today >= in.DailyCap || total >= in.LifetimeCap {
			continue
		}
		if err := r.enqueue(events.Released(rw.ID, rw.ReferrerID, rw.RefereeID, rw.AmountXU, in.At)); err != nil {
			errs = append(errs, err)
			continue
		}
		at := in.At
		rw.Status = store.RewardReleased
		rw.ReleasedAt = &at
		if ref, ok := r.referrals[rw.RefereeID]; ok {
			ref.Status = store.StatusRewarded
		}
		released++
	}
	return released, errors.Join(errs...)
}

func (r *InMemoryReferralRepo) PurgeProcessedEvents(_ context.Context, before time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for id, at := range r.processed {
		if at.Before(before) {
			delete(r.processed, id)
			n++
		}
	}
	return n, nil
}

func cloneReward(rw *store.Reward) store.Reward {
	out := *rw
	if rw.ReleasedAt != nil {
		at := *rw.ReleasedAt
		out.ReleasedAt = &at
	}
	return out
}
