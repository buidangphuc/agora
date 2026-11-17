package store_test

import (
	"context"
	"errors"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-identity/internal/store"
)

var deletedName = regexp.MustCompile(`^deleted_[0-9a-f]{16}$`)

func deletionMigration(t testing.TB, suffix string) string {
	t.Helper()
	names, err := fs.Glob(store.Migrations(), "0023_*."+suffix+".sql")
	if err != nil || len(names) != 1 {
		t.Fatalf("want exactly one 0023 %s migration, got %v (err %v)", suffix, names, err)
	}
	return migrationSQL(t, names[0])
}

func execAll(t testing.TB, pool *pgxpool.Pool, q string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func intOf(t testing.TB, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return n
}

func seedUser(t testing.TB, pool *pgxpool.Pool, id string, roles string) {
	t.Helper()
	execAll(t, pool, `INSERT INTO users (id, username, password_hash, roles) VALUES ($1, $1, 'hash', `+roles+`)`, id)
}

func seedFootprint(t testing.TB, pool *pgxpool.Pool, id string) {
	t.Helper()
	execAll(t, pool, `INSERT INTO user_addresses (id, user_id, recipient_name, phone, street, city) VALUES ($1, $2, 'n', 'p', 's', 'c')`, "addr-"+id, id)
	execAll(t, pool, `INSERT INTO sessions (id, user_id) VALUES ($1, $2)`, "sess-"+id, id)
	execAll(t, pool, `INSERT INTO login_history (id, user_id, ip) VALUES ($1, $2, '1.1.1.1')`, "lh-"+id, id)
	execAll(t, pool, `INSERT INTO handoff_codes (code_hash, user_id, source_sid, target_app, expires_at) VALUES (decode(md5($1), 'hex'), $1, 's', 'seller', now() + interval '1 hour')`, id)
	execAll(t, pool, `INSERT INTO password_reset_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, now() + interval '1 hour')`, "tok-"+id, id)
	execAll(t, pool, `INSERT INTO consents (user_id, purpose, granted, version, source) VALUES ($1, 'terms', true, 'v1', 'register')`, id)
	execAll(t, pool, `UPDATE users SET seller_terms_version = 'V1', seller_terms_accepted_at = now() WHERE id = $1`, id)
}

func markDue(t testing.TB, pool *pgxpool.Pool, id string, requested, erase time.Time) {
	t.Helper()
	execAll(t, pool, `UPDATE users SET deletion_requested_at = $2, erase_after = $3 WHERE id = $1`, id, requested, erase)
}

func userErasedEnqueue(failFor string) store.ErasedEnqueueFn {
	return func(e store.Erasure) (store.OutboxRow, error) {
		if e.UserID == failFor {
			return store.OutboxRow{}, errors.New("boom")
		}
		return store.OutboxRow{EventID: "evt-erased-" + e.UserID, AggregateID: e.UserID, EventType: "UserErased", Payload: []byte(strings.Join(e.Roles, ","))}, nil
	}
}

var base = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

// Scenario: Migration: deletion columns and deleted status (tasks 1.1)
func TestMigration0023AccountDeletionUpDown(t *testing.T) {
	pool := poolAt(t, 22)
	seedUser(t, pool, "u-m1", "'{}'")
	execAll(t, pool, deletionMigration(t, "up"))

	if n := intOf(t, pool, `SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name='users' AND column_name IN ('deletion_requested_at','erase_after')`); n != 2 {
		t.Fatalf("columns after up = %d, want 2", n)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM users WHERE deletion_requested_at IS NULL AND erase_after IS NULL`); n != 1 {
		t.Fatalf("existing rows must read as nothing pending")
	}
	execAll(t, pool, `UPDATE users SET status = 'deleted' WHERE id = 'u-m1'`)
	if _, err := pool.Exec(context.Background(), `UPDATE users SET status = 'bogus' WHERE id = 'u-m1'`); err == nil {
		t.Fatal("users_status_check must still reject unknown statuses")
	}

	execAll(t, pool, `UPDATE users SET status = 'active' WHERE id = 'u-m1'`)
	execAll(t, pool, deletionMigration(t, "down"))
	if n := intOf(t, pool, `SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name='users' AND column_name IN ('deletion_requested_at','erase_after')`); n != 0 {
		t.Fatalf("columns after down = %d, want 0", n)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE users SET status = 'deleted' WHERE id = 'u-m1'`); err == nil {
		t.Fatal("down must restore the old users_status_check")
	}
}

// Scenario: Sweeper: Due account is erased
func TestEraseDueErasesDueAccount(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	seedUser(t, pool, "u-due", "'{buyer,seller}'")
	seedFootprint(t, pool, "u-due")
	markDue(t, pool, "u-due", base.Add(-49*time.Hour), base.Add(-time.Hour))

	var gotRoles []string
	res, err := repo.EraseDue(ctx, base, 100, func(e store.Erasure) (store.OutboxRow, error) {
		gotRoles = e.Roles
		return userErasedEnqueue("")(e)
	})
	if err != nil {
		t.Fatalf("EraseDue: %v", err)
	}
	if res.Erased != 1 || res.Failed != 0 {
		t.Fatalf("result = %+v, want 1 erased", res)
	}
	if strings.Join(gotRoles, ",") != "buyer,seller" {
		t.Errorf("roles captured before clearing = %v", gotRoles)
	}

	var username, hash, status string
	var roles []string
	var req, after *time.Time
	if err := pool.QueryRow(ctx, `SELECT username, password_hash, status, roles, deletion_requested_at, erase_after FROM users WHERE id = 'u-due'`).Scan(&username, &hash, &status, &roles, &req, &after); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if !deletedName.MatchString(username) || hash != "" || status != "deleted" || len(roles) != 0 || req != nil || after != nil {
		t.Fatalf("erased row = %q %q %q %v %v %v", username, hash, status, roles, req, after)
	}
	for _, tbl := range []string{"user_addresses", "sessions", "login_history", "handoff_codes", "password_reset_tokens"} {
		if n := intOf(t, pool, `SELECT count(*) FROM `+tbl+` WHERE user_id = 'u-due'`); n != 0 {
			t.Errorf("%s rows left after erasure = %d", tbl, n)
		}
	}
}

// Scenario: Sweeper: UserErased is written in the same transaction
func TestEraseDueWritesUserErasedWithErasure(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	seedUser(t, pool, "u-ev", "'{buyer}'")
	markDue(t, pool, "u-ev", base.Add(-49*time.Hour), base.Add(-time.Hour))

	if _, err := repo.EraseDue(ctx, base, 100, userErasedEnqueue("")); err != nil {
		t.Fatalf("EraseDue: %v", err)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM identity_outbox_events WHERE event_type = 'UserErased' AND aggregate_id = 'u-ev' AND aggregate_type = 'User' AND status = 'pending'`); n != 1 {
		t.Fatalf("UserErased rows = %d, want 1", n)
	}

	seedUser(t, pool, "u-ev2", "'{buyer}'")
	markDue(t, pool, "u-ev2", base.Add(-49*time.Hour), base.Add(-time.Hour))
	if _, err := repo.EraseDue(ctx, base, 100, userErasedEnqueue("u-ev2")); err != nil {
		t.Fatalf("EraseDue: %v", err)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM identity_outbox_events WHERE aggregate_id = 'u-ev2'`); n != 0 {
		t.Fatalf("no event may exist when the erasure rolled back, got %d", n)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM users WHERE id = 'u-ev2' AND status <> 'deleted' AND password_hash = 'hash'`); n != 1 {
		t.Fatal("a failed enqueue must leave the account untouched")
	}
}

// Scenario: Sweeper: Consent and seller-terms proof survive erasure
func TestEraseDueKeepsConsentAndSellerTermsProof(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	seedUser(t, pool, "u-proof", "'{seller}'")
	seedFootprint(t, pool, "u-proof")
	markDue(t, pool, "u-proof", base.Add(-49*time.Hour), base.Add(-time.Hour))

	if _, err := repo.EraseDue(ctx, base, 100, userErasedEnqueue("")); err != nil {
		t.Fatalf("EraseDue: %v", err)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM consents WHERE user_id = 'u-proof'`); n != 1 {
		t.Fatalf("consent rows = %d, want 1", n)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM users WHERE id = 'u-proof' AND seller_terms_version = 'V1' AND seller_terms_accepted_at IS NOT NULL`); n != 1 {
		t.Fatal("seller-terms acceptance must survive erasure")
	}
}

// Scenario: Sweeper: Account not yet due is untouched
func TestEraseDueIgnoresNotYetDue(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	seedUser(t, pool, "u-future", "'{buyer}'")
	seedFootprint(t, pool, "u-future")
	markDue(t, pool, "u-future", base.Add(-time.Hour), base.Add(47*time.Hour))

	res, err := repo.EraseDue(ctx, base, 100, userErasedEnqueue(""))
	if err != nil {
		t.Fatalf("EraseDue: %v", err)
	}
	if res.Erased != 0 {
		t.Fatalf("erased = %d, want 0", res.Erased)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM users WHERE id = 'u-future' AND username = 'u-future' AND status = 'active' AND erase_after IS NOT NULL`); n != 1 {
		t.Fatal("not-yet-due account changed")
	}
	if n := intOf(t, pool, `SELECT count(*) FROM user_addresses WHERE user_id = 'u-future'`); n != 1 {
		t.Fatal("addresses of a not-yet-due account were removed")
	}
}

// Scenario: Sweeper: Cancelled account is untouched
func TestEraseDueIgnoresCancelledAccount(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	seedUser(t, pool, "u-cancel", "'{buyer}'")
	seedFootprint(t, pool, "u-cancel")
	markDue(t, pool, "u-cancel", base.Add(-49*time.Hour), base.Add(-time.Hour))
	if err := repo.CancelDeletion(ctx, "u-cancel"); err != nil {
		t.Fatalf("CancelDeletion: %v", err)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM users WHERE id = 'u-cancel' AND deletion_requested_at IS NULL AND erase_after IS NULL`); n != 1 {
		t.Fatal("cancel must clear both timestamps")
	}

	res, err := repo.EraseDue(ctx, base, 100, userErasedEnqueue(""))
	if err != nil {
		t.Fatalf("EraseDue: %v", err)
	}
	if res.Erased != 0 {
		t.Fatalf("erased = %d, want 0", res.Erased)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM users WHERE id = 'u-cancel' AND username = 'u-cancel' AND status = 'active'`); n != 1 {
		t.Fatal("cancelled account changed")
	}
}

// Scenario: Sweeper: Sweep is idempotent
func TestEraseDueIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	seedUser(t, pool, "u-idem", "'{buyer}'")
	markDue(t, pool, "u-idem", base.Add(-49*time.Hour), base.Add(-time.Hour))

	first, err := repo.EraseDue(ctx, base, 100, userErasedEnqueue(""))
	if err != nil || first.Erased != 1 {
		t.Fatalf("first sweep = %+v, %v", first, err)
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT username FROM users WHERE id = 'u-idem'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	second, err := repo.EraseDue(ctx, base.Add(time.Hour), 100, userErasedEnqueue(""))
	if err != nil || second.Erased != 0 {
		t.Fatalf("second sweep = %+v, %v", second, err)
	}
	var again string
	if err := pool.QueryRow(ctx, `SELECT username FROM users WHERE id = 'u-idem'`).Scan(&again); err != nil || again != name {
		t.Fatalf("username changed on second sweep: %q -> %q", name, again)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM identity_outbox_events WHERE event_type = 'UserErased' AND aggregate_id = 'u-idem'`); n != 1 {
		t.Fatalf("UserErased rows = %d, want exactly 1", n)
	}
}

// Scenario: Sweeper: One failing account does not stop the batch
func TestEraseDueOneFailureDoesNotStopBatch(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	for _, id := range []string{"u-b1", "u-b2", "u-b3"} {
		seedUser(t, pool, id, "'{buyer}'")
		seedFootprint(t, pool, id)
		markDue(t, pool, id, base.Add(-49*time.Hour), base.Add(-time.Hour))
	}

	res, err := repo.EraseDue(ctx, base, 100, userErasedEnqueue("u-b2"))
	if err != nil {
		t.Fatalf("EraseDue must not return an error for one failing account: %v", err)
	}
	if res.Erased != 2 || res.Failed != 1 {
		t.Fatalf("result = %+v, want 2 erased 1 failed", res)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM users WHERE status = 'deleted' AND id IN ('u-b1','u-b3')`); n != 2 {
		t.Fatal("healthy accounts must be erased")
	}
	if n := intOf(t, pool, `SELECT count(*) FROM users WHERE id = 'u-b2' AND status = 'active' AND username = 'u-b2' AND erase_after IS NOT NULL`); n != 1 {
		t.Fatal("failing account must stay due and intact")
	}
	if n := intOf(t, pool, `SELECT count(*) FROM user_addresses WHERE user_id = 'u-b2'`); n != 1 {
		t.Fatal("failing account lost its addresses")
	}
}

// Scenario: Sweeper: One failing account does not stop the batch (locked row skipped, FOR UPDATE SKIP LOCKED)
func TestEraseDueSkipsLockedRows(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	for _, id := range []string{"u-l1", "u-l2"} {
		seedUser(t, pool, id, "'{buyer}'")
		markDue(t, pool, id, base.Add(-49*time.Hour), base.Add(-time.Hour))
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = 'u-l1' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	var res store.ErasureResult
	var eerr error
	go func() {
		defer close(done)
		res, eerr = repo.EraseDue(ctx, base, 100, userErasedEnqueue(""))
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("EraseDue blocked on a locked row instead of skipping it")
	}
	if eerr != nil || res.Erased != 1 {
		t.Fatalf("result = %+v, %v; want the unlocked account erased", res, eerr)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM users WHERE id = 'u-l1' AND status = 'active'`); n != 1 {
		t.Fatal("locked account must be left for the next sweep")
	}
}

// Scenario: Deleted accounts: Former username is not reusable by lookup
func TestErasedUsernameIsNotFoundByLookup(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	seedUser(t, pool, "u-name", "'{buyer}'")
	markDue(t, pool, "u-name", base.Add(-49*time.Hour), base.Add(-time.Hour))
	if _, err := repo.EraseDue(ctx, base, 100, userErasedEnqueue("")); err != nil {
		t.Fatalf("EraseDue: %v", err)
	}
	if _, err := repo.GetByUsername(ctx, "u-name"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetByUsername(former) err = %v, want ErrNotFound", err)
	}
	u, err := repo.GetByID(ctx, "u-name")
	if err != nil || !deletedName.MatchString(u.Username) || u.Status != store.StatusDeleted {
		t.Fatalf("GetByID after erasure = %+v, %v", u, err)
	}
}

// Scenario: Scheduling revokes other sessions: Other sessions are revoked, current stays
func TestScheduleDeletionRevokesOtherSessionsKeepsCurrent(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	seedUser(t, pool, "u-sch", "'{buyer}'")
	seedUser(t, pool, "u-other", "'{buyer}'")
	for _, s := range [][2]string{{"s-cur", "u-sch"}, {"s-a", "u-sch"}, {"s-b", "u-sch"}, {"s-x", "u-other"}} {
		execAll(t, pool, `INSERT INTO sessions (id, user_id) VALUES ($1, $2)`, s[0], s[1])
	}

	got, err := repo.ScheduleDeletion(ctx, "u-sch", "s-cur", base, 48*time.Hour)
	if err != nil {
		t.Fatalf("ScheduleDeletion: %v", err)
	}
	if !got.Created || got.SessionsRevoked != 2 || !got.EraseAfter.Equal(base.Add(48*time.Hour)) || !got.RequestedAt.Equal(base) {
		t.Fatalf("schedule = %+v", got)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM sessions WHERE user_id = 'u-sch' AND revoked`); n != 2 {
		t.Fatalf("revoked sessions = %d, want 2", n)
	}
	if n := intOf(t, pool, `SELECT count(*) FROM sessions WHERE id = 's-cur' AND NOT revoked`); n != 1 {
		t.Fatal("current session must stay live")
	}
	if n := intOf(t, pool, `SELECT count(*) FROM sessions WHERE id = 's-x' AND NOT revoked`); n != 1 {
		t.Fatal("another user's session must be untouched")
	}
	if n := intOf(t, pool, `SELECT count(*) FROM users WHERE id = 'u-sch' AND deletion_requested_at IS NOT NULL AND erase_after IS NOT NULL AND status = 'active'`); n != 1 {
		t.Fatal("timestamps not stored (status must stay as is)")
	}
}

// Scenario: Scheduling revokes other sessions: Failed scheduling revokes nothing
func TestScheduleDeletionFailureRevokesNothing(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	seedUser(t, pool, "u-fail", "'{buyer}'")
	for _, id := range []string{"s-cur", "s-a"} {
		execAll(t, pool, `INSERT INTO sessions (id, user_id) VALUES ($1, 'u-fail')`, id)
	}
	execAll(t, pool, `CREATE FUNCTION deny_deletion_schedule() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
		IF NEW.erase_after IS NOT NULL THEN RAISE EXCEPTION 'scheduling denied'; END IF; RETURN NEW; END $$`)
	execAll(t, pool, `CREATE TRIGGER deny_deletion_schedule BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION deny_deletion_schedule()`)

	if _, err := repo.ScheduleDeletion(ctx, "u-fail", "s-cur", base, 48*time.Hour); err == nil {
		t.Fatal("expected scheduling to fail")
	}
	if n := intOf(t, pool, `SELECT count(*) FROM sessions WHERE user_id = 'u-fail' AND revoked`); n != 0 {
		t.Fatalf("sessions revoked despite failed scheduling = %d", n)
	}
	if _, err := repo.ScheduleDeletion(ctx, "u-ghost", "s-cur", base, 48*time.Hour); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown user err = %v, want ErrNotFound", err)
	}
}

// Scenario: Request: Repeated request keeps the first deadline
func TestScheduleDeletionKeepsFirstDeadline(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	seedUser(t, pool, "u-rep", "'{buyer}'")

	first, err := repo.ScheduleDeletion(ctx, "u-rep", "", base, 48*time.Hour)
	if err != nil || !first.Created {
		t.Fatalf("first = %+v, %v", first, err)
	}
	second, err := repo.ScheduleDeletion(ctx, "u-rep", "", base.Add(10*time.Hour), 48*time.Hour)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Created || !second.EraseAfter.Equal(first.EraseAfter) || !second.RequestedAt.Equal(first.RequestedAt) {
		t.Fatalf("second = %+v, want first deadline %v kept", second, first.EraseAfter)
	}
	var after time.Time
	if err := pool.QueryRow(ctx, `SELECT erase_after FROM users WHERE id = 'u-rep'`).Scan(&after); err != nil || !after.Equal(base.Add(48*time.Hour)) {
		t.Fatalf("stored erase_after = %v, %v", after, err)
	}
}

// Scenario: Cancel: Cancel with nothing pending is rejected
func TestCancelDeletionNothingPendingIsRejected(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	repo := store.NewPostgresUserRepository(pool)
	seedUser(t, pool, "u-nop", "'{buyer}'")
	if err := repo.CancelDeletion(ctx, "u-nop"); !errors.Is(err, store.ErrNoPendingDeletion) {
		t.Fatalf("cancel with nothing pending err = %v, want ErrNoPendingDeletion", err)
	}
	if _, err := repo.ScheduleDeletion(ctx, "u-nop", "", base, 48*time.Hour); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if err := repo.CancelDeletion(ctx, "u-nop"); err != nil {
		t.Fatalf("cancel pending: %v", err)
	}
	if err := repo.CancelDeletion(ctx, "u-nop"); !errors.Is(err, store.ErrNoPendingDeletion) {
		t.Fatalf("second cancel err = %v, want ErrNoPendingDeletion", err)
	}
}
