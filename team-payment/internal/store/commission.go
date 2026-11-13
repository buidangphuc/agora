package store

import (
	"math"
	"math/big"
)

type Commissionable struct {
	PlatformDiscount int64
	Base             int64
	RateBps          int
	OK               bool
}

const (
	LedgerTypeCommission         = "COMMISSION"
	LedgerTypeCommissionReversal = "COMMISSION_REVERSAL"
)

func CommissionAmount(base int64, rateBps int64) int64 {
	if base <= 0 || rateBps <= 0 {
		return 0
	}
	n := new(big.Int).Mul(big.NewInt(base), big.NewInt(rateBps))
	n.Add(n, big.NewInt(5000))
	return n.Div(n, big.NewInt(10000)).Int64()
}

func ReversalTarget(commission, refunded, paid int64) int64 {
	if commission <= 0 || refunded <= 0 || paid <= 0 {
		return 0
	}
	num := new(big.Int).Mul(big.NewInt(commission), big.NewInt(refunded))
	num.Mul(num, big.NewInt(2))
	num.Add(num, big.NewInt(paid))
	got := num.Div(num, new(big.Int).Mul(big.NewInt(2), big.NewInt(paid))).Int64()
	return min(got, commission)
}

func (c Commissionable) creditAmount(paid int64) int64 {
	if !c.OK {
		return paid
	}
	return paid + c.PlatformDiscount
}

func (c Commissionable) commission() (amount int64, rateBps int) {
	if !c.OK {
		return 0, 0
	}
	return CommissionAmount(c.Base, int64(c.RateBps)), c.RateBps
}

func (c Commissionable) CreditAmount(paid int64) int64 { return c.creditAmount(paid) }

func (c Commissionable) CommissionEntry(sellerID, paymentID string) (LedgerEntry, bool) {
	amount, rate := c.commission()
	if amount <= 0 || rate < 0 || rate > math.MaxInt32 {
		return LedgerEntry{}, false
	}
	return LedgerEntry{
		SellerID:    sellerID,
		Type:        LedgerTypeCommission,
		Amount:      -amount,
		Status:      LedgerStatusCompleted,
		ReferenceID: paymentID,
		RateBps:     int32(rate),
	}, true
}

func ReversalEntry(sellerID, refundKey string, amount int64) LedgerEntry {
	return LedgerEntry{
		SellerID:    sellerID,
		Type:        LedgerTypeCommissionReversal,
		Amount:      amount,
		Status:      LedgerStatusCompleted,
		ReferenceID: refundKey,
	}
}
