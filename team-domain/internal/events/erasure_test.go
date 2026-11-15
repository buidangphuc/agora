package events_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-domain/generated/platform/identity/v1"
	"github.com/buidangphuc/team-domain/internal/events"
	"github.com/buidangphuc/team-domain/internal/store"
)

const userErasedType = "platform.identity.v1.UserErased"

type fakeErasureStore struct {
	mu       sync.Mutex
	listings map[string]string
	deleted  []string
	users    []string
	events   []string
	failErr  error
	enqueued int
}

func newFakeErasureStore() *fakeErasureStore {
	return &fakeErasureStore{listings: map[string]string{}}
}

func (f *fakeErasureStore) EraseSellerListings(_ context.Context, userID, eventID string, enqueue store.EnqueueFn) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users = append(f.users, userID)
	f.events = append(f.events, eventID)
	if f.failErr != nil {
		return 0, f.failErr
	}
	n := 0
	for id, seller := range f.listings {
		if seller != userID {
			continue
		}
		if _, err := enqueue(store.Listing{ID: id, SellerID: seller}); err != nil {
			return 0, err
		}
		f.enqueued++
		f.deleted = append(f.deleted, id)
		delete(f.listings, id)
		n++
	}
	return n, nil
}

func (f *fakeErasureStore) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.users)
}

func erasedEvent(t *testing.T, eventID, user string, roles []string, withErasedAt bool) []byte {
	t.Helper()
	m := &identityv1.UserErased{UserId: user, Roles: roles}
	if withErasedAt {
		m.ErasedAt = timestamppb.New(t1)
	}
	return envBytes(t, eventID, userErasedType, m)
}

func isPermanent(err error) bool {
	pt := reflect.TypeOf(consume.Permanent(errors.New("probe")))
	for e := err; e != nil; e = errors.Unwrap(e) {
		if reflect.TypeOf(e) == pt {
			return true
		}
	}
	return false
}

func assertPermanent(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Handle returned nil, want a permanent error")
	}
	if !isPermanent(err) {
		t.Fatalf("error %v is not permanent", err)
	}
}

// Scenario: Malformed event goes to the DLQ (undecodable envelope)
func TestErasureMalformedEnvelopeIsPermanent(t *testing.T) {
	st := newFakeErasureStore()
	st.listings["l1"] = "u1"
	err := events.NewErasureConsumer(st, discardLogger()).Handle(t.Context(), consume.Record{Value: []byte{0xff, 0xff, 0xff, 0xff}})
	assertPermanent(t, err)
	if st.callCount() != 0 || len(st.listings) != 1 {
		t.Errorf("store touched: calls=%d listings=%d", st.callCount(), len(st.listings))
	}
}

// Scenario: Malformed event goes to the DLQ (undecodable payload)
func TestErasureGarbledPayloadIsPermanent(t *testing.T) {
	st := newFakeErasureStore()
	b, _ := proto.Marshal(&eventsv1.EventEnvelope{EventId: "e1", Type: userErasedType, Payload: []byte{0xff, 0xff, 0xff}})
	err := events.NewErasureConsumer(st, discardLogger()).Handle(t.Context(), consume.Record{Value: b})
	assertPermanent(t, err)
	if st.callCount() != 0 {
		t.Errorf("store called %d times", st.callCount())
	}
}

// Scenario: Malformed event goes to the DLQ (empty user_id)
func TestErasureEmptyUserIDIsPermanent(t *testing.T) {
	st := newFakeErasureStore()
	err := events.NewErasureConsumer(st, discardLogger()).Handle(t.Context(), consume.Record{Value: erasedEvent(t, "e1", "", nil, true)})
	assertPermanent(t, err)
	if st.callCount() != 0 {
		t.Errorf("store called %d times", st.callCount())
	}
}

// Scenario: Malformed event goes to the DLQ (missing erased_at)
func TestErasureMissingErasedAtIsPermanent(t *testing.T) {
	st := newFakeErasureStore()
	st.listings["l1"] = "u1"
	err := events.NewErasureConsumer(st, discardLogger()).Handle(t.Context(), consume.Record{Value: erasedEvent(t, "e1", "u1", nil, false)})
	assertPermanent(t, err)
	if st.callCount() != 0 || len(st.listings) != 1 {
		t.Errorf("store touched: calls=%d listings=%d", st.callCount(), len(st.listings))
	}
}

// Scenario: Other event types are ignored
func TestErasureIgnoresOtherEventTypes(t *testing.T) {
	st := newFakeErasureStore()
	st.listings["l1"] = "u1"
	h := events.NewErasureConsumer(st, discardLogger())
	banned := statusEvent(t, "e1", "u1", identityv1.UserStatus_USER_STATUS_BANNED, identityv1.UserStatus_USER_STATUS_ACTIVE, t1)
	registered := envBytes(t, "e2", "platform.identity.v1.UserRegistered", &identityv1.UserRegistered{UserId: "u1"})
	for name, v := range map[string][]byte{"banned": banned, "registered": registered} {
		if err := h.Handle(t.Context(), consume.Record{Value: v}); err != nil {
			t.Errorf("%s: Handle = %v, want nil", name, err)
		}
	}
	if st.callCount() != 0 || len(st.listings) != 1 {
		t.Errorf("store touched: calls=%d listings=%d", st.callCount(), len(st.listings))
	}
}

