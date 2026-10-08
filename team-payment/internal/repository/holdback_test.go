package repository_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/team-payment/internal/repository"
)

const window = 7 * 24 * time.Hour

// creditAt books a referenced settlement credit for seller created at `at` and returns
// the reference (the payment transaction it belongs to, so refunds can follow it).
func creditAt(t *testing.T, s stores, seller, order string, amount int64, at time.Time) repository.PaymentTransaction {
	t.Helper()
	tx := paidTx(t, s, order, amount)
	if _, created, err := s.ledger.AppendEntryOnce(context.Background(), repository.LedgerEntry{
		SellerID: seller, Type: repository.LedgerTypeOrderSettlement, Amount: amount,
		Status: repository.LedgerStatusCompleted, ReferenceID: tx.ID, CreatedAt: at,
	}); err != nil || !created {
		t.Fatalf("credit: %v %v", created, err)
	}
	return tx
}

func payout(s stores, seller string, amount int64, now time.Time, w time.Duration) error {
	_, err := s.ledger.AppendDebit(context.Background(), repository.LedgerEntry{
		SellerID: seller, Type: repository.LedgerTypePayout, Amount: -amount, Status: repository.LedgerStatusPending,
	}, repository.Holdback{Window: w, Now: now})
	return err
}

func payouts(t *testing.T, s stores, seller string) int {
	got, _ := ledgerOf(t, s, seller)
	return len(got[repository.LedgerTypePayout])
}

func TestHold_FreshProceedsCannotBePaidOut(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		now := time.Now()
		creditAt(t, s, "s", "o1", 500000, now.Add(-time.Minute))
		err := payout(s, "s", 100000, now, window)
		if !errors.Is(err, repository.ErrFundsOnHold) {
			t.Fatalf("want ErrFundsOnHold, got %v", err)
		}
		if _, bal := ledgerOf(t, s, "s"); payouts(t, s, "s") != 0 || bal != 500000 {
			t.Fatalf("payouts=%d balance=%d, want 0/500000", payouts(t, s, "s"), bal)
		}
	})
}

func TestHold_ProceedsPastWindowCanBePaidOut(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		now := time.Now()
		creditAt(t, s, "s", "o1", 500000, now.Add(-window-time.Second))
		if err := payout(s, "s", 500000, now, window); err != nil {
			t.Fatalf("payout: %v", err)
		}
		got, bal := ledgerOf(t, s, "s")
		if p := got[repository.LedgerTypePayout]; len(p) != 1 || p[0].Amount != -500000 || p[0].Status != repository.LedgerStatusPending || bal != 0 {
			t.Fatalf("payouts=%+v balance=%d", p, bal)
		}
	})
}

func TestHold_OnlyInWindowProceedsAreHeld(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		now := time.Now()
		creditAt(t, s, "s", "o-old", 300000, now.Add(-window-time.Hour))
		creditAt(t, s, "s", "o-new", 500000, now.Add(-time.Minute))
		if err := payout(s, "s", 300000, now, window); err != nil {
			t.Fatalf("first payout: %v", err)
		}
		if err := payout(s, "s", 1, now, window); !errors.Is(err, repository.ErrFundsOnHold) {
			t.Fatalf("second payout: want ErrFundsOnHold, got %v", err)
		}
	})
}

func TestHold_ConcurrentPayoutsCannotOverdraw(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		now := time.Now()
		creditAt(t, s, "s", "o-old", 100000, now.Add(-window-time.Hour))
		creditAt(t, s, "s", "o-new", 500000, now.Add(-time.Minute))
		var ok, held atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				switch err := payout(s, "s", 40000, now, window); {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, repository.ErrFundsOnHold):
					held.Add(1)
				default:
					t.Errorf("unexpected: %v", err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if _, bal := ledgerOf(t, s, "s"); ok.Load() != 2 || held.Load() != 6 || bal != 520000 {
			t.Fatalf("ok=%d held=%d balance=%d, want 2/6/520000", ok.Load(), held.Load(), bal)
		}
	})
}

