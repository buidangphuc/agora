package store_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-verification/internal/store"
	"github.com/buidangphuc/team-verification/migrations"
)

var _ store.DocumentRepository = (*store.PostgresKycRepo)(nil)

var t0 = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

func grant(user, key string, size int64) *store.Grant {
	return &store.Grant{ObjectKey: key, UserID: user, ContentType: "image/jpeg", SizeBytes: size, IssuedAt: t0, ExpiresAt: t0.Add(15 * time.Minute)}
}

func seed(t *testing.T, repo *store.PostgresKycRepo, user string, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if err := repo.CreateGrant(context.Background(), grant(user, k, 100), 10, t0); err != nil {
			t.Fatalf("CreateGrant %s: %v", k, err)
		}
	}
}

func okVerify(_ []store.Grant) error { return nil }

func countSubmissions(t *testing.T, pool *pgxpool.Pool, user string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM kyc_submissions WHERE user_id=$1`, user).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMigration002CreatesAndDropsKycDocuments(t *testing.T) {
	const sc = "scenario Concurrent submits cannot share a key: migration 002 up and down"
	ctx := context.Background()
	pool := migratedPool(t)

	exists := func(table string) bool {
		var ok bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	hasCol := func(col string) bool {
		var ok bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name='kyc_submissions' AND column_name=$1)`, col).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !exists("kyc_upload_grants") || !exists("kyc_documents") || !hasCol("reason_code") || !hasCol("superseded_at") {
		t.Fatalf("%s: up did not create the objects", sc)
	}

	down, err := migrations.Files.ReadFile("002_kyc_documents.down.sql")
	if err != nil {
		t.Fatalf("%s: read down: %v", sc, err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("%s: down: %v", sc, err)
	}
	if exists("kyc_upload_grants") || exists("kyc_documents") || hasCol("reason_code") || hasCol("superseded_at") {
		t.Errorf("%s: down left objects behind", sc)
	}
	if !exists("kyc_submissions") {
		t.Errorf("%s: down dropped kyc_submissions", sc)
	}
	up, err := migrations.Files.ReadFile("002_kyc_documents.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(up)); err != nil {
		t.Errorf("%s: up after down: %v", sc, err)
	}
}

func TestSubmitWithDocumentsHappyPath(t *testing.T) {
	const sc = "scenario A key cannot be submitted twice: first submit stores documents and marks grants used"
	ctx := context.Background()
	pool := migratedPool(t)
	repo := store.NewPostgresKycRepo(pool)
	seed(t, repo, "u-1", "kyc/u-1/a", "kyc/u-1/b")

	var seen []store.Grant
	sub, err := repo.SubmitWithDocuments(ctx, &store.Submission{ID: "kyc_1", UserID: "u-1", DocType: "national_id", Status: store.StatusPending},
		[]string{"kyc/u-1/a", "kyc/u-1/b"}, t0.Add(time.Minute), func(gs []store.Grant) error { seen = gs; return nil })
	if err != nil {
		t.Fatalf("%s: %v", sc, err)
	}
	if sub.DocRef != "kyc/u-1/a" || sub.Status != store.StatusPending {
		t.Errorf("%s: sub = %+v, want DocRef equal to document 0", sc, sub)
	}
	if len(seen) != 2 || seen[0].ObjectKey != "kyc/u-1/a" || seen[1].ObjectKey != "kyc/u-1/b" || seen[0].SizeBytes != 100 {
		t.Errorf("%s: verify saw %+v, want the grants in key order", sc, seen)
	}
	docs, err := repo.ListDocuments(ctx, "kyc_1")
	if err != nil || len(docs) != 2 {
		t.Fatalf("%s: ListDocuments = %+v, %v", sc, docs, err)
	}
	if docs[0].Index != 0 || docs[0].ObjectKey != "kyc/u-1/a" || docs[1].Index != 1 || docs[1].ObjectKey != "kyc/u-1/b" ||
		docs[0].ContentType != "image/jpeg" || docs[0].SizeBytes != 100 || docs[0].DeletedAt != nil {
		t.Errorf("%s: docs = %+v", sc, docs)
	}
	g, err := repo.GetGrant(ctx, "kyc/u-1/a")
	if err != nil || g.UsedAt == nil {
		t.Fatalf("%s: grant = %+v, %v, want used", sc, g, err)
	}
}

