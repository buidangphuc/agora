package events_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	eventsv1 "github.com/buidangphuc/team-notification/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-notification/generated/platform/identity/v1"
	"github.com/buidangphuc/team-notification/internal/events"
)

const (
	idUserErasedType        = "platform.identity.v1.UserErased"
	idUserStatusChangedType = "platform.identity.v1.UserStatusChanged"
	idErasedUser            = "user-0a1b2c3d-raw-id"
	idErasureSecret         = "notification-test-secret"
)

var idT0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

type idFakeEraser struct {
	mu     sync.Mutex
	erased []string
	err    error
}

func (f *idFakeEraser) Erase(_ context.Context, userID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return 0, f.err
	}
	f.erased = append(f.erased, userID)
	return 7, nil
}

func (f *idFakeEraser) ids() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.erased...)
}

type idLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *idLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *idLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func identityHandler(t *testing.T, st *idFakeEraser) (consume.Handler, *idLogBuffer, *kafkax.Fingerprinter) {
	t.Helper()
	fp, err := kafkax.NewFingerprinter(idErasureSecret)
	if err != nil {
		t.Fatal(err)
	}
	logs := &idLogBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return events.IdentityEventHandler(st, fp, logger), logs, fp
}

func identityEnvelope(t *testing.T, eventID, typ string, msg proto.Message) []byte {
	t.Helper()
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId: eventID, Type: typ, OccurredAt: timestamppb.New(idT0), Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func erasedRecord(t *testing.T, userID string) consume.Record {
	t.Helper()
	return consume.Record{
		Topic: "identity.events", Key: []byte(userID),
		Value: identityEnvelope(t, "evt-erase-1", idUserErasedType, &identityv1.UserErased{
			UserId: userID, Roles: []string{"buyer", "seller"}, ErasedAt: timestamppb.New(idT0),
		}),
	}
}

// Scenario: The user's notification data is deleted (handler passes the user id to the store)
func TestIdentityUserErasedCallsStoreWithUserID(t *testing.T) {
	st := &idFakeEraser{}
	h, _, _ := identityHandler(t, st)
	if err := h(context.Background(), erasedRecord(t, idErasedUser)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := st.ids(); len(got) != 1 || got[0] != idErasedUser {
		t.Fatalf("store erased %v, want [%s]", got, idErasedUser)
	}
}

// Scenario: The erased id is not logged
func TestIdentityErasureLogsNoRawUserID(t *testing.T) {
	st := &idFakeEraser{}
	h, logs, fp := identityHandler(t, st)
	if err := h(context.Background(), erasedRecord(t, idErasedUser)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	out := logs.String()
	if strings.TrimSpace(out) == "" {
		t.Fatal("handler logged nothing; a count and a hashed id are required")
	}
	if strings.Contains(out, idErasedUser) {
		t.Fatalf("raw user id leaked in log: %s", out)
	}
	if !strings.Contains(out, fp.Of([]byte(idErasedUser))) {
		t.Fatalf("log lacks the hashed id: %s", out)
	}
}

// Scenario: A store failure rolls back and is retried (the handler returns a retryable error and logs no raw id)
func TestIdentityErasureStoreFailureLogsNoRawUserIDAndIsRetryable(t *testing.T) {
	st := &idFakeEraser{err: errors.New("database unavailable")}
	h, logs, _ := identityHandler(t, st)
	err := h(context.Background(), erasedRecord(t, idErasedUser))
	if err == nil || isPermanent(err) {
		t.Fatalf("store failure must be returned as retryable, got %v", err)
	}
	if strings.Contains(logs.String(), idErasedUser) {
		t.Fatalf("raw user id leaked in log: %s", logs.String())
	}
}

// Scenario: A malformed event goes to the DLQ
func TestIdentityMalformedEnvelopeIsPermanent(t *testing.T) {
	st := &idFakeEraser{}
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
	st := &idFakeEraser{}
	h, _, _ := identityHandler(t, st)
	env, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "evt-bad", Type: idUserErasedType, OccurredAt: timestamppb.New(idT0), Payload: []byte("\xff\xff\xff")})
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

// Scenario: A malformed event goes to the DLQ (empty user_id)
func TestIdentityEmptyUserIDIsPermanentAndErasesNothing(t *testing.T) {
	st := &idFakeEraser{}
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
	st := &idFakeEraser{}
	h, _, _ := identityHandler(t, st)
	rec := consume.Record{Topic: "identity.events", Key: []byte(idErasedUser), Value: identityEnvelope(t, "evt-2", idUserStatusChangedType,
		&identityv1.UserStatusChanged{UserId: idErasedUser, ChangedAt: timestamppb.New(idT0)})}
	if err := h(context.Background(), rec); err != nil {
		t.Fatalf("other event types must be ignored without error, got %v", err)
	}
	if len(st.ids()) != 0 {
		t.Fatalf("store erased %v for UserStatusChanged", st.ids())
	}
}

// Scenario: A replayed event changes nothing (each delivery returns no error)
func TestIdentityReplayedUserErasedReturnsNoError(t *testing.T) {
	st := &idFakeEraser{}
	h, _, _ := identityHandler(t, st)
	for i := 0; i < 2; i++ {
		if err := h(context.Background(), erasedRecord(t, idErasedUser)); err != nil {
			t.Fatalf("delivery %d: %v", i+1, err)
		}
	}
}
