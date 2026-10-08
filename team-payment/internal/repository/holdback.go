package repository

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrFundsOnHold is returned by a payout whose amount is within the wallet balance but
// above the withdrawable amount because recent sale proceeds are still inside the
// refund hold window. Match with errors.Is; errors.As gives the *FundsOnHoldError.
var ErrFundsOnHold = errors.New("amount is held (refund window)")

// FundsOnHoldError carries the instant the requested amount becomes withdrawable. It
// deliberately holds no amounts: callers must not reveal more than the instant.
type FundsOnHoldError struct {
	Until time.Time
}

// Error is the caller-facing message: `amount is held until <RFC3339 UTC> (refund window)`.
func (e *FundsOnHoldError) Error() string {
	return fmt.Sprintf("amount is held until %s (refund window)", e.Until.UTC().Format(time.RFC3339))
}

func (e *FundsOnHoldError) Is(target error) bool { return target == ErrFundsOnHold }

// Holdback is the payout hold-back policy at one instant: COMPLETED ORDER_SETTLEMENT
// credits created after Now-Window are held. A zero Window disables the hold.
type Holdback struct {
	Window time.Duration
	Now    time.Time
}

// Enabled reports whether any hold applies.
func (h Holdback) Enabled() bool { return h.Window > 0 }

// Cutoff is Now-Window: a credit is held only when created strictly after it.
func (h Holdback) Cutoff() time.Time { return h.Now.Add(-h.Window) }

// heldCredit is one in-window credit net of the refund deductions booked against it.
type heldCredit struct {
	CreatedAt time.Time
	Net       int64
}

// Withdrawal is a seller's money split at an instant.
type Withdrawal struct {
	Balance      int64 // full ledger sum (the wallet balance)
	Held         int64 // unrefunded proceeds still inside the hold window
	Withdrawable int64 // max(0, Balance-Held)
}

// computeWithdrawal derives the split (design D7). Each credit is held for its
// unrefunded remainder only (never below 0), so refunding a held sale consumes the money
// held for it instead of free money; payouts never lower Held.
func computeWithdrawal(balance int64, credits []heldCredit) Withdrawal {
	var held int64
	for _, c := range credits {
		if c.Net > 0 {
			held += c.Net
		}
	}
	w := Withdrawal{Balance: balance, Held: held, Withdrawable: balance - held}
	if w.Withdrawable < 0 {
		w.Withdrawable = 0
	}
	return w
}

// releaseInstant is when enough held credits have left the window (oldest first, each at
// CreatedAt+Window) for amount to be withdrawable (design D8).
func releaseInstant(balance, amount int64, credits []heldCredit, h Holdback) time.Time {
	sorted := append([]heldCredit(nil), credits...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })
	held := computeWithdrawal(balance, credits).Held
	until := h.Now
	for _, c := range sorted {
		if balance-held >= amount {
			break
		}
		if c.Net > 0 {
			held -= c.Net
			until = c.CreatedAt.Add(h.Window)
		}
	}
	return until
}

// checkWithdrawable returns nil when amount may be paid out, ErrInsufficientBalance when
// it exceeds the balance, and a *FundsOnHoldError when it exceeds the withdrawable amount.
func checkWithdrawable(balance, amount int64, credits []heldCredit, h Holdback) error {
	if balance < amount {
		return ErrInsufficientBalance
	}
	if !h.Enabled() {
		return nil
	}
	if computeWithdrawal(balance, credits).Withdrawable < amount {
		return &FundsOnHoldError{Until: releaseInstant(balance, amount, credits, h)}
	}
	return nil
}
