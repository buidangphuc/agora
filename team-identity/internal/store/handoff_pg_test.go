package store_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-identity/internal/store"
)

const (
	handoffMigration = 20
	handoffCap       = 10
	handoffTTL       = 60 * time.Second
	handoffUser      = "u-ho1"
	handoffSeller    = "HANDOFF_APP_SELLER"
	handoffBuyer     = "HANDOFF_APP_BUYER"
)

var handoffNow = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

func handoffPG(t *testing.T) (*pgxpool.Pool, *store.PostgresHandoffRepository) {
	t.Helper()
	pool := poolAt(t, handoffMigration)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('u-ho1','u-ho1','x'), ('u-ho2','u-ho2','x')`,
		`INSERT INTO sessions (id, user_id) VALUES ('sid-1','u-ho1')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("seed (%s): %v", q, err)
		}
	}
	return pool, store.NewPostgresHandoffRepository(pool)
}

func hashOf(code string) []byte {
	h := sha256.Sum256([]byte(code))
	return h[:]
}

func newCode(code, user, target string, expires time.Time) store.HandoffCode {
	return store.HandoffCode{
		CodeHash:  hashOf(code),
		UserID:    user,
		SourceSID: "sid-1",
		TargetApp: target,
		ExpiresAt: expires,
	}
}

func seedHandoff(t *testing.T, pool *pgxpool.Pool, code, user string, expires time.Time, redeemedAt *time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `INSERT INTO handoff_codes
		(code_hash, user_id, source_sid, target_app, expires_at, redeemed_at, created_at)
		VALUES ($1,$2,'sid-1',$3,$4,$5,$6)`,
		hashOf(code), user, handoffSeller, expires, redeemedAt, expires.Add(-handoffTTL))
	if err != nil {
		t.Fatalf("seed handoff %s: %v", code, err)
	}
}

func handoffRowCount(t *testing.T, pool *pgxpool.Pool, where string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM handoff_codes WHERE `+where, args...).Scan(&n); err != nil {
		t.Fatalf("count handoff rows: %v", err)
	}
	return n
}

func TestHandoffMigrationHasIndexes(t *testing.T) {
	pool, _ := handoffPG(t)
	for _, col := range []string{"expires_at", "user_id"} {
		var n int
		err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_indexes
			WHERE tablename = 'handoff_codes' AND indexdef LIKE '%(' || $1 || '%'`, col).Scan(&n)
		if err != nil {
			t.Fatalf("query indexes: %v", err)
		}
		if n == 0 {
			t.Errorf("Old rows are purged, recent ones kept: no index leading with %s on handoff_codes", col)
		}
	}
}

func TestHandoffInsertStoresHashOnly(t *testing.T) {
	pool, repo := handoffPG(t)
	ctx := context.Background()
	code := "plaintext-code-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	exp := handoffNow.Add(handoffTTL)

	if err := repo.InsertHandoffCode(ctx, newCode(code, handoffUser, handoffSeller, exp), handoffCap, handoffNow, nil); err != nil {
		t.Fatalf("Authenticated user mints a code for the seller app: insert: %v", err)
	}

	var (
		hash      []byte
		user, sid string
		target    string
		gotExp    time.Time
		redeemed  *time.Time
		rowText   string
	)
	err := pool.QueryRow(ctx, `SELECT code_hash, user_id, source_sid, target_app, expires_at, redeemed_at, h::text
		FROM handoff_codes h`).Scan(&hash, &user, &sid, &target, &gotExp, &redeemed, &rowText)
	if err != nil {
		t.Fatalf("read row: %v", err)
	}
	if !bytes.Equal(hash, hashOf(code)) {
		t.Errorf("code_hash = %x, want sha256 of the code", hash)
	}
	if user != handoffUser || sid != "sid-1" || target != handoffSeller {
		t.Errorf("row = (%s,%s,%s), want (%s,sid-1,%s)", user, sid, target, handoffUser, handoffSeller)
	}
	if !gotExp.Equal(exp) {
		t.Errorf("expires_at = %v, want %v", gotExp, exp)
	}
	if redeemed != nil {
		t.Errorf("redeemed_at = %v, want NULL", redeemed)
	}
	if strings.Contains(rowText, code) {
		t.Errorf("Authenticated user mints a code for the seller app: row contains the plaintext code: %s", rowText)
	}
	if n := handoffRowCount(t, pool, `true`); n != 1 {
		t.Errorf("rows = %d, want 1", n)
	}
}