// Scenario: Roles in the event do not widen the delete
func TestErasureRolesDoNotGateOrWidenDelete(t *testing.T) {
	for name, roles := range map[string][]string{"empty": nil, "buyer only": {"buyer"}, "seller": {"seller"}, "admin": {"admin", "seller"}} {
		t.Run(name, func(t *testing.T) {
			st := newFakeErasureStore()
			st.listings["l1"] = "u1"
			st.listings["l2"] = "u2"
			err := events.NewErasureConsumer(st, discardLogger()).Handle(t.Context(), consume.Record{Value: erasedEvent(t, "e1", "u1", roles, true)})
			if err != nil {
				t.Fatalf("Handle = %v", err)
			}
			if len(st.users) != 1 || st.users[0] != "u1" || st.events[0] != "e1" {
				t.Errorf("store calls users=%v events=%v, want exactly u1/e1", st.users, st.events)
			}
			if _, ok := st.listings["l1"]; ok {
				t.Error("u1 listing not deleted")
			}
			if _, ok := st.listings["l2"]; !ok {
				t.Error("u2 listing was deleted")
			}
			if st.enqueued != 1 {
				t.Errorf("enqueued = %d, want 1", st.enqueued)
			}
		})
	}
}

// Scenario: Roles in the event do not widen the delete (user with no listings succeeds)
func TestErasureUserWithoutListingsSucceeds(t *testing.T) {
	st := newFakeErasureStore()
	err := events.NewErasureConsumer(st, discardLogger()).Handle(t.Context(), consume.Record{Value: erasedEvent(t, "e1", "ghost", []string{"buyer"}, true)})
	if err != nil {
		t.Fatalf("Handle = %v", err)
	}
	if st.callCount() != 1 || len(st.deleted) != 0 {
		t.Errorf("calls=%d deleted=%v", st.callCount(), st.deleted)
	}
}

// Scenario: The erasure consumer is isolated and fails safe (store error is not permanent)
func TestErasureStoreErrorIsNotPermanent(t *testing.T) {
	st := newFakeErasureStore()
	st.failErr = errors.New("db down")
	err := events.NewErasureConsumer(st, discardLogger()).Handle(t.Context(), consume.Record{Value: erasedEvent(t, "e1", "u1", nil, true)})
	if err == nil {
		t.Fatal("Handle = nil, want the store error")
	}
	if isPermanent(err) {
		t.Errorf("store error %v must be retried, not permanent", err)
	}
	if !errors.Is(err, st.failErr) {
		t.Errorf("error %v does not wrap the store error", err)
	}
}

// Scenario: Logs carry no raw identifiers
func TestErasureLogsNoRawIdentifiers(t *testing.T) {
	st := newFakeErasureStore()
	st.listings["listing-aaa-111"] = "user-123"
	st.listings["listing-bbb-222"] = "user-123"
	logger, buf := captureLogger()
	h := events.NewErasureConsumer(st, logger)
	rec := consume.Record{Key: []byte("user-123"), Value: erasedEvent(t, "e1", "user-123", []string{"seller"}, true)}
	if err := h.Handle(t.Context(), rec); err != nil {
		t.Fatalf("Handle = %v", err)
	}
	if len(st.deleted) != 2 {
		t.Fatalf("deleted = %v", st.deleted)
	}
	if strings.TrimSpace(buf.String()) == "" {
		t.Fatal("applied erasure logged nothing; want a line with a hashed key and counts")
	}
	for _, raw := range []string{"user-123", "listing-aaa-111", "listing-bbb-222"} {
		if strings.Contains(buf.String(), raw) {
			t.Errorf("raw identifier %q leaked in logs: %s", raw, buf.String())
		}
	}
}

// Scenario: Logs carry no raw identifiers (failure path)
func TestErasureFailureLogsNoRawIdentifiers(t *testing.T) {
	st := newFakeErasureStore()
	st.failErr = errors.New("delete for user-123 failed")
	logger, buf := captureLogger()
	h := events.NewErasureConsumer(st, logger)
	_ = h.Handle(t.Context(), consume.Record{Value: erasedEvent(t, "e1", "user-123", nil, true)})
	_ = h.Handle(t.Context(), consume.Record{Value: erasedEvent(t, "e2", "", nil, true)})
	if strings.Contains(buf.String(), "user-123") {
		t.Errorf("raw user id leaked in logs: %s", buf.String())
	}
}
