package consumer_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
)

func TestEmptyCodeIssuesWelcome(t *testing.T) {
	e := newEnv(t)
	e.mustRegister(t, "user-u1", "")
	v := e.requireOne(t, "user-u1", 20000)
	if v.Kind != 2 || v.Scope != 2 || v.DiscountType != 2 || v.MinSpend != 100000 || v.Quota != 1 || v.PerUser != 1 || v.Used != 0 || v.NewUserOnly || !v.Active {
		t.Fatalf("voucher = %+v, want kind 2, PLATFORM(2), FIXED(2), min_spend 100000, quota 1, per_user_limit 1, unused, active", v)
	}
	if !v.StartsAt.Equal(t0) || v.EndsAt.Sub(v.StartsAt) != 336*time.Hour {
		t.Fatalf("window = %v .. %v, want issue time and +336h", v.StartsAt, v.EndsAt)
	}
	if v.Code == "" || strings.Contains(strings.ToLower(v.Code), "u1") {
		t.Fatalf("voucher code %q is empty or derivable from the user id", v.Code)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM promotion_outbox`); n != 0 {
		t.Fatalf("outbox rows = %d, want 0 (no VoucherChanged for a NEW_USER voucher)", n)
	}
}

func TestIssuedCodesAreUniquePerUser(t *testing.T) {
	e := newEnv(t)
	e.mustRegister(t, "user-u1", "")
	e.mustRegister(t, "user-u2", "")
	a := e.requireOne(t, "user-u1", 20000)
	b := e.requireOne(t, "user-u2", 20000)
	if a.Code == b.Code {
		t.Fatalf("two users share the voucher code %q", a.Code)
	}
}

func TestWaitsThenInvited(t *testing.T) {
	e := newEnv(t)
	e.mustRegister(t, "user-u1", "C")
	e.requireNone(t, "user-u1")
	e.at(t0.Add(5 * time.Second))
	e.mustVerdict(t, typeAttributed, "user-u1")
	v := e.requireOne(t, "user-u1", 30000)
	if v.MinSpend != 100000 || v.Quota != 1 || v.PerUser != 1 || v.Kind != 2 {
		t.Fatalf("invited voucher = %+v", v)
	}
	if !v.StartsAt.Equal(t0.Add(5*time.Second)) || v.EndsAt.Sub(v.StartsAt) != 336*time.Hour {
		t.Fatalf("window = %v .. %v, want issue time and +336h", v.StartsAt, v.EndsAt)
	}
}

func TestRejectedGetsWelcome(t *testing.T) {
	e := newEnv(t)
	e.mustRegister(t, "user-u1", "C")
	e.requireNone(t, "user-u1")
	e.mustVerdict(t, typeRejected, "user-u1")
	e.requireOne(t, "user-u1", 20000)
}

func TestVerdictBeforeRegistration(t *testing.T) {
	e := newEnv(t)
	e.mustVerdict(t, typeAttributed, "user-u1")
	e.requireNone(t, "user-u1")
	e.mustRegister(t, "user-u1", "C")
	e.requireOne(t, "user-u1", 30000)
}

func TestRejectedBeforeRegistration(t *testing.T) {
	e := newEnv(t)
	e.mustVerdict(t, typeRejected, "user-u1")
	e.requireNone(t, "user-u1")
	e.mustRegister(t, "user-u1", "C")
	e.requireOne(t, "user-u1", 20000)
}

func TestRedeliveryIdempotent(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 2; i++ {
		if err := e.registered(t, "ur-same", "user-u1", ""); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}
	e.requireOne(t, "user-u1", 20000)
	if n := count(t, e.pool, `SELECT count(*) FROM promotion_processed_events`); n != 1 {
		t.Fatalf("processed-event rows = %d, want 1 for one event id", n)
	}
	if err := e.registered(t, "ur-other-id", "user-u1", ""); err != nil {
		t.Fatalf("second UserRegistered with a new event id: %v", err)
	}
	e.requireOne(t, "user-u1", 20000)

	if err := e.registered(t, "ur-c", "user-u2", "C"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := e.referral(context.Background(), verdictRecord(t, "vd-same", typeAttributed, "user-u2")); err != nil {
			t.Fatalf("verdict delivery %d: %v", i, err)
		}
	}
	e.requireOne(t, "user-u2", 30000)
	if n := count(t, e.pool, `SELECT count(*) FROM vouchers WHERE kind = 2`); n != 2 {
		t.Fatalf("new-user vouchers = %d, want 2", n)
	}
}

func TestLateVerdictIgnored(t *testing.T) {
	e := newEnv(t)
	e.mustRegister(t, "user-u1", "C")
	e.mustVerdict(t, typeAttributed, "user-u1")
	e.requireOne(t, "user-u1", 30000)
	e.mustVerdict(t, typeRejected, "user-u1")
	e.mustVerdict(t, typeAttributed, "user-u1")
	e.requireOne(t, "user-u1", 30000)

	e.mustRegister(t, "user-u2", "C")
	e.mustVerdict(t, typeRejected, "user-u2")
	e.mustVerdict(t, typeAttributed, "user-u2")
	e.requireOne(t, "user-u2", 20000)

	e.mustRegister(t, "user-u3", "")
	e.mustVerdict(t, typeAttributed, "user-u3")
	e.requireOne(t, "user-u3", 20000)
}

func TestVerdictTimeoutFallback(t *testing.T) {
	e := newEnv(t)
	e.mustRegister(t, "user-u1", "C")
	e.at(t0.Add(verdictTimeout + time.Second))
	n, err := e.sweeper.Sweep(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("Sweep = %d, %v, want 1 issued", n, err)
	}
	v := e.requireOne(t, "user-u1", 20000)
	if !v.StartsAt.Equal(t0.Add(verdictTimeout + time.Second)) {
		t.Fatalf("starts_at = %v, want the sweep time", v.StartsAt)
	}
	n, err = e.sweeper.Sweep(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("second Sweep = %d, %v, want 0", n, err)
	}
	e.requireOne(t, "user-u1", 20000)
}

func TestLateAttributedDoesNotUpgrade(t *testing.T) {
	e := newEnv(t)
	e.mustRegister(t, "user-u1", "C")
	e.at(t0.Add(verdictTimeout + time.Minute))
	if _, err := e.sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.requireOne(t, "user-u1", 20000)
	e.mustVerdict(t, typeAttributed, "user-u1")
	e.requireOne(t, "user-u1", 20000)
}

func TestSweeperTimeoutBoundary(t *testing.T) {
	e := newEnv(t)
	e.mustRegister(t, "user-u1", "C")
	e.at(t0.Add(verdictTimeout - time.Second))
	if n, err := e.sweeper.Sweep(context.Background()); err != nil || n != 0 {
		t.Fatalf("Sweep 1s before the timeout = %d, %v, want 0", n, err)
	}
	e.requireNone(t, "user-u1")
	e.at(t0.Add(verdictTimeout + time.Second))
	if n, err := e.sweeper.Sweep(context.Background()); err != nil || n != 1 {
		t.Fatalf("Sweep after the timeout = %d, %v, want 1", n, err)
	}
	e.requireOne(t, "user-u1", 20000)
}

func TestSweeperIgnoresUnregisteredAndDecidedRows(t *testing.T) {
	e := newEnv(t)
	e.mustVerdict(t, typeAttributed, "user-early")
	e.mustRegister(t, "user-done", "")
	e.mustRegister(t, "user-rej", "C")
	e.mustVerdict(t, typeRejected, "user-rej")
	e.at(t0.Add(24 * time.Hour))
	if n, err := e.sweeper.Sweep(context.Background()); err != nil || n != 0 {
		t.Fatalf("Sweep = %d, %v, want 0", n, err)
	}
	e.requireNone(t, "user-early")
	e.requireOne(t, "user-done", 20000)
	e.requireOne(t, "user-rej", 20000)
}

func TestIgnoresOtherTypes(t *testing.T) {
	e := newEnv(t)
	other := consume.Record{Topic: identityTopic, Key: []byte("user-u1"), Value: rawEnvelope(t, "ev-other", "platform.identity.v1.UserLoggedIn", []byte{0xff, 0x01})}
	if err := e.identity(context.Background(), other); err != nil {
		t.Fatalf("other identity type: %v, want acknowledged (nil)", err)
	}
	released := verdictRecord(t, "ev-released", typeReleased, "user-u1")
	if err := e.referral(context.Background(), released); err != nil {
		t.Fatalf("ReferralRewardReleased: %v, want acknowledged (nil)", err)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM new_user_voucher_state`); n != 0 {
		t.Fatalf("state rows = %d, want 0", n)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM vouchers`); n != 0 {
		t.Fatalf("vouchers = %d, want 0", n)
	}
	e.mustRegister(t, "user-u1", "C")
	if err := e.referral(context.Background(), released); err != nil {
		t.Fatal(err)
	}
	e.requireNone(t, "user-u1")
	e.mustVerdict(t, typeAttributed, "user-u1")
	e.requireOne(t, "user-u1", 30000)
}

func TestMalformedEventsArePermanentAndChangeNothing(t *testing.T) {
	e := newEnv(t)
	garbage := []byte{0xff, 0xff, 0xff, 0x01}
	identity := map[string]consume.Record{
		"undecodable envelope": {Topic: identityTopic, Key: []byte("k"), Value: garbage},
		"undecodable payload":  {Topic: identityTopic, Key: []byte("k"), Value: rawEnvelope(t, "bad-1", typeUserRegistered, garbage)},
		"empty user_id":        registeredRecord(t, "bad-2", "", ""),
		"empty user_id code":   registeredRecord(t, "bad-3", "", "C"),
		"empty event id":       registeredRecord(t, "", "user-x", ""),
	}
	for name, rec := range identity {
		if err := e.identity(context.Background(), rec); err == nil || !isPermanent(err) {
			t.Errorf("identity %s: err = %v, want a consume.Permanent error", name, err)
		}
	}
	referral := map[string]consume.Record{
		"undecodable envelope":  {Topic: referralTopic, Key: []byte("k"), Value: garbage},
		"attributed payload":    {Topic: referralTopic, Key: []byte("k"), Value: rawEnvelope(t, "bad-4", typeAttributed, garbage)},
		"rejected payload":      {Topic: referralTopic, Key: []byte("k"), Value: rawEnvelope(t, "bad-5", typeRejected, garbage)},
		"attributed no referee": verdictRecord(t, "bad-6", typeAttributed, ""),
		"rejected no referee":   verdictRecord(t, "bad-7", typeRejected, ""),
	}
	for name, rec := range referral {
		if err := e.referral(context.Background(), rec); err == nil || !isPermanent(err) {
			t.Errorf("referral %s: err = %v, want a consume.Permanent error", name, err)
		}
	}
	for _, table := range []string{"vouchers", "new_user_voucher_state", "promotion_processed_events"} {
		if n := count(t, e.pool, `SELECT count(*) FROM `+table); n != 0 {
			t.Errorf("%s has %d rows after malformed events, want 0", table, n)
		}
	}
}
