package consumer_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/events"
	"github.com/buidangphuc/team-referral/internal/store"
)

func aliceCode(t *testing.T, e *env) string {
	t.Helper()
	code, err := e.repo.CreateCode(context.Background(), "alice", "REFAAAA1111")
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func TestUserRegisteredAttributes(t *testing.T) {
	e := newEnv(t)
	code := aliceCode(t, e)

	if err := e.registered(t, "ev-1", "bob", code); err != nil {
		t.Fatalf("handler: %v", err)
	}
	ref, err := e.repo.GetReferral(context.Background(), "bob")
	if err != nil || ref.ReferrerID != "alice" || ref.Status != store.StatusAttributed {
		t.Fatalf("referral = %+v, %v", ref, err)
	}
	rows := e.rows()
	if len(rows) != 1 || rows[0].EventType != typeAttributed || rows[0].AggregateID != "bob" {
		t.Fatalf("outbox = %+v, want one ReferralAttributed keyed by bob", rows)
	}
	if rows[0].EventID != events.StableEventID("ReferralAttributed", "bob") {
		t.Fatalf("event id %q is not the stable id", rows[0].EventID)
	}
	var p referralv1.ReferralAttributed
	payloadOf(t, rows[0], &p)
	if p.GetRefereeId() != "bob" || p.GetReferrerId() != "alice" || p.GetAttributedAt() == nil {
		t.Fatalf("payload = %v", &p)
	}
}

func TestUserRegisteredUnknownCode(t *testing.T) {
	e := newEnv(t)
	if err := e.registered(t, "ev-1", "bob", "NOT-A-REAL-CODE"); err != nil {
		t.Fatalf("handler: %v", err)
	}
	assertRejected(t, e, "bob", referralv1.ReferralRejectReason_REFERRAL_REJECT_REASON_UNKNOWN_CODE)
}

func TestUserRegisteredSelfReferral(t *testing.T) {
	e := newEnv(t)
	code := aliceCode(t, e)
	if err := e.registered(t, "ev-1", "alice", code); err != nil {
		t.Fatalf("handler: %v", err)
	}
	assertRejected(t, e, "alice", referralv1.ReferralRejectReason_REFERRAL_REJECT_REASON_SELF_REFERRAL)
}

func TestUserRegisteredCappedReferrer(t *testing.T) {
	e := newEnv(t)
	e.seedRewards(t, "alice", 50)
	code, _ := e.repo.GetCodeByUser(context.Background(), "alice")
	before := len(e.rows())

	if err := e.registered(t, "ev-late", "bob", code); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if _, err := e.repo.GetReferral(context.Background(), "bob"); err == nil {
		t.Fatal("a referral row was created for a capped inviter")
	}
	rows := e.rowsOfType(typeRejected)
	if len(rows) != 1 || len(e.rows()) != before+1 {
		t.Fatalf("want exactly one new ReferralRejected row, got %d", len(e.rows())-before)
	}
	var p referralv1.ReferralRejected
	payloadOf(t, rows[0], &p)
	if p.GetReason() != referralv1.ReferralRejectReason_REFERRAL_REJECT_REASON_REFERRER_CAPPED {
		t.Fatalf("reason = %v", p.GetReason())
	}
}

func assertRejected(t *testing.T, e *env, referee string, reason referralv1.ReferralRejectReason) {
	t.Helper()
	if _, err := e.repo.GetReferral(context.Background(), referee); err == nil {
		t.Fatal("a referral row was created for a rejected signup")
	}
	rows := e.rows()
	if len(rows) != 1 || rows[0].EventType != typeRejected || rows[0].AggregateID != referee {
		t.Fatalf("outbox = %+v, want one ReferralRejected keyed by %s", rows, referee)
	}
	var p referralv1.ReferralRejected
	payloadOf(t, rows[0], &p)
	if p.GetRefereeId() != referee || p.GetReason() != reason {
		t.Fatalf("payload = %v, want reason %v", &p, reason)
	}
}

func TestUserRegisteredWithoutCode(t *testing.T) {
	e := newEnv(t)
	if err := e.registered(t, "ev-1", "bob", ""); err != nil {
		t.Fatalf("handler: %v", err)
	}
	at, err := e.repo.RegisteredAt(context.Background(), "bob")
	if err != nil || !at.Equal(t0) {
		t.Fatalf("RegisteredAt = %v, %v; want %v", at, err, t0)
	}
	if _, err := e.repo.GetReferral(context.Background(), "bob"); err == nil {
		t.Fatal("referral row exists for a signup without a code")
	}
	if n := len(e.rows()); n != 0 {
		t.Fatalf("%d outbox rows for a signup without a code, want 0", n)
	}
}

func TestUserRegisteredRedelivery(t *testing.T) {
	e := newEnv(t)
	code := aliceCode(t, e)
	for i := 0; i < 2; i++ {
		if err := e.registered(t, "ev-1", "bob", code); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}
	if n, _ := e.repo.CountReferralsByReferrer(context.Background(), "alice"); n != 1 {
		t.Fatalf("referrals for alice = %d, want 1", n)
	}
	rows := e.rows()
	if len(rows) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(rows))
	}
	if rows[0].EventID != events.StableEventID("ReferralAttributed", "bob") {
		t.Fatalf("event id %q is not stable", rows[0].EventID)
	}
}

