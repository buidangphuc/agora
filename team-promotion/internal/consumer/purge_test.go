package consumer_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/consumer"
)

func TestPurgeOldProcessedEventsAndIssuedState(t *testing.T) {
	e := newEnv(t)
	if err := e.registered(t, "ur-issued", "user-issued", ""); err != nil {
		t.Fatal(err)
	}
	e.mustRegister(t, "user-pending", "C")
	if n := count(t, e.pool, `SELECT count(*) FROM promotion_processed_events`); n != 2 {
		t.Fatalf("processed rows = %d, want 2", n)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM new_user_voucher_state`); n != 2 {
		t.Fatalf("state rows = %d, want 2", n)
	}

	if _, err := consumer.Purge(context.Background(), e.pool, t0.Add(29*24*time.Hour)); err != nil {
		t.Fatalf("Purge at 29 days: %v", err)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM promotion_processed_events`); n != 2 {
		t.Fatalf("processed rows after a 29-day purge = %d, want 2", n)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM new_user_voucher_state`); n != 2 {
		t.Fatalf("state rows after a 29-day purge = %d, want 2", n)
	}

	if _, err := consumer.Purge(context.Background(), e.pool, t0.Add(31*24*time.Hour)); err != nil {
		t.Fatalf("Purge at 31 days: %v", err)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM promotion_processed_events`); n != 0 {
		t.Fatalf("processed rows after a 31-day purge = %d, want 0", n)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM new_user_voucher_state WHERE user_id = 'user-issued'`); n != 0 {
		t.Fatalf("issued state rows after a 31-day purge = %d, want 0", n)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM new_user_voucher_state WHERE user_id = 'user-pending'`); n != 1 {
		t.Fatalf("a still-pending state row must survive the purge, got %d", n)
	}
	e.requireOne(t, "user-issued", 20000)

	if err := e.registered(t, "ur-issued", "user-issued", ""); err != nil {
		t.Fatalf("redelivery after purge: %v", err)
	}
	e.requireOne(t, "user-issued", 20000)
}
