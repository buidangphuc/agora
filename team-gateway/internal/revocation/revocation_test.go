package revocation

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-gateway/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
)

func envelope(t *testing.T, typ string, ev *identityv1.SessionRevoked) []byte {
	t.Helper()
	payload, err := proto.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "e", Type: typ, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func revoked(sid string, exp time.Time) *identityv1.SessionRevoked {
	return &identityv1.SessionRevoked{SessionId: sid, UserId: "u1", ExpiresAt: timestamppb.New(exp)}
}

func fixedClock(d *Denylist, now *time.Time) { d.now = func() time.Time { return *now } }

func TestApplyAddsRevokedSession(t *testing.T) {
	d := NewDenylist()
	if d.Revoked("s1") {
		t.Fatal("empty denylist must not revoke anything")
	}
	if err := Apply(d, envelope(t, sessionRevokedType, revoked("s1", time.Now().Add(time.Hour)))); err != nil {
		t.Fatal(err)
	}
	if !d.Revoked("s1") {
		t.Fatal("s1 should be revoked")
	}
	if d.Revoked("s2") {
		t.Fatal("s2 was never revoked")
	}
}

func TestApplyIgnoresOtherEventTypesAndEmptySession(t *testing.T) {
	d := NewDenylist()
	if err := Apply(d, envelope(t, "platform.listing.v1.ListingChanged", revoked("s1", time.Now().Add(time.Hour)))); err != nil {
		t.Fatal(err)
	}
	if err := Apply(d, envelope(t, sessionRevokedType, revoked("", time.Now().Add(time.Hour)))); err != nil {
		t.Fatal(err)
	}
	if d.Len() != 0 {
		t.Fatalf("nothing should be held, got %d", d.Len())
	}
}

func TestApplyRejectsUndecodableRecord(t *testing.T) {
	if err := Apply(NewDenylist(), []byte{0xff, 0xff, 0xff}); err == nil {
		t.Fatal("want a decode error for a poison record")
	}
}

// Duplicate delivery (at-least-once) and a full replay from the earliest offset
// converge on the same state; the later expiry wins.
func TestApplyDuplicatesAndReplayAreIdempotent(t *testing.T) {
	exp := time.Now().Add(time.Hour)
	stream := [][]byte{
		envelope(t, sessionRevokedType, revoked("s1", exp)),
		envelope(t, sessionRevokedType, revoked("s2", exp)),
		envelope(t, sessionRevokedType, revoked("s1", exp)), // duplicate
	}
	d := NewDenylist()
	for round := 0; round < 2; round++ { // second round = replay after restart
		for _, rec := range stream {
			if err := Apply(d, rec); err != nil {
				t.Fatal(err)
			}
		}
	}
	if d.Len() != 2 || !d.Revoked("s1") || !d.Revoked("s2") {
		t.Fatalf("want exactly s1 and s2 revoked, len=%d", d.Len())
	}

	later := exp.Add(time.Hour)
	_ = Apply(d, envelope(t, sessionRevokedType, revoked("s1", later)))
	_ = Apply(d, envelope(t, sessionRevokedType, revoked("s1", exp))) // older duplicate must not shorten it
	now := exp.Add(30 * time.Minute)
	fixedClock(d, &now)
	if !d.Revoked("s1") {
		t.Fatal("s1 should still be revoked until the later expiry")
	}
}

func TestExpiredEntriesAreRejectedAndPruned(t *testing.T) {
	now := time.Now()
	d := NewDenylist()
	fixedClock(d, &now)

	d.Add("past", now.Add(-time.Second)) // already expired: never stored
	if d.Len() != 0 {
		t.Fatalf("an already-expired entry must not be stored, len=%d", d.Len())
	}
	d.Add("s1", now.Add(time.Minute))
	d.Add("s2", now.Add(time.Hour))
	if !d.Revoked("s1") {
		t.Fatal("s1 revoked before expiry")
	}

	now = now.Add(2 * time.Minute) // s1's token has now expired
	if d.Revoked("s1") {
		t.Fatal("s1 must no longer be reported after its expiry")
	}
	if n := d.Prune(); n != 1 {
		t.Fatalf("Prune removed %d, want 1", n)
	}
	if d.Len() != 1 || !d.Revoked("s2") {
		t.Fatalf("only s2 should remain, len=%d", d.Len())
	}
}

func TestConsumerStatsStartDown(t *testing.T) {
	d := NewDenylist()
	d.Add("s1", time.Now().Add(time.Hour))
	c := NewConsumer([]string{"localhost:1"}, "identity.events", d, nil)
	s := c.Stats()
	if s.Up || s.Lag != 0 || s.DenylistSize != 1 {
		t.Fatalf("unexpected initial stats: %+v", s)
	}
}

// Kafka being unreachable must never block or crash the gateway: Run keeps
// retrying in the background, reports up=false, and stops promptly on shutdown.
func TestRunWithUnreachableKafkaFailsOpen(t *testing.T) {
	d := NewDenylist()
	c := NewConsumer([]string{"127.0.0.1:1"}, "identity.events", d, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()

	time.Sleep(700 * time.Millisecond)
	if c.Stats().Up {
		t.Fatal("up must be false while Kafka is unreachable")
	}
	if d.Revoked("anything") {
		t.Fatal("fail open: nothing is revoked without the topic")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}
