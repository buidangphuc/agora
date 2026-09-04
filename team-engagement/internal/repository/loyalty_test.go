package repository

import (
	"context"
	"testing"
	"time"
)

var checkInDay = time.Date(2026, 9, 4, 10, 30, 0, 0, time.UTC)

// first check-in: awards CheckInCoins, sets streak to 1, and records the day.
func TestLoyalty_FirstCheckInAwardsAndStartsStreak(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	loy, earned, err := r.CheckIn(ctx, "userA", checkInDay)
	if err != nil {
		t.Fatalf("check-in: %v", err)
	}
	if earned != CheckInCoins {
		t.Fatalf("expected %d coins earned, got %d", CheckInCoins, earned)
	}
	if loy.Streak != 1 {
		t.Fatalf("expected streak 1, got %d", loy.Streak)
	}
	if loy.CoinBalance != CheckInCoins {
		t.Fatalf("expected balance %d, got %d", CheckInCoins, loy.CoinBalance)
	}
	if !loy.LastCheckin.Equal(dayOf(checkInDay)) {
		t.Fatalf("expected last_checkin %v, got %v", dayOf(checkInDay), loy.LastCheckin)
	}

	// GetLoyalty mirrors the snapshot returned by CheckIn.
	snap, err := r.GetLoyalty(ctx, "userA")
	if err != nil {
		t.Fatalf("get loyalty: %v", err)
	}
	if snap.Streak != 1 || snap.CoinBalance != CheckInCoins {
		t.Fatalf("snapshot mismatch: %+v", snap)
	}
}

// same-day idempotency: a second check-in on the same calendar day is a no-op —
// no extra coins, streak unchanged. Different clock time, same day.
func TestLoyalty_SameDayCheckInIsIdempotent(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	if _, _, err := r.CheckIn(ctx, "userA", checkInDay); err != nil {
		t.Fatalf("first check-in: %v", err)
	}

	later := checkInDay.Add(5 * time.Hour) // same day, later hour
	loy, earned, err := r.CheckIn(ctx, "userA", later)
	if err != nil {
		t.Fatalf("second check-in: %v", err)
	}
	if earned != 0 {
		t.Fatalf("expected 0 coins on same-day re-check-in, got %d", earned)
	}
	if loy.Streak != 1 {
		t.Fatalf("expected streak still 1, got %d", loy.Streak)
	}
	if loy.CoinBalance != CheckInCoins {
		t.Fatalf("expected balance unchanged at %d, got %d", CheckInCoins, loy.CoinBalance)
	}
}

// consecutive days advance the streak and keep accumulating coins.
func TestLoyalty_ConsecutiveDaysAdvanceStreak(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	day1 := checkInDay
	day2 := checkInDay.AddDate(0, 0, 1)
	day3 := checkInDay.AddDate(0, 0, 2)

	for i, d := range []time.Time{day1, day2, day3} {
		loy, earned, err := r.CheckIn(ctx, "userA", d)
		if err != nil {
			t.Fatalf("check-in day %d: %v", i+1, err)
		}
		if earned != CheckInCoins {
			t.Fatalf("day %d: expected %d coins, got %d", i+1, CheckInCoins, earned)
		}
		if want := int32(i + 1); loy.Streak != want {
			t.Fatalf("day %d: expected streak %d, got %d", i+1, want, loy.Streak)
		}
	}

	snap, _ := r.GetLoyalty(ctx, "userA")
	if snap.CoinBalance != 3*CheckInCoins {
		t.Fatalf("expected balance %d after 3 days, got %d", 3*CheckInCoins, snap.CoinBalance)
	}
}

// skipped day resets the streak to 1 (but still awards coins and keeps balance).
func TestLoyalty_SkippedDayResetsStreak(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	if _, _, err := r.CheckIn(ctx, "userA", checkInDay); err != nil {
		t.Fatalf("day 1 check-in: %v", err)
	}
	// Consecutive day → streak 2.
	loy, _, err := r.CheckIn(ctx, "userA", checkInDay.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("day 2 check-in: %v", err)
	}
	if loy.Streak != 2 {
		t.Fatalf("expected streak 2, got %d", loy.Streak)
	}

	// Skip a day: jump two days ahead → streak resets to 1.
	loy, earned, err := r.CheckIn(ctx, "userA", checkInDay.AddDate(0, 0, 3))
	if err != nil {
		t.Fatalf("day 4 check-in: %v", err)
	}
	if loy.Streak != 1 {
		t.Fatalf("expected streak reset to 1 after skipped day, got %d", loy.Streak)
	}
	if earned != CheckInCoins {
		t.Fatalf("expected %d coins on reset check-in, got %d", CheckInCoins, earned)
	}
	if loy.CoinBalance != 3*CheckInCoins {
		t.Fatalf("expected balance %d (3 check-ins), got %d", 3*CheckInCoins, loy.CoinBalance)
	}
}

// cross-user isolation: one user's loyalty state never leaks into another's.
func TestLoyalty_CrossUserIsolation(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	if _, _, err := r.CheckIn(ctx, "userA", checkInDay); err != nil {
		t.Fatalf("userA check-in: %v", err)
	}

	// userB has never checked in → zero-value snapshot, no panic.
	snap, err := r.GetLoyalty(ctx, "userB")
	if err != nil {
		t.Fatalf("get loyalty userB: %v", err)
	}
	if snap.Streak != 0 || snap.CoinBalance != 0 || !snap.LastCheckin.IsZero() {
		t.Fatalf("expected empty snapshot for userB, got %+v", snap)
	}

	// userB's first check-in is independent of userA's.
	loy, _, err := r.CheckIn(ctx, "userB", checkInDay.AddDate(0, 0, 5))
	if err != nil {
		t.Fatalf("userB check-in: %v", err)
	}
	if loy.Streak != 1 {
		t.Fatalf("expected userB streak 1, got %d", loy.Streak)
	}

	aSnap, _ := r.GetLoyalty(ctx, "userA")
	if aSnap.CoinBalance != CheckInCoins || aSnap.Streak != 1 {
		t.Fatalf("userA snapshot changed by userB activity: %+v", aSnap)
	}
}