func TestHold_RefundOfHeldSaleLeavesOlderProceedsWithdrawable(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		now := time.Now()
		creditAt(t, s, "s", "o-old", 300000, now.Add(-window-time.Hour))
		fresh := creditAt(t, s, "s", "o-new", 500000, now.Add(-time.Minute))
		if _, err := rpcRefund(context.Background(), s, fresh.ID, "r1", 500000); err != nil {
			t.Fatal(err)
		}
		if err := payout(s, "s", 300000, now, window); err != nil {
			t.Fatalf("payout of the older proceeds: %v", err)
		}
	})
}

func TestHold_RefundPastWindowReducesWithdrawable(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		now := time.Now()
		old := creditAt(t, s, "s", "o-old", 500000, now.Add(-window-time.Hour))
		if _, err := rpcRefund(context.Background(), s, old.ID, "r1", 200000); err != nil {
			t.Fatal(err)
		}
		if err := payout(s, "s", 300001, now, window); !errors.Is(err, repository.ErrInsufficientBalance) {
			t.Fatalf("want ErrInsufficientBalance, got %v", err)
		}
		if err := payout(s, "s", 300000, now, window); err != nil {
			t.Fatalf("payout 300000: %v", err)
		}
	})
}

func TestHold_AboveBalanceIsInsufficient(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		now := time.Now()
		creditAt(t, s, "s", "o1", 500000, now.Add(-time.Minute))
		if err := payout(s, "s", 500001, now, window); !errors.Is(err, repository.ErrInsufficientBalance) || errors.Is(err, repository.ErrFundsOnHold) {
			t.Fatalf("want ErrInsufficientBalance, got %v", err)
		}
	})
}

func TestHold_MessageNamesReleaseInstantWithoutAmounts(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		now := time.Now()
		w := 20 * time.Second
		at := now.Add(-5 * time.Second).Truncate(time.Second)
		creditAt(t, s, "s", "o1", 500000, at)
		err := payout(s, "s", 100000, now, w)
		var held *repository.FundsOnHoldError
		if !errors.As(err, &held) {
			t.Fatalf("want *FundsOnHoldError, got %v", err)
		}
		if !held.Until.Equal(at.Add(w)) {
			t.Fatalf("until = %s, want %s", held.Until, at.Add(w))
		}
		msg := err.Error()
		if !strings.HasPrefix(msg, "amount is held until ") || !strings.HasSuffix(msg, " (refund window)") {
			t.Fatalf("message shape: %q", msg)
		}
		stamp := strings.TrimSuffix(strings.TrimPrefix(msg, "amount is held until "), " (refund window)")
		parsed, perr := time.Parse(time.RFC3339, stamp)
		if perr != nil || !parsed.Equal(at.Add(w)) || !strings.HasSuffix(stamp, "Z") {
			t.Fatalf("timestamp %q: %v", stamp, perr)
		}
		if strings.Contains(msg, "100000") || strings.Contains(msg, "500000") {
			t.Fatalf("message reveals an amount: %q", msg)
		}
	})
}

// The release instant is when enough credits, oldest first, have left the window.
func TestHold_ReleaseInstantOldestFirst(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		now := time.Now().Truncate(time.Second)
		creditAt(t, s, "s", "o1", 100, now.Add(-3*time.Hour))
		creditAt(t, s, "s", "o2", 100, now.Add(-2*time.Hour))
		creditAt(t, s, "s", "o3", 100, now.Add(-1*time.Hour))
		var held *repository.FundsOnHoldError
		if err := payout(s, "s", 150, now, window); !errors.As(err, &held) || !held.Until.Equal(now.Add(-2*time.Hour+window)) {
			t.Fatalf("150 needs the two oldest credits: %v", err)
		}
	})
}

func TestHold_ZeroWindowDisables(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		now := time.Now()
		creditAt(t, s, "s", "o1", 500000, now)
		if err := payout(s, "s", 500000, now, 0); err != nil {
			t.Fatalf("window 0 must not hold: %v", err)
		}
	})
}
