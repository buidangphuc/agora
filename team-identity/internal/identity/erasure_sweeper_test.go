package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-identity/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
)

const userErasedType = "platform.identity.v1.UserErased"

type erasable struct {
	id, username string
	roles        []string
	eraseAfter   time.Time
	erased       bool
	erasedAt     time.Time
	row          store.OutboxRow
}

type fakeEraser struct {
	mu      sync.Mutex
	users   []*erasable
	failFor map[string]error
	limits  []int
}

func (f *fakeEraser) EraseDue(_ context.Context, now time.Time, limit int, enqueue store.ErasedEnqueueFn) (store.ErasureResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.limits = append(f.limits, limit)
	var res store.ErasureResult
	for _, u := range f.users {
		if u.erased || u.eraseAfter.IsZero() || u.eraseAfter.After(now) {
			continue
		}
		if res.Erased+res.Failed >= limit {
			break
		}
		if err := f.failFor[u.id]; err != nil {
			res.Failed++
			continue
		}
		row, err := enqueue(store.Erasure{UserID: u.id, Roles: u.roles, ErasedAt: now})
		if err != nil {
			res.Failed++
			continue
		}
		u.erased, u.erasedAt, u.row = true, now, row
		res.Erased++
	}
	return res, nil
}

func (f *fakeEraser) get(id string) *erasable {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.id == id {
			return u
		}
	}
	return nil
}

func quietSweeper(r identity.ErasureRepository) *identity.ErasureSweeper {
	return identity.NewErasureSweeper(r, identity.ErasureSweeperConfig{Interval: time.Hour, BatchSize: 100}, slog.New(slog.DiscardHandler))
}

var eraseT0 = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

// Scenario: identity.account-deletion-due-account-is-erased (sweeper path)
func TestEraserDueAccountIsErasedAndPublishesUserErased(t *testing.T) {
	repo := &fakeEraser{users: []*erasable{
		{id: "u-due", username: "dora", roles: []string{"buyer", "seller"}, eraseAfter: eraseT0.Add(-time.Hour)},
		{id: "u-later", username: "later", roles: []string{"buyer"}, eraseAfter: eraseT0.Add(time.Second)},
		{id: "u-none", username: "none", roles: []string{"buyer"}},
	}}
	res, err := quietSweeper(repo).SweepOnce(context.Background(), eraseT0)
	if err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}
	if res.Erased != 1 {
		t.Fatalf("erased = %d, want 1 (result %+v)", res.Erased, res)
	}
	if repo.get("u-later").erased || repo.get("u-none").erased {
		t.Fatal("not-due or cancelled account was erased")
	}
	row := repo.get("u-due").row
	if row.EventType != userErasedType || row.AggregateID != "u-due" || row.EventID == "" {
		t.Fatalf("outbox row = type %q aggregate %q event %q", row.EventType, row.AggregateID, row.EventID)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(row.Payload, &env); err != nil {
		t.Fatalf("payload is not an EventEnvelope: %v", err)
	}
	if env.GetType() != userErasedType || env.GetEventId() != row.EventID {
		t.Fatalf("envelope type %q id %q, row id %q", env.GetType(), env.GetEventId(), row.EventID)
	}
	var ev identityv1.UserErased
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		t.Fatalf("payload is not UserErased: %v", err)
	}
	if ev.GetUserId() != "u-due" || strings.Join(ev.GetRoles(), ",") != "buyer,seller" || !ev.GetErasedAt().AsTime().Equal(eraseT0) {
		t.Fatalf("UserErased = %v", &ev)
	}
}

// Scenario: identity.account-deletion-erasure-completes-within-72-h-of-the-request
func TestEraserErasureCompletesWithin49HoursOfRequestWithHourlySweeps(t *testing.T) {
	requested := eraseT0
	repo := &fakeEraser{users: []*erasable{{id: "u-72", username: "seventy", roles: []string{"buyer"}, eraseAfter: requested.Add(48 * time.Hour)}}}
	clock := requested
	sw := quietSweeper(repo).WithClock(func() time.Time { return clock })
	var erasedAt time.Time
	for h := 1; h <= 49; h++ {
		clock = requested.Add(time.Duration(h) * time.Hour)
		if err := sw.Tick(context.Background()); err != nil {
			t.Fatalf("Tick at +%dh: %v", h, err)
		}
		if h < 48 && repo.get("u-72").erased {
			t.Fatalf("erased early at +%dh", h)
		}
		if repo.get("u-72").erased && erasedAt.IsZero() {
			erasedAt = clock
		}
	}
	if erasedAt.IsZero() || erasedAt.After(requested.Add(49*time.Hour)) {
		t.Fatalf("erased at %v, want no later than request + 49h", erasedAt)
	}
	if repo.get("u-72").row.EventType != userErasedType {
		t.Fatal("UserErased not produced within the bound")
	}
}