func TestHandoffRedeemIsAtomic(t *testing.T) {
	pool, repo := handoffPG(t)
	ctx := context.Background()
	code := "redeem-race-code"
	exp := handoffNow.Add(handoffTTL)
	if err := repo.InsertHandoffCode(ctx, newCode(code, handoffUser, handoffSeller, exp), handoffCap, handoffNow, nil); err != nil {
		t.Fatalf("insert: %v", err)
	}

	const racers = 20
	var wins, invalid, inTx atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := repo.RedeemHandoffCode(ctx, hashOf(code), handoffSeller, handoffNow, func(_ context.Context, c store.HandoffCode) error {
				inTx.Add(1)
				if c.UserID != handoffUser || c.SourceSID != "sid-1" {
					t.Errorf("callback row = %+v", c)
				}
				return nil
			})
			switch {
			case err == nil:
				wins.Add(1)
				if got.UserID != handoffUser || got.SourceSID != "sid-1" || got.TargetApp != handoffSeller {
					t.Errorf("redeemed row = %+v", got)
				}
			case errors.Is(err, store.ErrHandoffInvalid):
				invalid.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if wins.Load() != 1 || invalid.Load() != racers-1 {
		t.Errorf("Concurrent redeems create one session: wins=%d invalid=%d, want 1 and %d", wins.Load(), invalid.Load(), racers-1)
	}
	if inTx.Load() != 1 {
		t.Errorf("Concurrent redeems create one session: in-transaction callback ran %d times, want 1", inTx.Load())
	}
	if n := handoffRowCount(t, pool, `redeemed_at IS NOT NULL`); n != 1 {
		t.Errorf("redeemed rows = %d, want 1", n)
	}
	if _, err := repo.RedeemHandoffCode(ctx, hashOf(code), handoffSeller, handoffNow, nil); !errors.Is(err, store.ErrHandoffInvalid) {
		t.Errorf("Reused code is refused: err = %v, want ErrHandoffInvalid", err)
	}
}

func TestHandoffRedeemCallbackFailureKeepsCodeUsable(t *testing.T) {
	pool, repo := handoffPG(t)
	ctx := context.Background()
	code := "rollback-code"
	exp := handoffNow.Add(handoffTTL)
	if err := repo.InsertHandoffCode(ctx, newCode(code, handoffUser, handoffSeller, exp), handoffCap, handoffNow, nil); err != nil {
		t.Fatalf("insert: %v", err)
	}
	boom := errors.New("audit down")
	_, err := repo.RedeemHandoffCode(ctx, hashOf(code), handoffSeller, handoffNow, func(context.Context, store.HandoffCode) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("redeem with failing callback: err = %v, want the callback error", err)
	}
	if n := handoffRowCount(t, pool, `redeemed_at IS NOT NULL`); n != 0 {
		t.Errorf("redeemed rows after rollback = %d, want 0", n)
	}
	if _, err := repo.RedeemHandoffCode(ctx, hashOf(code), handoffSeller, handoffNow, nil); err != nil {
		t.Errorf("Audit down blocks the redeem and keeps the code usable: second redeem: %v", err)
	}
}

func TestHandoffRedeemAtExpiryBoundary(t *testing.T) {
	_, repo := handoffPG(t)
	ctx := context.Background()
	exp := handoffNow.Add(handoffTTL)
	for _, c := range []string{"boundary-a", "boundary-b"} {
		if err := repo.InsertHandoffCode(ctx, newCode(c, handoffUser, handoffSeller, exp), handoffCap, handoffNow, nil); err != nil {
			t.Fatalf("insert %s: %v", c, err)
		}
	}
	if _, err := repo.RedeemHandoffCode(ctx, hashOf("boundary-a"), handoffSeller, exp, nil); err != nil {
		t.Errorf("Redeem at the TTL boundary: redeem at expires_at: %v", err)
	}
	if _, err := repo.RedeemHandoffCode(ctx, hashOf("boundary-b"), handoffSeller, exp.Add(time.Millisecond), nil); !errors.Is(err, store.ErrHandoffInvalid) {
		t.Errorf("Redeem at the TTL boundary: redeem at expires_at+1ms: err = %v, want ErrHandoffInvalid", err)
	}
}

func TestHandoffRedeemRefusesWrongTargetAndUnknown(t *testing.T) {
	_, repo := handoffPG(t)
	ctx := context.Background()
	exp := handoffNow.Add(handoffTTL)
	if err := repo.InsertHandoffCode(ctx, newCode("target-code", handoffUser, handoffSeller, exp), handoffCap, handoffNow, nil); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := repo.RedeemHandoffCode(ctx, hashOf("target-code"), handoffBuyer, handoffNow, nil); !errors.Is(err, store.ErrHandoffInvalid) {
		t.Errorf("Wrong target is refused: err = %v, want ErrHandoffInvalid", err)
	}
	if _, err := repo.RedeemHandoffCode(ctx, hashOf("never-minted"), handoffSeller, handoffNow, nil); !errors.Is(err, store.ErrHandoffInvalid) {
		t.Errorf("unknown code: err = %v, want ErrHandoffInvalid", err)
	}
}

func TestHandoffCapAtomicUnderConcurrency(t *testing.T) {
	pool, repo := handoffPG(t)
	ctx := context.Background()
	exp := handoffNow.Add(handoffTTL)

	const callers = 20
	var ok, capped, inTx atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			err := repo.InsertHandoffCode(ctx, newCode(fmt.Sprintf("cap-%d", i), handoffUser, handoffSeller, exp), handoffCap, handoffNow,
				func(context.Context) error { inTx.Add(1); return nil })
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, store.ErrHandoffCapReached):
				capped.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if ok.Load() != handoffCap || capped.Load() != callers-handoffCap {
		t.Errorf("Concurrent mints cannot exceed the cap: ok=%d capped=%d, want %d and %d", ok.Load(), capped.Load(), handoffCap, callers-handoffCap)
	}
	if inTx.Load() != handoffCap {
		t.Errorf("Concurrent mints cannot exceed the cap: in-transaction callback ran %d times, want %d (a refusal runs none)", inTx.Load(), handoffCap)
	}
	if n, err := repo.CountLiveHandoffCodes(ctx, handoffUser, handoffNow); err != nil || n != handoffCap {
		t.Errorf("Concurrent mints cannot exceed the cap: live = %d (err %v), want %d", n, err, handoffCap)
	}
	if n := handoffRowCount(t, pool, `user_id = $1`, handoffUser); n != handoffCap {
		t.Errorf("rows = %d, want %d", n, handoffCap)
	}
}