func TestUserRegisteredForAlreadyReferredUser(t *testing.T) {
	e := newEnv(t)
	code := aliceCode(t, e)
	carolCode, _ := e.repo.CreateCode(context.Background(), "carol", "REFCCCC2222")
	if err := e.registered(t, "ev-1", "bob", code); err != nil {
		t.Fatal(err)
	}
	before := len(e.rows())

	if err := e.registered(t, "ev-2", "bob", carolCode); err != nil {
		t.Fatalf("handler: %v", err)
	}
	ref, err := e.repo.GetReferral(context.Background(), "bob")
	if err != nil || ref.ReferrerID != "alice" || ref.Status != store.StatusAttributed {
		t.Fatalf("referral changed: %+v, %v", ref, err)
	}
	if n := len(e.rows()); n != before {
		t.Fatalf("outbox grew from %d to %d for an already referred user", before, n)
	}
}

func TestMalformedUserRegisteredIsDeadLettered(t *testing.T) {
	e := newEnv(t)
	code := aliceCode(t, e)

	bad := []consume.Record{
		{Topic: "identity.events", Key: []byte("bob"), Value: rawEnvelope(t, "ev-bad", typeUserRegistered, []byte{0xff, 0xff, 0xff, 0xff})},
		{Topic: "identity.events", Key: []byte("bob"), Value: []byte{0xff, 0xfe, 0xfd}},
	}
	for i, rec := range bad {
		err := e.identity(context.Background(), rec)
		if err == nil || !isPermanent(err) {
			t.Fatalf("record %d: err = %v, want a consume.Permanent error so the record is dead-lettered", i, err)
		}
	}
	if _, err := e.repo.RegisteredAt(context.Background(), "bob"); err == nil {
		t.Fatal("a row was written for a malformed event")
	}
	if n := len(e.rows()); n != 0 {
		t.Fatalf("%d outbox rows after malformed events", n)
	}
	if err := e.registered(t, "ev-ok", "bob", code); err != nil {
		t.Fatalf("next valid event: %v", err)
	}
	if _, err := e.repo.GetReferral(context.Background(), "bob"); err != nil {
		t.Fatalf("next valid event was not processed: %v", err)
	}
}

func TestOtherEnvelopeTypesAreIgnored(t *testing.T) {
	e := newEnv(t)
	other := consume.Record{Topic: "identity.events", Key: []byte("bob"), Value: rawEnvelope(t, "ev-x", "platform.identity.v1.UserStatusChanged", []byte{0xff, 0xff})}
	if err := e.identity(context.Background(), other); err != nil {
		t.Fatalf("identity handler: %v", err)
	}
	otherOrder := consume.Record{Topic: "order.events", Key: []byte("o"), Value: rawEnvelope(t, "ev-y", "platform.order.v1.OrderCreated", []byte{0xff, 0xff})}
	if err := e.order(context.Background(), otherOrder); err != nil {
		t.Fatalf("order handler: %v", err)
	}
	if n := len(e.rows()); n != 0 {
		t.Fatalf("%d outbox rows for ignored events", n)
	}
}