// Scenario: identity.account-deletion-one-failing-account-does-not-stop-the-batch
func TestEraserOneFailingAccountDoesNotStopBatch(t *testing.T) {
	due := eraseT0.Add(-time.Minute)
	repo := &fakeEraser{
		users: []*erasable{
			{id: "u-a", username: "a", roles: []string{"buyer"}, eraseAfter: due},
			{id: "u-bad", username: "bad", roles: []string{"buyer"}, eraseAfter: due},
			{id: "u-c", username: "c", roles: []string{"buyer"}, eraseAfter: due},
		},
		failFor: map[string]error{"u-bad": errors.New("simulated db error")},
	}
	sw := quietSweeper(repo).WithClock(func() time.Time { return eraseT0 })
	if err := sw.Tick(context.Background()); err != nil {
		t.Fatalf("Tick must absorb a per-account failure, got %v", err)
	}
	if !repo.get("u-a").erased || !repo.get("u-c").erased {
		t.Fatal("other due accounts must be erased despite the failure")
	}
	if repo.get("u-bad").erased {
		t.Fatal("failing account must stay pending")
	}
	repo.failFor = nil
	if err := sw.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !repo.get("u-bad").erased {
		t.Fatal("failing account must be retried and erased on the next sweep")
	}
}

func erasureJSONLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// Scenario: identity.account-deletion-erasure-logs-counts-only
func TestErasureLogsCountsOnly(t *testing.T) {
	var buf bytes.Buffer
	repo := &fakeEraser{users: []*erasable{{id: "u-log-1f9e", username: "former-name-xyz", roles: []string{"buyer"}, eraseAfter: eraseT0.Add(-time.Minute)}}}
	sw := identity.NewErasureSweeper(repo, identity.ErasureSweeperConfig{Interval: time.Hour, BatchSize: 100}, erasureJSONLogger(&buf)).
		WithClock(func() time.Time { return eraseT0 })
	if err := sw.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, secret := range []string{"u-log-1f9e", "former-name-xyz", "password"} {
		if strings.Contains(out, secret) {
			t.Fatalf("log output leaks %q: %s", secret, out)
		}
	}
	found := false
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		for k, v := range rec {
			if n, ok := v.(float64); ok && n == 1 && strings.Contains(strings.ToLower(k), "erased") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no log attribute with key containing \"erased\" and value 1: %s", out)
	}
}

type errEraser struct{ err error }

func (e errEraser) EraseDue(context.Context, time.Time, int, store.ErasedEnqueueFn) (store.ErasureResult, error) {
	return store.ErasureResult{Failed: 1}, e.err
}

// Scenario: identity.account-deletion-a-failed-erasure-logs-no-personal-data
func TestFailedErasureLogsNoPersonalData(t *testing.T) {
	var buf bytes.Buffer
	dbErr := fmt.Errorf("update users set username=%q where id=%q: connection reset", "former-name-xyz", "u-fail-77aa")
	sw := identity.NewErasureSweeper(errEraser{err: dbErr}, identity.ErasureSweeperConfig{Interval: time.Hour, BatchSize: 100}, erasureJSONLogger(&buf))
	_ = sw.Tick(context.Background())
	out := buf.String()
	if strings.TrimSpace(out) == "" {
		t.Fatal("a failed erasure must be logged")
	}
	for _, secret := range []string{"u-fail-77aa", "former-name-xyz"} {
		if strings.Contains(out, secret) {
			t.Fatalf("failure log leaks %q: %s", secret, out)
		}
	}
	if !strings.Contains(out, `"level":"WARN"`) && !strings.Contains(out, `"level":"ERROR"`) {
		t.Fatalf("failure not logged at warn or error: %s", out)
	}
}