func TestHandoffCapIsPerUser(t *testing.T) {
	_, repo := handoffPG(t)
	ctx := context.Background()
	exp := handoffNow.Add(handoffTTL)
	for i := 0; i < handoffCap; i++ {
		if err := repo.InsertHandoffCode(ctx, newCode(fmt.Sprintf("u1-%d", i), handoffUser, handoffSeller, exp), handoffCap, handoffNow, nil); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	if err := repo.InsertHandoffCode(ctx, newCode("u1-over", handoffUser, handoffSeller, exp), handoffCap, handoffNow, nil); !errors.Is(err, store.ErrHandoffCapReached) {
		t.Errorf("Eleventh live code is refused: err = %v, want ErrHandoffCapReached", err)
	}
	if err := repo.InsertHandoffCode(ctx, newCode("u2-0", "u-ho2", handoffSeller, exp), handoffCap, handoffNow, nil); err != nil {
		t.Errorf("another user at 0 live codes is refused: %v", err)
	}
}

func TestHandoffCapIgnoresExpiredAndRedeemed(t *testing.T) {
	pool, repo := handoffPG(t)
	ctx := context.Background()
	live := handoffNow.Add(handoffTTL)
	past := handoffNow.Add(-time.Minute)
	redeemed := handoffNow.Add(-30 * time.Second)
	for i := 0; i < 9; i++ {
		seedHandoff(t, pool, fmt.Sprintf("live-%d", i), handoffUser, live, nil)
	}
	for i := 0; i < 5; i++ {
		seedHandoff(t, pool, fmt.Sprintf("expired-%d", i), handoffUser, past, nil)
		seedHandoff(t, pool, fmt.Sprintf("redeemed-%d", i), handoffUser, live, &redeemed)
	}

	if err := repo.InsertHandoffCode(ctx, newCode("tenth", handoffUser, handoffSeller, live), handoffCap, handoffNow, nil); err != nil {
		t.Fatalf("Expired and redeemed codes do not count toward the cap: insert: %v", err)
	}
	if n, err := repo.CountLiveHandoffCodes(ctx, handoffUser, handoffNow); err != nil || n != handoffCap {
		t.Errorf("Expired and redeemed codes do not count toward the cap: live = %d (err %v), want %d", n, err, handoffCap)
	}
	if err := repo.InsertHandoffCode(ctx, newCode("eleventh", handoffUser, handoffSeller, live), handoffCap, handoffNow, nil); !errors.Is(err, store.ErrHandoffCapReached) {
		t.Errorf("Eleventh live code is refused: err = %v, want ErrHandoffCapReached", err)
	}
}

func TestHandoffCapRefusalAndCallbackFailureWriteNothing(t *testing.T) {
	pool, repo := handoffPG(t)
	ctx := context.Background()
	live := handoffNow.Add(handoffTTL)

	boom := errors.New("audit down")
	err := repo.InsertHandoffCode(ctx, newCode("rolled-back", handoffUser, handoffSeller, live), handoffCap, handoffNow,
		func(context.Context) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("insert with failing callback: err = %v, want the callback error", err)
	}
	if n := handoffRowCount(t, pool, `true`); n != 0 {
		t.Errorf("Audit down blocks the mint: rows after rollback = %d, want 0", n)
	}

	for i := 0; i < handoffCap; i++ {
		seedHandoff(t, pool, fmt.Sprintf("full-%d", i), handoffUser, live, nil)
	}
	var called atomic.Int32
	err = repo.InsertHandoffCode(ctx, newCode("refused", handoffUser, handoffSeller, live), handoffCap, handoffNow,
		func(context.Context) error { called.Add(1); return nil })
	if !errors.Is(err, store.ErrHandoffCapReached) {
		t.Fatalf("err = %v, want ErrHandoffCapReached", err)
	}
	if called.Load() != 0 {
		t.Errorf("A mint refused by the cap writes no audit entry: callback ran %d times", called.Load())
	}
}

func TestHandoffPeekDoesNotConsume(t *testing.T) {
	pool, repo := handoffPG(t)
	ctx := context.Background()
	exp := handoffNow.Add(handoffTTL)
	if err := repo.InsertHandoffCode(ctx, newCode("peek-code", handoffUser, handoffSeller, exp), handoffCap, handoffNow, nil); err != nil {
		t.Fatalf("insert: %v", err)
	}
	snapshot := func() string {
		var s string
		if err := pool.QueryRow(ctx, `SELECT xmin::text || '|' || h::text FROM handoff_codes h`).Scan(&s); err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		return s
	}
	before := snapshot()

	for i := 0; i < 3; i++ {
		got, err := repo.PeekHandoffCode(ctx, hashOf("peek-code"), handoffSeller, handoffNow)
		if err != nil {
			t.Fatalf("Peek returns the username and does not consume: peek %d: %v", i, err)
		}
		if got.UserID != handoffUser || !got.ExpiresAt.Equal(exp) || got.TargetApp != handoffSeller {
			t.Errorf("peek %d = %+v", i, got)
		}
	}
	if after := snapshot(); after != before {
		t.Errorf("Peek writes nothing: row changed from %q to %q", before, after)
	}
	if n := handoffRowCount(t, pool, `redeemed_at IS NULL`); n != 1 {
		t.Errorf("Peek writes nothing: unredeemed rows = %d, want 1", n)
	}
	if _, err := repo.RedeemHandoffCode(ctx, hashOf("peek-code"), handoffSeller, handoffNow, nil); err != nil {
		t.Errorf("Peek returns the username and does not consume: redeem after peeks: %v", err)
	}
}

func TestHandoffPeekRefusesInvalid(t *testing.T) {
	pool, repo := handoffPG(t)
	ctx := context.Background()
	exp := handoffNow.Add(handoffTTL)
	redeemed := handoffNow.Add(-time.Second)
	seedHandoff(t, pool, "ok", handoffUser, exp, nil)
	seedHandoff(t, pool, "used", handoffUser, exp, &redeemed)
	seedHandoff(t, pool, "old", handoffUser, handoffNow.Add(-time.Second), nil)

	cases := []struct {
		name, code, target string
	}{
		{"unknown", "nope", handoffSeller},
		{"used", "used", handoffSeller},
		{"expired", "old", handoffSeller},
		{"wrong target", "ok", handoffBuyer},
	}
	for _, c := range cases {
		if _, err := repo.PeekHandoffCode(ctx, hashOf(c.code), c.target, handoffNow); !errors.Is(err, store.ErrHandoffInvalid) {
			t.Errorf("Peek of an invalid code is indistinguishable: %s: err = %v, want ErrHandoffInvalid", c.name, err)
		}
	}
	if _, err := repo.PeekHandoffCode(ctx, hashOf("ok"), handoffSeller, exp); err != nil {
		t.Errorf("peek at expires_at: %v", err)
	}
}

func TestHandoffPurge(t *testing.T) {
	pool, repo := handoffPG(t)
	ctx := context.Background()
	redeemed := handoffNow.Add(-26 * time.Hour)
	seedHandoff(t, pool, "old-unredeemed", handoffUser, handoffNow.Add(-25*time.Hour), nil)
	seedHandoff(t, pool, "old-redeemed", handoffUser, handoffNow.Add(-25*time.Hour), &redeemed)
	seedHandoff(t, pool, "recent", handoffUser, handoffNow.Add(-23*time.Hour), nil)
	seedHandoff(t, pool, "live", handoffUser, handoffNow.Add(handoffTTL), nil)

	n, err := repo.PurgeHandoffCodes(ctx, handoffNow, 100)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 2 {
		t.Errorf("Old rows are purged, recent ones kept: deleted = %d, want 2", n)
	}
	for code, want := range map[string]int{"old-unredeemed": 0, "old-redeemed": 0, "recent": 1, "live": 1} {
		if got := handoffRowCount(t, pool, `code_hash = $1`, hashOf(code)); got != want {
			t.Errorf("Old rows are purged, recent ones kept: %s rows = %d, want %d", code, got, want)
		}
	}
}

func TestHandoffPurgeRespectsLimit(t *testing.T) {
	pool, repo := handoffPG(t)
	for i := 0; i < 5; i++ {
		seedHandoff(t, pool, fmt.Sprintf("old-%d", i), handoffUser, handoffNow.Add(-48*time.Hour), nil)
	}
	n, err := repo.PurgeHandoffCodes(context.Background(), handoffNow, 3)
	if err != nil || n != 3 {
		t.Errorf("Old rows are purged, recent ones kept: purge limit 3 deleted %d (err %v), want 3", n, err)
	}
	if left := handoffRowCount(t, pool, `true`); left != 2 {
		t.Errorf("rows left = %d, want 2", left)
	}
}
