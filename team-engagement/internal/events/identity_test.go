package events_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-engagement/generated/platform/identity/v1"
	"github.com/buidangphuc/team-engagement/internal/events"
)

const (
	userErasedType        = "platform.identity.v1.UserErased"
	userStatusChangedType = "platform.identity.v1.UserStatusChanged"
	erasedUser            = "user-0a1b2c3d-raw-id"
)

type fakeEraser struct {
	mu     sync.Mutex
	erased []string
	err    error
}

func (f *fakeEraser) Erase(_ context.Context, userID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return 0, f.err
	}
	f.erased = append(f.erased, userID)
	return 7, nil
}

func (f *fakeEraser) ids() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.erased...)
}

func identityHandler(t *testing.T, st *fakeEraser) (consume.Handler, *syncBuffer, *kafkax.Fingerprinter) {
	t.Helper()
	fp, err := kafkax.NewFingerprinter(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return events.IdentityEventHandler(st, fp, logger), logs, fp
}

func erasedRecord(t *testing.T, userID string) consume.Record {
	t.Helper()
	return consume.Record{
		Topic: "identity.events", Key: []byte(userID),
		Value: envelopeOf(t, "evt-erase-1", userErasedType, &identityv1.UserErased{
			UserId: userID, Roles: []string{"buyer", "seller"}, ErasedAt: timestamppb.New(t0),
		}, t0),
	}
}

func isPermanent(err error) bool {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if fmt.Sprintf("%T", e) == "consume.permanentError" {
			return true
		}
	}
	return false
}

// Scenario: The erased id is not logged (UserErased is applied once per delivery)
func TestIdentityUserErasedCallsStoreWithUserID(t *testing.T) {
	st := &fakeEraser{}
	h, _, _ := identityHandler(t, st)
	if err := h(context.Background(), erasedRecord(t, erasedUser)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := st.ids(); len(got) != 1 || got[0] != erasedUser {
		t.Fatalf("store erased %v, want [%s]", got, erasedUser)
	}
}

// Scenario: The erased id is not logged
func TestIdentityErasureLogsNoRawUserID(t *testing.T) {
	st := &fakeEraser{}
	h, logs, fp := identityHandler(t, st)
	if err := h(context.Background(), erasedRecord(t, erasedUser)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	out := logs.String()
	if strings.TrimSpace(out) == "" {
		t.Fatal("handler logged nothing; a count and a hashed id are required")
	}
	if strings.Contains(out, erasedUser) {
		t.Fatalf("raw user id leaked in log: %s", out)
	}
	if !strings.Contains(out, fp.Of([]byte(erasedUser))) {
		t.Fatalf("log lacks the hashed id %s: %s", fp.Of([]byte(erasedUser)), out)
	}
}

// Scenario: The erased id is not logged (also on store failure)
func TestIdentityErasureStoreFailureLogsNoRawUserIDAndIsRetryable(t *testing.T) {
	st := &fakeEraser{err: errors.New("database unavailable")}
	h, logs, _ := identityHandler(t, st)
	err := h(context.Background(), erasedRecord(t, erasedUser))
	if err == nil || isPermanent(err) {
		t.Fatalf("store failure must be returned as retryable, got %v", err)
	}
	if strings.Contains(logs.String(), erasedUser) {
		t.Fatalf("raw user id leaked in log: %s", logs.String())
	}
}

// Scenario: A malformed event goes to the DLQ
func TestIdentityMalformedEnvelopeIsPermanent(t *testing.T) {
	st := &fakeEraser{}
	h, _, _ := identityHandler(t, st)
	err := h(context.Background(), consume.Record{Topic: "identity.events", Key: []byte("k"), Value: []byte("not a proto \xff\xff")})
	if err == nil || !isPermanent(err) {
		t.Fatalf("malformed envelope must be a permanent error, got %v", err)
	}
	if len(st.ids()) != 0 {
		t.Fatal("store was called for a malformed record")
	}
}

// Scenario: A malformed event goes to the DLQ (undecodable UserErased payload)
func TestIdentityMalformedUserErasedPayloadIsPermanent(t *testing.T) {
	st := &fakeEraser{}
	h, _, _ := identityHandler(t, st)
	env, err := proto.Marshal(mustEnvelope(userErasedType, []byte("\xff\xff\xff")))
	if err != nil {
		t.Fatal(err)
	}
	err = h(context.Background(), consume.Record{Topic: "identity.events", Value: env})
	if err == nil || !isPermanent(err) {
		t.Fatalf("undecodable payload must be permanent, got %v", err)
	}
	if len(st.ids()) != 0 {
		t.Fatal("store was called for a malformed record")
	}
}

// Scenario: An event without a user id deletes nothing
func TestIdentityEmptyUserIDDeletesNothing(t *testing.T) {
	st := &fakeEraser{}
	h, _, _ := identityHandler(t, st)
	err := h(context.Background(), erasedRecord(t, ""))
	if err == nil || !isPermanent(err) {
		t.Fatalf("empty user_id must be a permanent error, got %v", err)
	}
	if len(st.ids()) != 0 {
		t.Fatalf("store erased %v for an empty user id", st.ids())
	}
}

// Scenario: Other identity events are ignored
func TestIdentityIgnoresOtherEventTypes(t *testing.T) {
	st := &fakeEraser{}
	h, _, _ := identityHandler(t, st)
	rec := consume.Record{Topic: "identity.events", Key: []byte(erasedUser), Value: envelopeOf(t, "evt-2", userStatusChangedType,
		&identityv1.UserStatusChanged{UserId: erasedUser, ChangedAt: timestamppb.New(t0)}, t0)}
	if err := h(context.Background(), rec); err != nil {
		t.Fatalf("other event types must be ignored without error, got %v", err)
	}
	if len(st.ids()) != 0 {
		t.Fatalf("store erased %v for UserStatusChanged", st.ids())
	}
}

// Scenario: A replayed event changes nothing (the handler applies each delivery without error)
func TestIdentityReplayedUserErasedReturnsNoError(t *testing.T) {
	st := &fakeEraser{}
	h, _, _ := identityHandler(t, st)
	for i := 0; i < 2; i++ {
		if err := h(context.Background(), erasedRecord(t, erasedUser)); err != nil {
			t.Fatalf("delivery %d: %v", i+1, err)
		}
	}
}

func mustEnvelope(typ string, payload []byte) *eventsv1.EventEnvelope {
	return &eventsv1.EventEnvelope{EventId: "evt-bad", Type: typ, OccurredAt: timestamppb.New(t0), Payload: payload}
}