func TestKeyCannotBeSubmittedTwice(t *testing.T) {
	const sc = "scenario A key cannot be submitted twice"
	ctx := context.Background()
	pool := migratedPool(t)
	repo := store.NewPostgresKycRepo(pool)
	seed(t, repo, "u-1", "kyc/u-1/a", "kyc/u-1/b")
	if _, err := repo.SubmitWithDocuments(ctx, &store.Submission{ID: "kyc_1", UserID: "u-1", DocType: "national_id", Status: store.StatusPending},
		[]string{"kyc/u-1/a"}, t0, okVerify); err != nil {
		t.Fatal(err)
	}

	t.Run(sc+": already used by an earlier submission", func(t *testing.T) {
		_, err := repo.SubmitWithDocuments(ctx, &store.Submission{ID: "kyc_2", UserID: "u-1", DocType: "national_id", Status: store.StatusPending},
			[]string{"kyc/u-1/a"}, t0, okVerify)
		if !errors.Is(err, store.ErrGrantInvalid) {
			t.Fatalf("err = %v, want ErrGrantInvalid", err)
		}
		if n := countSubmissions(t, pool, "u-1"); n != 1 {
			t.Errorf("submissions = %d, want 1", n)
		}
	})
	t.Run(sc+": same key listed twice", func(t *testing.T) {
		_, err := repo.SubmitWithDocuments(ctx, &store.Submission{ID: "kyc_3", UserID: "u-1", DocType: "national_id", Status: store.StatusPending},
			[]string{"kyc/u-1/b", "kyc/u-1/b"}, t0, okVerify)
		if !errors.Is(err, store.ErrGrantInvalid) {
			t.Fatalf("err = %v, want ErrGrantInvalid", err)
		}
		if n := countSubmissions(t, pool, "u-1"); n != 1 {
			t.Errorf("submissions = %d, want 1", n)
		}
		if g, _ := repo.GetGrant(ctx, "kyc/u-1/b"); g == nil || g.UsedAt != nil {
			t.Errorf("grant b = %+v, want still unused", g)
		}
	})
	t.Run(sc+": key issued to another user or unknown", func(t *testing.T) {
		seed(t, repo, "u-2", "kyc/u-2/a")
		for _, key := range []string{"kyc/u-2/a", "kyc/u-1/never-issued"} {
			_, err := repo.SubmitWithDocuments(ctx, &store.Submission{ID: "kyc_x", UserID: "u-1", DocType: "national_id", Status: store.StatusPending},
				[]string{key}, t0, okVerify)
			if !errors.Is(err, store.ErrGrantInvalid) {
				t.Errorf("%s: err = %v, want ErrGrantInvalid", key, err)
			}
		}
		if g, _ := repo.GetGrant(ctx, "kyc/u-2/a"); g == nil || g.UsedAt != nil {
			t.Errorf("foreign grant = %+v, want still unused", g)
		}
	})
}

func TestExpiredGrantAndVerifyFailureCreateNothing(t *testing.T) {
	const sc = "scenario A key cannot be submitted twice: failed submit creates nothing"
	ctx := context.Background()
	pool := migratedPool(t)
	repo := store.NewPostgresKycRepo(pool)
	seed(t, repo, "u-1", "kyc/u-1/a", "kyc/u-1/b")

	_, err := repo.SubmitWithDocuments(ctx, &store.Submission{ID: "kyc_1", UserID: "u-1", DocType: "national_id", Status: store.StatusPending},
		[]string{"kyc/u-1/a"}, t0.Add(16*time.Minute), okVerify)
	if !errors.Is(err, store.ErrGrantExpired) {
		t.Fatalf("%s: expired err = %v, want ErrGrantExpired", sc, err)
	}

	boom := errors.New("object mismatch")
	_, err = repo.SubmitWithDocuments(ctx, &store.Submission{ID: "kyc_2", UserID: "u-1", DocType: "national_id", Status: store.StatusPending},
		[]string{"kyc/u-1/a", "kyc/u-1/b"}, t0, func([]store.Grant) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("%s: verify err = %v, want the verify error", sc, err)
	}
	if n := countSubmissions(t, pool, "u-1"); n != 0 {
		t.Errorf("%s: submissions = %d, want 0", sc, n)
	}
	if docs, _ := repo.ListDocuments(ctx, "kyc_2"); len(docs) != 0 {
		t.Errorf("%s: docs = %+v", sc, docs)
	}
	for _, k := range []string{"kyc/u-1/a", "kyc/u-1/b"} {
		if g, _ := repo.GetGrant(ctx, k); g == nil || g.UsedAt != nil {
			t.Errorf("%s: grant %s = %+v, want unused", sc, k, g)
		}
	}
	if _, err := repo.SubmitWithDocuments(ctx, &store.Submission{ID: "kyc_3", UserID: "u-1", DocType: "national_id", Status: store.StatusPending},
		[]string{"kyc/u-1/a"}, t0, okVerify); err != nil {
		t.Errorf("%s: retry after failure: %v", sc, err)
	}
}

