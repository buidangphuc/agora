package store

import (
	"context"
	"time"
)

// HoldSellerListings mirrors PgTxWriter: it holds a seller's published and
// pending_review listings, remembering the prior status.
func (w *InMemoryTxWriter) HoldSellerListings(ctx context.Context, userID, _ string, _ time.Time, enqueue EnqueueFn) (int, error) {
	return w.moveSeller(ctx, userID, enqueue, func(l Listing) (Listing, bool) {
		if l.Status != "published" && l.Status != "pending_review" {
			return l, false
		}
		w.heldPrior[l.ID] = l.Status
		l.Status = "held"
		l.ModerationReasonCode = ReasonSellerEnforcement
		return l, true
	})
}

// RestoreSellerListings mirrors PgTxWriter: held listings return to their prior
// status without any screening.
func (w *InMemoryTxWriter) RestoreSellerListings(ctx context.Context, userID, _ string, _ time.Time, enqueue EnqueueFn) (int, error) {
	return w.moveSeller(ctx, userID, enqueue, func(l Listing) (Listing, bool) {
		prior, ok := w.heldPrior[l.ID]
		if l.Status != "held" || !ok {
			return l, false
		}
		delete(w.heldPrior, l.ID)
		l.Status = prior
		l.ModerationReasonCode = ""
		return l, true
	})
}

func (w *InMemoryTxWriter) moveSeller(ctx context.Context, userID string, enqueue EnqueueFn, change func(Listing) (Listing, bool)) (int, error) {
	w.mu.Lock()
	if w.heldPrior == nil {
		w.heldPrior = map[string]string{}
	}
	w.mu.Unlock()
	w.repo.mu.RLock()
	var mine []Listing
	for _, l := range w.repo.byID {
		if l.SellerID == userID {
			mine = append(mine, l)
		}
	}
	w.repo.mu.RUnlock()
	n := 0
	for _, l := range mine {
		w.mu.Lock()
		out, ok := change(l)
		w.mu.Unlock()
		if !ok {
			continue
		}
		if _, err := w.UpdateTx(ctx, out, enqueue); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
