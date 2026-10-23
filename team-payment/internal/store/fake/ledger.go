package fake

import (
	"context"
	"sort"
	"sync"

	"github.com/buidangphuc/team-payment/internal/store"
)

type InMemoryLedgerRepository struct {
	mu      sync.RWMutex
	entries []store.LedgerEntry
	seq     int64 // monotonic tiebreaker so ordering is deterministic in tests
	seqByID map[string]int64
	// refunds is the settlement ledger's refund table (set by NewInMemorySettlementLedger):
	// hold-back maps a deduction to its payment through it.
	refunds *refundIndex
}

func NewInMemoryLedgerRepository() *InMemoryLedgerRepository {
	return &InMemoryLedgerRepository{seqByID: make(map[string]int64)}
}

// appendLocked appends e; callers hold r.mu for writing.
func (r *InMemoryLedgerRepository) appendLocked(e store.LedgerEntry) store.LedgerEntry {
	e = store.NormalizeEntry(e)
	r.seq++
	r.seqByID[e.ID] = r.seq
	r.entries = append(r.entries, e)
	return e
}

// byReferenceLocked finds the entry of type typ for ref; callers hold r.mu.
func (r *InMemoryLedgerRepository) byReferenceLocked(typ, ref string) (store.LedgerEntry, bool) {
	for _, x := range r.entries {
		if ref != "" && x.Type == typ && x.ReferenceID == ref {
			return x, true
		}
	}
	return store.LedgerEntry{}, false
}

// appendOnceLocked mirrors insertLedgerOnce; callers hold r.mu for writing.
func (r *InMemoryLedgerRepository) appendOnceLocked(e store.LedgerEntry) (store.LedgerEntry, bool, error) {
	if e.ReferenceID == "" {
		return store.LedgerEntry{}, false, store.ErrReferenceRequired
	}
	if existing, ok := r.byReferenceLocked(e.Type, e.ReferenceID); ok {
		return existing, false, nil
	}
	return r.appendLocked(e), true, nil
}

func (r *InMemoryLedgerRepository) AppendEntry(_ context.Context, e store.LedgerEntry) (store.LedgerEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendLocked(e), nil
}

func (r *InMemoryLedgerRepository) AppendEntryOnce(_ context.Context, e store.LedgerEntry) (store.LedgerEntry, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendOnceLocked(e)
}

// heldCreditsLocked mirrors heldCreditsSQL; callers hold r.mu.
func (r *InMemoryLedgerRepository) heldCreditsLocked(sellerID string, h store.Holdback) []store.HeldCredit {
	if !h.Enabled() {
		return nil
	}
	var out []store.HeldCredit
	for _, c := range r.entries {
		if c.SellerID != sellerID || c.Type != store.LedgerTypeOrderSettlement || c.Status != store.LedgerStatusCompleted || !c.CreatedAt.After(h.Cutoff()) {
			continue
		}
		net := c.Amount
		for _, d := range r.entries {
			if c.ReferenceID == "" || d.Type != store.LedgerTypeRefundDeduction || r.refunds == nil {
				continue
			}
			if pid, ok := r.refunds.paymentOf(d.ReferenceID); ok && pid == c.ReferenceID {
				net += d.Amount
			}
		}
		out = append(out, store.HeldCredit{CreatedAt: c.CreatedAt, Net: net})
	}
	return out
}

// AppendDebit checks the balance and the hold and appends the debit under one write lock.
func (r *InMemoryLedgerRepository) AppendDebit(_ context.Context, e store.LedgerEntry, hold store.Holdback) (store.LedgerEntry, error) {
	if e.Amount >= 0 {
		return store.LedgerEntry{}, store.ErrInvalidAmount
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var balance int64
	for _, x := range r.entries {
		if x.SellerID == e.SellerID {
			balance += x.Amount
		}
	}
	if err := store.CheckWithdrawable(balance, -e.Amount, r.heldCreditsLocked(e.SellerID, hold), hold); err != nil {
		return store.LedgerEntry{}, err
	}
	return r.appendLocked(e), nil
}

func (r *InMemoryLedgerRepository) Balance(_ context.Context, sellerID string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var balance int64
	for _, e := range r.entries {
		if e.SellerID == sellerID {
			balance += e.Amount
		}
	}
	return balance, nil
}

func (r *InMemoryLedgerRepository) ListEntries(_ context.Context, sellerID string, offset, limit int) ([]store.LedgerEntry, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var owned []store.LedgerEntry
	for _, e := range r.entries {
		if e.SellerID == sellerID {
			owned = append(owned, e)
		}
	}
	// Newest first: higher insertion sequence sorts earlier (matches created_at DESC).
	sort.SliceStable(owned, func(i, j int) bool {
		return r.seqByID[owned[i].ID] > r.seqByID[owned[j].ID]
	})

	total := int64(len(owned))
	if offset >= len(owned) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(owned) {
		end = len(owned)
	}
	page := make([]store.LedgerEntry, end-offset)
	copy(page, owned[offset:end])
	return page, total, nil
}

var (
	_ store.LedgerRepository = (*InMemoryLedgerRepository)(nil)
)
