package audit_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-audit/internal/audit"
)

type fakeRepo struct {
	mu     sync.Mutex
	events []audit.Event
	seq    int
	last   audit.Filter
	err    error
}

func (r *fakeRepo) Append(_ context.Context, ev audit.Event) (audit.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return audit.Event{}, r.err
	}
	r.seq++
	if ev.ID == "" {
		ev.ID = "gen-" + string(rune('a'+r.seq))
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Date(2026, 6, 1, 0, 0, r.seq, 0, time.UTC)
	}
	r.events = append(r.events, ev)
	return ev, nil
}

func (r *fakeRepo) Query(_ context.Context, f audit.Filter) ([]audit.Event, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last = f
	if r.err != nil {
		return nil, 0, r.err
	}
	var m []audit.Event
	for _, e := range r.events {
		if f.ActorID != "" && e.ActorID != f.ActorID {
			continue
		}
		if f.TargetType != "" && e.TargetType != f.TargetType {
			continue
		}
		if f.TargetID != "" && e.TargetID != f.TargetID {
			continue
		}
		if f.Action != "" && e.Action != f.Action {
			continue
		}
		if !f.CreatedAfter.IsZero() && e.CreatedAt.Before(f.CreatedAfter) {
			continue
		}
		if !f.CreatedBefore.IsZero() && !e.CreatedAt.Before(f.CreatedBefore) {
			continue
		}
		m = append(m, e)
	}
	sort.SliceStable(m, func(i, j int) bool { return m[i].CreatedAt.After(m[j].CreatedAt) })
	total := int64(len(m))
	if f.Offset >= len(m) {
		return nil, total, nil
	}
	m = m[f.Offset:]
	if f.Limit > 0 && f.Limit < len(m) {
		m = m[:f.Limit]
	}
	return m, total, nil
}

func newService() (*audit.Service, *fakeRepo) {
	repo := &fakeRepo{}
	return audit.NewService(repo), repo
}

func mustWrite(t *testing.T, svc *audit.Service, actor, action, tType, tID string) {
	t.Helper()
	if _, err := svc.Write(context.Background(), actor, action, tType, tID, nil); err != nil {
		t.Fatalf("write %s/%s: %v", actor, action, err)
	}
}

func actions(evs []audit.Event) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Action
	}
	return out
}

func ids(evs []audit.Event) []string {
	out := []string{}
	for _, e := range evs {
		out = append(out, e.ID)
	}
	return out
}