func TestConcurrentSubmitsCannotShareAKey(t *testing.T) {
	const sc = "scenario Concurrent submits cannot share a key"
	ctx := context.Background()
	pool := migratedPool(t)
	repo := store.NewPostgresKycRepo(pool)

	for round := range 10 {
		key := fmt.Sprintf("kyc/u-1/race-%d", round)
		seed(t, repo, "u-1", key)
		errs := make([]error, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, errs[i] = repo.SubmitWithDocuments(ctx,
					&store.Submission{ID: fmt.Sprintf("kyc_r%d_%d", round, i), UserID: "u-1", DocType: "national_id", Status: store.StatusPending},
					[]string{key}, t0, okVerify)
			}()
		}
		close(start)
		wg.Wait()

		ok, invalid := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, store.ErrGrantInvalid):
				invalid++
			default:
				t.Errorf("%s round %d: unexpected err %v", sc, round, err)
			}
		}
		if ok != 1 || invalid != 1 {
			t.Fatalf("%s round %d: ok=%d invalid=%d, want exactly 1 and 1", sc, round, ok, invalid)
		}
		var docs int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM kyc_documents WHERE object_key=$1`, key).Scan(&docs); err != nil || docs != 1 {
			t.Fatalf("%s round %d: documents with the key = %d, %v", sc, round, docs, err)
		}
	}
}

func TestNewSubmissionSupersedesEarlierOnes(t *testing.T) {
	const sc = "scenario Concurrent submits cannot share a key: earlier submissions are superseded in the same transaction"
	ctx := context.Background()
	pool := migratedPool(t)
	repo := store.NewPostgresKycRepo(pool)
	seed(t, repo, "u-1", "kyc/u-1/a", "kyc/u-1/b")
	seed(t, repo, "u-2", "kyc/u-2/a")

	for i, k := range []string{"kyc/u-1/a", "kyc/u-1/b"} {
		if _, err := repo.SubmitWithDocuments(ctx, &store.Submission{ID: fmt.Sprintf("kyc_%d", i+1), UserID: "u-1", DocType: "national_id", Status: store.StatusPending},
			[]string{k}, t0.Add(time.Duration(i)*time.Minute), okVerify); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.SubmitWithDocuments(ctx, &store.Submission{ID: "kyc_other", UserID: "u-2", DocType: "national_id", Status: store.StatusPending},
		[]string{"kyc/u-2/a"}, t0, okVerify); err != nil {
		t.Fatal(err)
	}

	first, err := repo.GetByID(ctx, "kyc_1")
	if err != nil || first.SupersededAt == nil {
		t.Fatalf("%s: first = %+v, %v, want SupersededAt set", sc, first, err)
	}
	second, _ := repo.GetByID(ctx, "kyc_2")
	if second.SupersededAt != nil {
		t.Errorf("%s: newest submission superseded: %+v", sc, second)
	}
	other, _ := repo.GetByID(ctx, "kyc_other")
	if other.SupersededAt != nil {
		t.Errorf("%s: another user's submission superseded", sc)
	}
}

func TestReasonCodePersisted(t *testing.T) {
	const sc = "scenario Concurrent submits cannot share a key: reason_code column round-trips"
	ctx := context.Background()
	pool := migratedPool(t)
	repo := store.NewPostgresKycRepo(pool)
	if _, err := repo.Create(ctx, pending("kyc_1", "u-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE kyc_submissions SET reason_code='blurry_document' WHERE id='kyc_1'`); err != nil {
		t.Fatalf("%s: %v", sc, err)
	}
	got, err := repo.GetByID(ctx, "kyc_1")
	if err != nil || got.ReasonCode != "blurry_document" {
		t.Fatalf("%s: got = %+v, %v", sc, got, err)
	}
}

