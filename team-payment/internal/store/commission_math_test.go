package store_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/buidangphuc/team-payment/internal/store"
)

func bigCommission(base, bps int64) int64 {
	n := new(big.Int).Mul(big.NewInt(base), big.NewInt(bps))
	n.Add(n, big.NewInt(5000))
	return n.Div(n, big.NewInt(10000)).Int64()
}

func bigTarget(commission, refunded, paid int64) int64 {
	num := new(big.Int).Mul(big.NewInt(commission), big.NewInt(refunded))
	num.Mul(num, big.NewInt(2))
	num.Add(num, big.NewInt(paid))
	den := new(big.Int).Mul(big.NewInt(2), big.NewInt(paid))
	return num.Div(num, den).Int64()
}

func TestCommissionAmount_RoundsHalfUpToWholeVND(t *testing.T) {
	// scenario: Commission rounds half up to whole VND
	cases := []struct {
		name      string
		base, bps int64
		want      int64
	}{
		{"half rounds up", 100010, 500, 5001},
		{"below half rounds down", 100001, 500, 5000},
		{"exact", 280000, 500, 14000},
		{"platform voucher outside base", 300000, 500, 15000},
		{"max rate", 100000, 3000, 30000},
		{"rate zero", 100000, 0, 0},
		{"zero base", 0, 500, 0},
		{"negative base is clamped to zero", -100, 500, 0},
		{"just under half", 9, 500, 0},
		{"exactly half", 10, 500, 1},
		{"tiny base", 5, 500, 0},
		{"one bps", 10000, 1, 1},
		{"one bps rounds down", 4999, 1, 0},
		{"one bps rounds up", 5000, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := store.CommissionAmount(tc.base, tc.bps); got != tc.want {
				t.Fatalf("CommissionAmount(%d, %d) = %d, want %d", tc.base, tc.bps, got, tc.want)
			}
		})
	}
}

func TestCommissionAmount_DoesNotOverflow(t *testing.T) {
	// scenario: Commission rounds half up to whole VND
	for _, base := range []int64{math.MaxInt64, math.MaxInt64 / 2, 1 << 62, 1_000_000_000_000_000_000} {
		for _, bps := range []int64{1, 500, 3000} {
			if got, want := store.CommissionAmount(base, bps), bigCommission(base, bps); got != want {
				t.Fatalf("CommissionAmount(%d, %d) = %d, want %d", base, bps, got, want)
			}
		}
	}
}

func TestReversalTarget_ProportionalHalfUp(t *testing.T) {
	// scenario: Cumulative rounding never exceeds the commission
	cases := []struct {
		name                       string
		commission, refunded, paid int64
		want                       int64
	}{
		{"40 percent", 14000, 124000, 310000, 5600},
		{"remainder", 14000, 310000, 310000, 14000},
		{"tiny share rounds to zero", 14000, 1, 310000, 0},
		{"nothing refunded", 14000, 0, 310000, 0},
		{"no commission", 0, 124000, 310000, 0},
		{"just under half", 1, 154999, 310000, 0},
		{"exactly half rounds up", 1, 155000, 310000, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := store.ReversalTarget(tc.commission, tc.refunded, tc.paid); got != tc.want {
				t.Fatalf("ReversalTarget(%d, %d, %d) = %d, want %d", tc.commission, tc.refunded, tc.paid, got, tc.want)
			}
		})
	}
}

func TestReversalTarget_CumulativeNeverExceedsCommissionAndEndsExact(t *testing.T) {
	// scenario: Cumulative rounding never exceeds the commission
	const commission, paid = 14000, 310000
	var prev int64
	for refunded := int64(1000); refunded <= paid; refunded += 1000 {
		got := store.ReversalTarget(commission, refunded, paid)
		if got != bigTarget(commission, refunded, paid) {
			t.Fatalf("ReversalTarget(%d, %d, %d) = %d, want %d", commission, refunded, paid, got, bigTarget(commission, refunded, paid))
		}
		if got < prev || got > commission {
			t.Fatalf("target %d after refunding %d is not monotonic within [0, %d] (prev %d)", got, refunded, commission, prev)
		}
		prev = got
	}
	if prev != commission {
		t.Fatalf("final target = %d, want %d", prev, commission)
	}
}

func TestReversalTarget_DoesNotOverflow(t *testing.T) {
	// scenario: Cumulative rounding never exceeds the commission
	cases := [][3]int64{
		{math.MaxInt64 / 2, math.MaxInt64 / 3, math.MaxInt64 / 2},
		{1 << 60, 1 << 61, 1 << 62},
		{math.MaxInt64 / 4, math.MaxInt64, math.MaxInt64},
	}
	for _, c := range cases {
		if got, want := store.ReversalTarget(c[0], c[1], c[2]), bigTarget(c[0], c[1], c[2]); got != want {
			t.Fatalf("ReversalTarget(%d, %d, %d) = %d, want %d", c[0], c[1], c[2], got, want)
		}
	}
}

func TestReversalTarget_NonPositivePaidIsZero(t *testing.T) {
	// scenario: A zero commission writes no row
	if got := store.ReversalTarget(14000, 100, 0); got != 0 {
		t.Fatalf("ReversalTarget with paid 0 = %d, want 0", got)
	}
}
