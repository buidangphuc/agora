package store_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-identity/internal/store"
)

const evRoleGranted = "t.UserRoleGranted"

var gsNow = time.Date(2026, 11, 4, 9, 30, 15, 0, time.UTC)

type grantEnv struct {
	pool  *pgxpool.Pool
	users *store.PostgresUserRepository
}

func newGrantEnv(t *testing.T) grantEnv {
	t.Helper()
	pool := latestPool(t)
	return grantEnv{pool: pool, users: store.NewPostgresUserRepository(pool)}
}

func (e grantEnv) seed(t *testing.T, id string, roles ...string) {
	t.Helper()
	if roles == nil {
		roles = []string{"buyer"}
	}
	if _, err := e.users.Create(context.Background(), store.User{ID: id, Username: id, PasswordHash: "x", Roles: roles}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func (e grantEnv) roles(t *testing.T, id string) []string {
	t.Helper()
	u, err := e.users.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return u.Roles
}

func (e grantEnv) terms(t *testing.T, id string) (*string, *time.Time) {
	t.Helper()
	var v *string
	var at *time.Time
	if err := e.pool.QueryRow(context.Background(), `SELECT seller_terms_version, seller_terms_accepted_at FROM users WHERE id=$1`, id).Scan(&v, &at); err != nil {
		t.Fatalf("terms %s: %v", id, err)
	}
	return v, at
}

func (e grantEnv) eventCount(t *testing.T, id string) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM identity_outbox_events WHERE aggregate_id=$1 AND event_type=$2`, id, evRoleGranted).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

func (e grantEnv) setStatus(t *testing.T, id, status string) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), `UPDATE users SET status=$2 WHERE id=$1`, id, status); err != nil {
		t.Fatalf("set status: %v", err)
	}
}

func grantBuilder(calls *[]store.SellerGrant, mu *sync.Mutex) store.SellerEnqueueFn {
	return func(g store.SellerGrant) (store.OutboxRow, error) {
		if mu != nil {
			mu.Lock()
			defer mu.Unlock()
		}
		if calls != nil {
			*calls = append(*calls, g)
		}
		return store.OutboxRow{
			EventID: uuid.NewString(), AggregateType: "User", AggregateID: g.UserID, EventType: evRoleGranted,
			Payload: []byte(g.Role),
		}, nil
	}
}

// Scenario: Seller onboarding: the first grant emits one event
func TestGrantSellerFirstGrantEmitsOneEvent(t *testing.T) {
	e := newGrantEnv(t)
	e.seed(t, "gs-1")
	var calls []store.SellerGrant

	res, err := e.users.GrantSeller(context.Background(), "gs-1", "V1", gsNow, grantBuilder(&calls, nil), nil)
	if err != nil {
		t.Fatalf("GrantSeller: %v", err)
	}
	if !res.Granted {
		t.Errorf("Granted = false, want true on first grant")
	}
	if !slices.Contains(res.User.Roles, "seller") || !slices.Contains(res.User.Roles, "buyer") {
		t.Errorf("result roles = %v, want buyer and seller", res.User.Roles)
	}
	if !slices.Contains(e.roles(t, "gs-1"), "seller") {
		t.Errorf("stored roles = %v, want seller", e.roles(t, "gs-1"))
	}
	v, at := e.terms(t, "gs-1")
	if v == nil || *v != "V1" || at == nil || at.Sub(gsNow).Abs() > time.Second {
		t.Errorf("terms = (%v, %v), want (V1, %v)", v, at, gsNow)
	}
	if got := e.eventCount(t, "gs-1"); got != 1 {
		t.Errorf("UserRoleGranted rows = %d, want 1", got)
	}
	if len(calls) != 1 || calls[0].UserID != "gs-1" || calls[0].Role != "seller" || !calls[0].GrantedAt.Equal(gsNow) {
		t.Errorf("enqueue calls = %+v, want one {gs-1 seller %v}", calls, gsNow)
	}
}

// Scenario: Seller onboarding: a second call is idempotent
func TestGrantSellerSecondCallIsIdempotent(t *testing.T) {
	e := newGrantEnv(t)
	e.seed(t, "gs-2")
	ctx := context.Background()
	var calls []store.SellerGrant
	if _, err := e.users.GrantSeller(ctx, "gs-2", "V1", gsNow, grantBuilder(&calls, nil), nil); err != nil {
		t.Fatalf("first: %v", err)
	}

	res, err := e.users.GrantSeller(ctx, "gs-2", "V1", gsNow.Add(time.Hour), grantBuilder(&calls, nil), nil)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if res.Granted {
		t.Errorf("Granted = true on second call, want false")
	}
	if !slices.Contains(res.User.Roles, "seller") {
		t.Errorf("result roles = %v, want seller", res.User.Roles)
	}
	v, at := e.terms(t, "gs-2")
	if v == nil || *v != "V1" || at == nil || at.Sub(gsNow).Abs() > time.Second {
		t.Errorf("terms changed by second call: (%v, %v), want (V1, %v)", v, at, gsNow)
	}
	if got := e.eventCount(t, "gs-2"); got != 1 {
		t.Errorf("UserRoleGranted rows = %d, want 1", got)
	}
	if len(calls) != 1 {
		t.Errorf("enqueue calls = %d, want 1", len(calls))
	}
	n := 0
	for _, r := range e.roles(t, "gs-2") {
		if r == "seller" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("seller role count = %d, want 1", n)
	}
}

// Scenario: Seller onboarding: a second call is idempotent
func TestGrantSellerExistingSellerWithoutTermsIsNotRewritten(t *testing.T) {
	e := newGrantEnv(t)
	e.seed(t, "gs-3", "buyer", "seller")

	res, err := e.users.GrantSeller(context.Background(), "gs-3", "V1", gsNow, grantBuilder(nil, nil), nil)
	if err != nil {
		t.Fatalf("GrantSeller: %v", err)
	}
	if res.Granted {
		t.Errorf("Granted = true for an existing seller, want false")
	}
	if got := e.eventCount(t, "gs-3"); got != 0 {
		t.Errorf("UserRoleGranted rows = %d, want 0 for an existing seller", got)
	}
	if v, at := e.terms(t, "gs-3"); v != nil || at != nil {
		t.Errorf("terms = (%v, %v), want untouched NULLs", v, at)
	}
}

// Scenario: Seller onboarding: a failed transaction leaves no event
func TestGrantSellerRollsBackOutbox(t *testing.T) {
	e := newGrantEnv(t)
	e.seed(t, "gs-4")
	boom := errors.New("enqueue failed")

	_, err := e.users.GrantSeller(context.Background(), "gs-4", "V1", gsNow, func(store.SellerGrant) (store.OutboxRow, error) {
		return store.OutboxRow{}, boom
	}, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap the enqueue error", err)
	}
	if slices.Contains(e.roles(t, "gs-4"), "seller") {
		t.Errorf("seller role persisted after a failed transaction")
	}
	if v, at := e.terms(t, "gs-4"); v != nil || at != nil {
		t.Errorf("terms persisted after rollback: (%v, %v)", v, at)
	}
	if got := e.eventCount(t, "gs-4"); got != 0 {
		t.Errorf("outbox rows = %d, want 0", got)
	}
}

// Scenario: Seller onboarding: a failed transaction leaves no event
func TestGrantSellerRollsBackRoleWhenOutboxInsertRefused(t *testing.T) {
	e := newGrantEnv(t)
	e.seed(t, "gs-5")

	_, err := e.users.GrantSeller(context.Background(), "gs-5", "V1", gsNow, func(g store.SellerGrant) (store.OutboxRow, error) {
		return store.OutboxRow{EventID: "", AggregateType: "User", AggregateID: g.UserID, EventType: evRoleGranted, Payload: []byte("seller")}, nil
	}, nil)
	if err == nil {
		t.Fatal("want an error when the outbox insert is refused")
	}
	if slices.Contains(e.roles(t, "gs-5"), "seller") {
		t.Errorf("seller role persisted although the outbox insert failed")
	}
	if got := e.eventCount(t, "gs-5"); got != 0 {
		t.Errorf("outbox rows = %d, want 0", got)
	}
}

// Scenario: Seller onboarding: concurrent calls emit one event
func TestGrantSellerConcurrentOneEvent(t *testing.T) {
	e := newGrantEnv(t)
	e.seed(t, "gs-6")
	var mu sync.Mutex
	var calls []store.SellerGrant

	const n = 2
	start := make(chan struct{})
	results := make([]store.SellerGrantResult, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = e.users.GrantSeller(context.Background(), "gs-6", "V1", gsNow, grantBuilder(&calls, &mu), nil)
		}()
	}
	close(start)
	wg.Wait()

	granted := 0
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("call %d: %v", i, errs[i])
		}
		if !slices.Contains(results[i].User.Roles, "seller") {
			t.Errorf("call %d roles = %v, want seller", i, results[i].User.Roles)
		}
		if results[i].Granted {
			granted++
		}
	}
	if granted != 1 {
		t.Errorf("calls reporting Granted = %d, want exactly 1", granted)
	}
	if got := e.eventCount(t, "gs-6"); got != 1 {
		t.Errorf("UserRoleGranted rows = %d, want 1", got)
	}
	if len(calls) != 1 {
		t.Errorf("enqueue calls = %d, want 1", len(calls))
	}
}

// Scenario: Seller onboarding: an admin restriction is not bypassed by a concurrent call
func TestGrantSellerRechecksStatusUnderLock(t *testing.T) {
	e := newGrantEnv(t)
	e.seed(t, "gs-7")
	ctx := context.Background()

	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id='gs-7' FOR UPDATE`).Scan(&locked); err != nil {
		t.Fatalf("lock row: %v", err)
	}

	type out struct {
		res store.SellerGrantResult
		err error
	}
	done := make(chan out, 1)
	go func() {
		res, err := e.users.GrantSeller(ctx, "gs-7", "V1", gsNow, grantBuilder(nil, nil), nil)
		done <- out{res, err}
	}()

	blocked := false
	var early *out
	for i := 0; i < 500 && !blocked && early == nil; i++ {
		select {
		case o := <-done:
			early = &o
		default:
		}
		if early != nil {
			break
		}
		var n int
		if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&n); err != nil {
			t.Fatalf("poll locks: %v", err)
		}
		blocked = n > 0
		if !blocked {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if early != nil {
		t.Fatalf("GrantSeller finished (err=%v) while another transaction held the row lock; it must lock the row before reading status", early.err)
	}
	if !blocked {
		t.Fatal("GrantSeller neither blocked on the row lock nor finished")
	}

	if _, err := tx.Exec(ctx, `UPDATE users SET status='restricted' WHERE id='gs-7'`); err != nil {
		t.Fatalf("restrict: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	o := <-done
	if !errors.Is(o.err, store.ErrNotActive) {
		t.Fatalf("err = %v, want store.ErrNotActive", o.err)
	}
	if slices.Contains(e.roles(t, "gs-7"), "seller") {
		t.Errorf("seller role granted to a restricted user")
	}
	if got := e.eventCount(t, "gs-7"); got != 0 {
		t.Errorf("outbox rows = %d, want 0", got)
	}
}

// Scenario: Seller onboarding: an admin restriction is not bypassed by a concurrent call
func TestGrantSellerRefusesNonActiveStatuses(t *testing.T) {
	for _, status := range []string{store.StatusRestricted, store.StatusSuspended, store.StatusBanned} {
		t.Run(status, func(t *testing.T) {
			e := newGrantEnv(t)
			e.seed(t, "gs-8")
			e.setStatus(t, "gs-8", status)

			_, err := e.users.GrantSeller(context.Background(), "gs-8", "V1", gsNow, grantBuilder(nil, nil), nil)
			if !errors.Is(err, store.ErrNotActive) {
				t.Fatalf("err = %v, want store.ErrNotActive", err)
			}
			if slices.Contains(e.roles(t, "gs-8"), "seller") {
				t.Errorf("seller role granted to a %s user", status)
			}
			if v, at := e.terms(t, "gs-8"); v != nil || at != nil {
				t.Errorf("terms written for a %s user: (%v, %v)", status, v, at)
			}
			if got := e.eventCount(t, "gs-8"); got != 0 {
				t.Errorf("outbox rows = %d, want 0", got)
			}
		})
	}
}

// Scenario: Seller onboarding: an admin restriction is not bypassed by a concurrent call
func TestGrantSellerExistingSellerNotActiveIsRefused(t *testing.T) {
	e := newGrantEnv(t)
	e.seed(t, "gs-9", "buyer", "seller")
	e.setStatus(t, "gs-9", store.StatusRestricted)

	_, err := e.users.GrantSeller(context.Background(), "gs-9", "V1", gsNow, grantBuilder(nil, nil), nil)
	if !errors.Is(err, store.ErrNotActive) {
		t.Fatalf("err = %v, want store.ErrNotActive before the idempotent path", err)
	}
}

// Scenario: Seller onboarding: the first grant emits one event
func TestGrantSellerUnknownUserIsNotFound(t *testing.T) {
	e := newGrantEnv(t)

	_, err := e.users.GrantSeller(context.Background(), "gs-missing", "V1", gsNow, grantBuilder(nil, nil), nil)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want store.ErrNotFound", err)
	}
	if got := e.eventCount(t, "gs-missing"); got != 0 {
		t.Errorf("outbox rows = %d, want 0", got)
	}
}

// Scenario: Seller onboarding: an audit failure after the role write rolls it back
func TestGrantSellerAuditFailureRollsBack(t *testing.T) {
	assertUntouched := func(t *testing.T, e grantEnv, id string) {
		t.Helper()
		if slices.Contains(e.roles(t, id), "seller") {
			t.Errorf("seller role persisted after the audit failure")
		}
		if v, at := e.terms(t, id); v != nil || at != nil {
			t.Errorf("terms persisted after rollback: (%v, %v)", v, at)
		}
		if got := e.eventCount(t, id); got != 0 {
			t.Errorf("outbox rows = %d, want 0", got)
		}
	}

	t.Run("audit write fails after the role update", func(t *testing.T) {
		e := newGrantEnv(t)
		e.seed(t, "gs-a1")
		boom := errors.New("audit unavailable")
		var ran bool
		_, err := e.users.GrantSeller(context.Background(), "gs-a1", "V1", gsNow, grantBuilder(nil, nil), func(context.Context) error {
			ran = true
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want it to wrap the audit error", err)
		}
		if !ran {
			t.Fatal("audit callback was not run")
		}
		assertUntouched(t, e, "gs-a1")
	})

	t.Run("audit write succeeds but the commit fails", func(t *testing.T) {
		e := newGrantEnv(t)
		e.seed(t, "gs-a2")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, err := e.users.GrantSeller(ctx, "gs-a2", "V1", gsNow, grantBuilder(nil, nil), func(context.Context) error {
			cancel()
			return nil
		})
		if err == nil {
			t.Fatal("want an error when the commit fails")
		}
		assertUntouched(t, e, "gs-a2")
	})

	t.Run("a repeat call by a seller does not run the audit callback", func(t *testing.T) {
		e := newGrantEnv(t)
		e.seed(t, "gs-a3", "buyer", "seller")
		calls := 0
		res, err := e.users.GrantSeller(context.Background(), "gs-a3", "V1", gsNow, grantBuilder(nil, nil), func(context.Context) error {
			calls++
			return errors.New("audit unavailable")
		})
		if err != nil {
			t.Fatalf("repeat GrantSeller: %v", err)
		}
		if res.Granted || calls != 0 {
			t.Fatalf("Granted = %v audit calls = %d, want false and 0", res.Granted, calls)
		}
	})
}