func eqStr(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func hour(h int) time.Time { return time.Date(2026, 1, 1, h, 0, 0, 0, time.UTC) }

func seedAt(t *testing.T, hours ...int) *audit.Service {
	t.Helper()
	repo := &fakeRepo{}
	for _, h := range hours {
		if _, err := repo.Append(context.Background(), audit.Event{
			ID: "ev-" + hour(h).Format("15"), Action: "a.b", TargetType: "order", TargetID: "o-1", CreatedAt: hour(h),
		}); err != nil {
			t.Fatalf("seed %d: %v", h, err)
		}
	}
	return audit.NewService(repo)
}

func TestWriteQueryRoundtrip(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()

	ev, err := svc.Write(ctx, "seller_1", "listing.update", "listing", "lst_9", map[string]string{"price": "100"})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if ev.ID == "" || ev.CreatedAt.IsZero() {
		t.Fatalf("expected server-assigned id and created_at, got %+v", ev)
	}

	events, total, err := svc.Query(ctx, audit.Filter{Limit: 20})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || len(events) != 1 {
		t.Fatalf("expected 1 event, got total=%d len=%d", total, len(events))
	}
	got := events[0]
	if got.ActorID != "seller_1" || got.Action != "listing.update" || got.TargetType != "listing" || got.TargetID != "lst_9" {
		t.Fatalf("roundtrip field mismatch: %+v", got)
	}
	if got.Metadata["price"] != "100" {
		t.Fatalf("metadata not roundtripped: %v", got.Metadata)
	}
}

func TestQueryFilterByActorAndTarget(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	mustWrite(t, svc, "seller_1", "listing.update", "listing", "lst_1")
	mustWrite(t, svc, "seller_2", "payout.request", "payout", "po_1")
	mustWrite(t, svc, "seller_1", "order.ship", "order", "ord_1")

	byActor, total, err := svc.Query(ctx, audit.Filter{ActorID: "seller_1", Limit: 20})
	if err != nil {
		t.Fatalf("query by actor: %v", err)
	}
	if total != 2 || len(byActor) != 2 {
		t.Fatalf("expected 2 events for seller_1, got total=%d len=%d", total, len(byActor))
	}
	for _, ev := range byActor {
		if ev.ActorID != "seller_1" {
			t.Fatalf("actor filter leaked: %v", ev.ActorID)
		}
	}

	byTarget, total, err := svc.Query(ctx, audit.Filter{TargetType: "payout", Limit: 20})
	if err != nil {
		t.Fatalf("query by target: %v", err)
	}
	if total != 1 || len(byTarget) != 1 || byTarget[0].TargetType != "payout" {
		t.Fatalf("expected 1 payout event, got total=%d len=%d", total, len(byTarget))
	}
}

func TestQueryPaginationNewestFirst(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	mustWrite(t, svc, "seller_1", "a1", "listing", "lst_1")
	mustWrite(t, svc, "seller_1", "a2", "listing", "lst_2")
	mustWrite(t, svc, "seller_1", "a3", "listing", "lst_3")

	page1, total, err := svc.Query(ctx, audit.Filter{ActorID: "seller_1", Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	if !eqStr(actions(page1), []string{"a3", "a2"}) {
		t.Fatalf("expected newest-first [a3 a2], got %v", actions(page1))
	}

	page2, _, err := svc.Query(ctx, audit.Filter{ActorID: "seller_1", Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if !eqStr(actions(page2), []string{"a1"}) {
		t.Fatalf("expected [a1] on page2, got %v", actions(page2))
	}
}

func TestWriteValidation(t *testing.T) {
	svc, repo := newService()
	ctx := context.Background()

	if _, err := svc.Write(ctx, "seller_1", "", "listing", "lst_1", nil); !errors.Is(err, audit.ErrEmptyAction) {
		t.Fatalf("expected ErrEmptyAction, got %v", err)
	}
	if len(repo.events) != 0 {
		t.Fatalf("rejected write must not be stored")
	}
	if _, err := svc.Write(ctx, "", "system.startup", "", "", nil); err != nil {
		t.Fatalf("anonymous write should succeed, got %v", err)
	}
}

func TestQueryPageSizeAndOffsetNormalization(t *testing.T) {
	cases := []struct {
		name           string
		limit, offset  int
		wantLimit, off int
	}{
		{"zero uses default", 0, 0, 20, 0},
		{"negative uses default", -5, 0, 20, 0},
		{"within range kept", 50, 0, 50, 0},
		{"max kept", 100, 0, 100, 0},
		{"above max clamped", 101, 0, 100, 0},
		{"negative offset floored", 10, -3, 10, 0},
		{"positive offset kept", 10, 7, 10, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, repo := newService()
			if _, _, err := svc.Query(context.Background(), audit.Filter{Limit: c.limit, Offset: c.offset}); err != nil {
				t.Fatalf("query: %v", err)
			}
			if repo.last.Limit != c.wantLimit || repo.last.Offset != c.off {
				t.Fatalf("repo saw limit=%d offset=%d, want %d/%d", repo.last.Limit, repo.last.Offset, c.wantLimit, c.off)
			}
		})
	}
}

func TestQueryRepositoryErrorIsReturned(t *testing.T) {
	svc, repo := newService()
	boom := errors.New("boom")
	repo.err = boom
	if _, _, err := svc.Query(context.Background(), audit.Filter{}); !errors.Is(err, boom) {
		t.Fatalf("query err = %v, want boom", err)
	}
	if _, err := svc.Write(context.Background(), "a", "x.y", "", "", nil); !errors.Is(err, boom) {
		t.Fatalf("write err = %v, want boom", err)
	}
}

// Scenario: Time range includes the lower bound and excludes the upper bound.
func TestQueryTimeRangeBoundaries(t *testing.T) {
	svc := seedAt(t, 10, 11, 12)
	evs, total, err := svc.Query(context.Background(), audit.Filter{
		TargetType: "order", TargetID: "o-1", CreatedAfter: hour(10), CreatedBefore: hour(12), Limit: 20,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if want := []string{"ev-11", "ev-10"}; !eqStr(ids(evs), want) || total != 2 {
		t.Fatalf("got %v total=%d, want %v total=2", ids(evs), total, want)
	}
}

// Scenario: One-sided time bounds.
func TestQueryOneSidedTimeBounds(t *testing.T) {
	svc := seedAt(t, 10, 12)
	ctx := context.Background()

	evs, total, err := svc.Query(ctx, audit.Filter{CreatedAfter: hour(11), Limit: 20})
	if err != nil {
		t.Fatalf("after only: %v", err)
	}
	if want := []string{"ev-12"}; !eqStr(ids(evs), want) || total != 1 {
		t.Fatalf("created_after only: got %v total=%d, want %v", ids(evs), total, want)
	}

	evs, total, err = svc.Query(ctx, audit.Filter{CreatedBefore: hour(11), Limit: 20})
	if err != nil {
		t.Fatalf("before only: %v", err)
	}
	if want := []string{"ev-10"}; !eqStr(ids(evs), want) || total != 1 {
		t.Fatalf("created_before only: got %v total=%d, want %v", ids(evs), total, want)
	}
}

// Scenario: Equal bounds match nothing.
func TestQueryEqualTimeBoundsIsEmpty(t *testing.T) {
	svc := seedAt(t, 10, 11, 12)
	evs, total, err := svc.Query(context.Background(), audit.Filter{
		CreatedAfter: hour(11), CreatedBefore: hour(11), Limit: 20,
	})
	if err != nil {
		t.Fatalf("equal bounds must not error, got %v", err)
	}
	if len(evs) != 0 || total != 0 {
		t.Fatalf("got %v total=%d, want empty", ids(evs), total)
	}
}

func TestQueryInvertedTimeRangeIsRejected(t *testing.T) {
	svc := seedAt(t, 10, 11, 12)
	evs, total, err := svc.Query(context.Background(), audit.Filter{
		CreatedAfter: hour(12), CreatedBefore: hour(10), Limit: 20,
	})
	if !errors.Is(err, audit.ErrInvalidTimeRange) {
		t.Fatalf("err = %v, want ErrInvalidTimeRange", err)
	}
	if len(evs) != 0 || total != 0 {
		t.Fatalf("no result expected on error, got %v total=%d", ids(evs), total)
	}
}