func TestOpenGrantsAreCapped(t *testing.T) {
	const sc = "scenario Open grants are capped"
	ctx := context.Background()
	pool := migratedPool(t)
	repo := store.NewPostgresKycRepo(pool)

	for i := range 10 {
		if err := repo.CreateGrant(ctx, grant("u-1", fmt.Sprintf("kyc/u-1/g%d", i), 10), 10, t0); err != nil {
			t.Fatalf("%s: grant %d: %v", sc, i, err)
		}
	}
	err := repo.CreateGrant(ctx, grant("u-1", "kyc/u-1/g10", 10), 10, t0)
	if !errors.Is(err, store.ErrGrantCapReached) {
		t.Fatalf("%s: 11th err = %v, want ErrGrantCapReached", sc, err)
	}
	if _, err := repo.GetGrant(ctx, "kyc/u-1/g10"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("%s: 11th grant exists: %v", sc, err)
	}

	t.Run(sc+": other users are unaffected", func(t *testing.T) {
		if err := repo.CreateGrant(ctx, grant("u-2", "kyc/u-2/g0", 10), 10, t0); err != nil {
			t.Errorf("u-2: %v", err)
		}
	})
	t.Run(sc+": expired grants do not count", func(t *testing.T) {
		if err := repo.CreateGrant(ctx, grant("u-1", "kyc/u-1/late", 10), 10, t0.Add(16*time.Minute)); err != nil {
			t.Errorf("after expiry: %v", err)
		}
	})
	t.Run(sc+": used grants do not count", func(t *testing.T) {
		for i := range 10 {
			if err := repo.CreateGrant(ctx, grant("u-3", fmt.Sprintf("kyc/u-3/g%d", i), 10), 10, t0); err != nil {
				t.Fatalf("u-3 grant %d: %v", i, err)
			}
		}
		if err := repo.CreateGrant(ctx, grant("u-3", "kyc/u-3/over", 10), 10, t0); !errors.Is(err, store.ErrGrantCapReached) {
			t.Fatalf("u-3 at cap: err = %v, want ErrGrantCapReached", err)
		}
		if _, err := repo.SubmitWithDocuments(ctx, &store.Submission{ID: "kyc_3", UserID: "u-3", DocType: "national_id", Status: store.StatusPending},
			[]string{"kyc/u-3/g0"}, t0, okVerify); err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateGrant(ctx, grant("u-3", "kyc/u-3/after-use", 10), 10, t0); err != nil {
			t.Errorf("after use: %v", err)
		}
	})
}

func TestOpenGrantCapHoldsUnderConcurrency(t *testing.T) {
	const sc = "scenario Open grants are capped: concurrent requests"
	ctx := context.Background()
	pool := migratedPool(t)
	repo := store.NewPostgresKycRepo(pool)

	var wg sync.WaitGroup
	errs := make([]error, 16)
	start := make(chan struct{})
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = repo.CreateGrant(ctx, grant("u-1", fmt.Sprintf("kyc/u-1/c%d", i), 10), 10, t0)
		}()
	}
	close(start)
	wg.Wait()
	ok, capped := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, store.ErrGrantCapReached):
			capped++
		default:
			t.Errorf("%s: unexpected err %v", sc, err)
		}
	}
	if ok != 10 || capped != 6 {
		t.Errorf("%s: ok=%d capped=%d, want 10 and 6", sc, ok, capped)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM kyc_upload_grants WHERE user_id='u-1'`).Scan(&n); err != nil || n != 10 {
		t.Errorf("%s: rows = %d, %v, want 10", sc, n, err)
	}
}

func TestGetGrantUnknownKey(t *testing.T) {
	const sc = "scenario Open grants are capped: unknown grant lookup"
	repo := store.NewPostgresKycRepo(migratedPool(t))
	if _, err := repo.GetGrant(context.Background(), "kyc/u/none"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("%s: err = %v, want ErrNotFound", sc, err)
	}
}
