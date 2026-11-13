package store_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-engagement/internal/store"
)

const (
	erasedName = "Người dùng đã xóa"
	victim     = "user-erase-victim"
	other      = "user-erase-other"
)

var eraseTables = []string{
	"favorites", "listing_stats", "reviews", "review_helpful_votes", "product_questions",
	"product_answers", "collections", "collection_items", "view_history", "follows",
	"loyalty_accounts", "checkins", "reports", "seller_listings",
}

func eraser(t *testing.T, pool *pgxpool.Pool) *store.PostgresRepository {
	t.Helper()
	return store.NewPostgresRepository(pool, testOutbox(t, pool))
}

func snapshot(t *testing.T, pool *pgxpool.Pool) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, tbl := range eraseTables {
		rows, err := pool.Query(context.Background(),
			fmt.Sprintf(`SELECT t::text FROM %s t ORDER BY 1`, tbl))
		if err != nil {
			t.Fatalf("snapshot %s: %v", tbl, err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out[tbl] = append(out[tbl], s)
		}
		rows.Close()
	}
	return out
}

func rowsJSON(t *testing.T, pool *pgxpool.Pool, query string, args ...any) []map[string]any {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT row_to_json(q) FROM (`+query+`) q`, args...)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		m := map[string]any{}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func countWhere(t *testing.T, pool *pgxpool.Pool, table, where string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s`, table, where), args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func seedReview(t *testing.T, pool *pgxpool.Pool, id, listing, user, name string, rating int, comment string) {
	t.Helper()
	exec(t, pool, `INSERT INTO reviews (id, listing_id, user_id, user_name, rating, comment)
		VALUES ($1,$2,$3,$4,$5,$6)`, id, listing, user, name, rating, comment)
}

func seedFullVictim(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, u := range []string{victim, other} {
		exec(t, pool, `INSERT INTO favorites (user_id, listing_id) VALUES ($1,'l1'),($1,'l2')`, u)
		exec(t, pool, `INSERT INTO follows (user_id, seller_id) VALUES ($1,'seller-x')`, u)
		exec(t, pool, `INSERT INTO loyalty_accounts (user_id, coin_balance, streak, last_checkin) VALUES ($1,30,3,'2026-10-01')`, u)
		exec(t, pool, `INSERT INTO checkins (user_id, day) VALUES ($1,'2026-09-29'),($1,'2026-09-30'),($1,'2026-10-01')`, u)
		for i := 1; i <= 5; i++ {
			exec(t, pool, `INSERT INTO view_history (user_id, listing_id) VALUES ($1,$2)`, u, fmt.Sprintf("v%d", i))
		}
		cid := "col-" + u
		exec(t, pool, `INSERT INTO collections (id, user_id, name) VALUES ($1,$2,'wish')`, cid, u)
		exec(t, pool, `INSERT INTO collection_items (collection_id, listing_id) VALUES ($1,'l1'),($1,'l2')`, cid)
		exec(t, pool, `INSERT INTO reports (id, target_type, target_id, reason_code, reporter_id) VALUES ($1,'listing','lx','spam',$2)`, "rep-by-"+u, u)
		seedReview(t, pool, "rev-"+u, "l1", u, "Name "+u, 4, "text "+u)
		exec(t, pool, `INSERT INTO product_questions (id, listing_id, user_id, question_text) VALUES ($1,'l1',$2,'q text')`, "q-"+u, u)
	}
	exec(t, pool, `INSERT INTO review_helpful_votes (review_id, user_id) VALUES ('rev-`+other+`',$1)`, victim)
	exec(t, pool, `INSERT INTO review_helpful_votes (review_id, user_id) VALUES ('rev-`+victim+`',$1)`, other)
	exec(t, pool, `INSERT INTO follows (user_id, seller_id) VALUES ('f1',$1),('f2',$1),('f3',$1),('f1','seller-y')`, victim)
}

func eraseOK(t *testing.T, repo *store.PostgresRepository, id string) {
	t.Helper()
	if _, err := repo.Erase(context.Background(), id); err != nil {
		t.Fatalf("Erase(%s): %v", id, err)
	}
}

// Scenario: A review keeps rating and text under an anonymised author
func TestEraseReviewKeepsRatingAndTextUnderAnonymisedAuthor(t *testing.T) {
	pool := testPool(t)
	seedReview(t, pool, "r1", "l1", victim, "Real Name", 4, "Giao nhanh")
	repo := eraser(t, pool)
	rows, err := repo.Erase(context.Background(), victim)
	if err != nil || rows < 1 {
		t.Fatalf("Erase = %d, %v; want at least one row and no error", rows, err)
	}
	reviews := store.NewPostgresReviewRepository(pool, testOutbox(t, pool))
	got, total, err := reviews.ListReviews(context.Background(), "l1", 0, 0, 10)
	if err != nil || total != 1 || len(got) != 1 {
		t.Fatalf("ListReviews = %v total=%d err=%v", got, total, err)
	}
	r := got[0]
	if r.Rating != 4 || r.Comment != "Giao nhanh" || r.UserName != erasedName {
		t.Fatalf("review = rating %d comment %q name %q", r.Rating, r.Comment, r.UserName)
	}
	if r.UserID == victim {
		t.Fatal("review still carries the erased user id")
	}
	if n := countWhere(t, pool, "reviews", "user_id = $1 OR user_name = 'Real Name'", victim); n != 0 {
		t.Fatalf("%d review rows still identify the user", n)
	}
}

// Scenario: A question and its answers keep their text under an anonymised author
func TestEraseQuestionAndAnswersKeepTextUnderAnonymisedAuthor(t *testing.T) {
	pool := testPool(t)
	exec(t, pool, `INSERT INTO product_questions (id, listing_id, user_id, question_text) VALUES
		('q1','l1',$1,'Con hang khong?'),('q2','l1','asker','Bao hanh?')`, victim)
	exec(t, pool, `UPDATE product_questions SET user_id='asker' WHERE id='q2'`)
	exec(t, pool, `INSERT INTO product_answers (id, question_id, listing_id, user_id, answer_text) VALUES
		('a1','q1','l1','seller','Con'),('a2','q2','l1',$1,'Co 12 thang')`, victim)
	eraseOK(t, eraser(t, pool), victim)

	qs, total, err := store.NewPostgresQARepository(pool).ListQuestionsByListing(context.Background(), "l1", 0, 10)
	if err != nil || total != 2 {
		t.Fatalf("ListQuestionsByListing total=%d err=%v", total, err)
	}
	texts := map[string]bool{}
	for _, q := range qs {
		texts[q.QuestionText] = true
		if q.UserID == victim {
			t.Fatal("question still carries the erased user id")
		}
		for _, a := range q.Answers {
			texts[a.AnswerText] = true
			if a.UserID == victim {
				t.Fatal("answer still carries the erased user id")
			}
		}
	}
	for _, want := range []string{"Con hang khong?", "Bao hanh?", "Con", "Co 12 thang"} {
		if !texts[want] {
			t.Fatalf("text %q lost; have %v", want, texts)
		}
	}
	if n := countWhere(t, pool, "product_questions", "user_id=$1", victim) +
		countWhere(t, pool, "product_answers", "user_id=$1", victim); n != 0 {
		t.Fatalf("%d Q&A rows still carry the erased id", n)
	}
}

// Scenario: The listing rating is unchanged
func TestEraseListingRatingUnchanged(t *testing.T) {
	pool := testPool(t)
	seedReview(t, pool, "r1", "l1", victim, "V", 5, "a")
	seedReview(t, pool, "r2", "l1", "u2", "U2", 3, "b")
	seedReview(t, pool, "r3", "l1", "u3", "U3", 2, "c")
	for _, id := range []string{"r1", "r2", "r3"} {
		exec(t, pool, `UPDATE reviews SET verified_purchase = TRUE, order_id = 'ord-' || id WHERE id = $1`, id)
	}
	reviews := store.NewPostgresReviewRepository(pool, testOutbox(t, pool))
	before, err := reviews.GetRatingSummary(context.Background(), "l1")
	if err != nil {
		t.Fatal(err)
	}
	eraseOK(t, eraser(t, pool), victim)
	after, err := reviews.GetRatingSummary(context.Background(), "l1")
	if err != nil {
		t.Fatal(err)
	}
	if before.ReviewCount != 3 || !reflect.DeepEqual(before, after) {
		t.Fatalf("summary before %+v after %+v; want identical with count 3", before, after)
	}
}

// Scenario: The moderation reason of an erased author is cleared
func TestEraseClearsHideReasonOfErasedAuthor(t *testing.T) {
	pool := testPool(t)
	seedReview(t, pool, "r1", "l1", victim, "V", 1, "bad")
	exec(t, pool, `UPDATE reviews SET hidden_at = now(), hidden_by = 'admin-1', hide_reason = 'contains phone 0901234567' WHERE id = 'r1'`)
	eraseOK(t, eraser(t, pool), victim)
	rows := rowsJSON(t, pool, `SELECT hide_reason, hidden_at, hidden_by FROM reviews WHERE id = 'r1'`)
	if len(rows) != 1 {
		t.Fatalf("review row missing: %v", rows)
	}
	if rows[0]["hide_reason"] != "" || rows[0]["hidden_at"] == nil || rows[0]["hidden_by"] != "admin-1" {
		t.Fatalf("hide fields = %v; want empty reason, hidden_at set, hidden_by kept", rows[0])
	}
	visible, _, err := store.NewPostgresReviewRepository(pool, testOutbox(t, pool)).ListReviews(context.Background(), "l1", 0, 0, 10)
	if err != nil || len(visible) != 0 {
		t.Fatalf("review must stay hidden from buyers: %v %v", visible, err)
	}
}

// Scenario: An erased moderator no longer appears as hidden_by
func TestEraseErasedModeratorNoLongerHiddenBy(t *testing.T) {
	pool := testPool(t)
	seedReview(t, pool, "r1", "l1", "u1", "U1", 1, "x")
	seedReview(t, pool, "r2", "l1", "u2", "U2", 1, "y")
	seedReview(t, pool, "r3", "l1", "u3", "U3", 1, "z")
	exec(t, pool, `UPDATE reviews SET hidden_at = now(), hidden_by = $1, hide_reason = 'spam' WHERE id IN ('r1','r2')`, victim)
	exec(t, pool, `UPDATE reviews SET hidden_at = now(), hidden_by = 'admin-2', hide_reason = 'abuse' WHERE id = 'r3'`)
	eraseOK(t, eraser(t, pool), victim)
	for _, id := range []string{"r1", "r2"} {
		r := rowsJSON(t, pool, `SELECT hidden_by, hide_reason, hidden_at FROM reviews WHERE id = $1`, id)[0]
		if r["hidden_by"] != "" || r["hide_reason"] != "spam" || r["hidden_at"] == nil {
			t.Fatalf("%s = %v; want hidden_by empty, reason spam, still hidden", id, r)
		}
	}
	r3 := rowsJSON(t, pool, `SELECT hidden_by, hide_reason FROM reviews WHERE id = 'r3'`)[0]
	if r3["hidden_by"] != "admin-2" || r3["hide_reason"] != "abuse" {
		t.Fatalf("other moderator's review changed: %v", r3)
	}
}

// Scenario: Other users' content is untouched
func TestEraseOtherUsersContentUntouched(t *testing.T) {
	pool := testPool(t)
	seedFullVictim(t, pool)
	queries := map[string]string{
		"reviews":   `SELECT * FROM reviews WHERE id = 'rev-` + other + `'`,
		"questions": `SELECT * FROM product_questions WHERE id = 'q-` + other + `'`,
		"favorites": `SELECT * FROM favorites WHERE user_id = '` + other + `'`,
		"follows":   `SELECT * FROM follows WHERE user_id = '` + other + `'`,
		"loyalty":   `SELECT * FROM loyalty_accounts WHERE user_id = '` + other + `'`,
		"checkins":  `SELECT * FROM checkins WHERE user_id = '` + other + `'`,
		"history":   `SELECT * FROM view_history WHERE user_id = '` + other + `'`,
		"coll":      `SELECT * FROM collections WHERE user_id = '` + other + `'`,
		"items":     `SELECT * FROM collection_items WHERE collection_id = 'col-` + other + `'`,
		"reports":   `SELECT * FROM reports WHERE reporter_id = '` + other + `'`,
		"votes":     `SELECT * FROM review_helpful_votes WHERE user_id = '` + other + `'`,
	}
	before := map[string][]map[string]any{}
	for k, q := range queries {
		before[k] = rowsJSON(t, pool, q)
		if len(before[k]) == 0 {
			t.Fatalf("seed for %s is empty", k)
		}
	}
	eraseOK(t, eraser(t, pool), victim)
	for k, q := range queries {
		if after := rowsJSON(t, pool, q); !reflect.DeepEqual(before[k], after) {
			t.Errorf("%s changed: before %v after %v", k, before[k], after)
		}
	}
}

// Scenario: Favourites, follows and reports filed by the user are deleted
func TestEraseDeletesFavouritesFollowsReportsAndVotes(t *testing.T) {
	pool := testPool(t)
	seedFullVictim(t, pool)
	eraseOK(t, eraser(t, pool), victim)
	for tbl, col := range map[string]string{
		"favorites": "user_id", "follows": "user_id", "reports": "reporter_id", "review_helpful_votes": "user_id",
	} {
		if n := countWhere(t, pool, tbl, col+" = $1", victim); n != 0 {
			t.Errorf("%s keeps %d rows of the erased user", tbl, n)
		}
	}
}

// Scenario: An erased seller loses their followers
func TestEraseErasedSellerLosesFollowers(t *testing.T) {
	pool := testPool(t)
	seedFullVictim(t, pool)
	eraseOK(t, eraser(t, pool), victim)
	if n := countWhere(t, pool, "follows", "seller_id = $1", victim); n != 0 {
		t.Fatalf("%d follows of the erased seller remain", n)
	}
	if n := countWhere(t, pool, "follows", "user_id = 'f1' AND seller_id = 'seller-y'"); n != 1 {
		t.Fatalf("a follower's other follow was removed")
	}
	if n := countWhere(t, pool, "follows", "user_id = $1", other); n != 1 {
		t.Fatalf("another user's follow was removed")
	}
}

// Scenario: Collections, history, check-ins and loyalty are deleted
func TestEraseDeletesCollectionsHistoryCheckinsAndLoyalty(t *testing.T) {
	pool := testPool(t)
	seedFullVictim(t, pool)
	eraseOK(t, eraser(t, pool), victim)
	for tbl, col := range map[string]string{
		"collections": "user_id", "view_history": "user_id", "checkins": "user_id", "loyalty_accounts": "user_id",
	} {
		if n := countWhere(t, pool, tbl, col+" = $1", victim); n != 0 {
			t.Errorf("%s keeps %d rows of the erased user", tbl, n)
		}
	}
	if n := countWhere(t, pool, "collection_items", "collection_id = $1", "col-"+victim); n != 0 {
		t.Errorf("collection_items keeps %d rows", n)
	}
}

// Scenario: Reports about the erased user's content stay open
func TestEraseReportsAboutErasedContentStayOpen(t *testing.T) {
	pool := testPool(t)
	seedReview(t, pool, "r1", "l1", victim, "V", 2, "meh")
	exec(t, pool, `INSERT INTO reports (id, target_type, target_id, reason_code, reporter_id, status)
		VALUES ('rep1','review','r1','spam','reporter-1','OPEN')`)
	eraseOK(t, eraser(t, pool), victim)
	rows := rowsJSON(t, pool, `SELECT status, target_id FROM reports WHERE id = 'rep1'`)
	if len(rows) != 1 || rows[0]["status"] != "OPEN" || rows[0]["target_id"] != "r1" {
		t.Fatalf("report = %v; want still OPEN on r1", rows)
	}
}

// Scenario: A replayed event changes nothing
func TestEraseReplayChangesNothing(t *testing.T) {
	pool := testPool(t)
	seedFullVictim(t, pool)
	exec(t, pool, `UPDATE reviews SET hidden_at = now(), hidden_by = $1, hide_reason = 'r' WHERE id = 'rev-`+other+`'`, victim)
	repo := eraser(t, pool)
	eraseOK(t, repo, victim)
	first := snapshot(t, pool)
	if _, err := repo.Erase(context.Background(), victim); err != nil {
		t.Fatalf("second Erase: %v", err)
	}
	if second := snapshot(t, pool); !reflect.DeepEqual(first, second) {
		t.Fatalf("replay changed state:\nfirst  %v\nsecond %v", first, second)
	}
}

// Scenario: A store failure rolls back and is retried
func TestEraseRollsBackOnFailure(t *testing.T) {
	pool := testPool(t)
	seedFullVictim(t, pool)
	exec(t, pool, `CREATE FUNCTION fail_erase() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$`)
	for _, tbl := range []string{"loyalty_accounts", "checkins", "view_history", "reports"} {
		exec(t, pool, fmt.Sprintf(`CREATE TRIGGER fail_erase_%s BEFORE DELETE ON %s FOR EACH ROW EXECUTE FUNCTION fail_erase()`, tbl, tbl))
	}
	before := snapshot(t, pool)
	repo := eraser(t, pool)
	if _, err := repo.Erase(context.Background(), victim); err == nil {
		t.Fatal("Erase must return the store failure so the record is retried")
	} else if !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("error does not carry the cause: %v", err)
	}
	if after := snapshot(t, pool); !reflect.DeepEqual(before, after) {
		t.Fatalf("partial change survived the failure:\nbefore %v\nafter  %v", before, after)
	}
	for _, tbl := range []string{"loyalty_accounts", "checkins", "view_history", "reports"} {
		exec(t, pool, fmt.Sprintf(`DROP TRIGGER fail_erase_%s ON %s`, tbl, tbl))
	}
	eraseOK(t, repo, victim)
	if n := countWhere(t, pool, "favorites", "user_id = $1", victim); n != 0 {
		t.Fatalf("retry after recovery did not complete the erasure: %d favourites left", n)
	}
}

// Scenario: An event without a user id deletes nothing
func TestEraseEmptyUserIDRefusedAndDeletesNothing(t *testing.T) {
	pool := testPool(t)
	seedFullVictim(t, pool)
	before := snapshot(t, pool)
	if _, err := eraser(t, pool).Erase(context.Background(), ""); err == nil {
		t.Fatal("Erase with an empty user id must fail")
	}
	if after := snapshot(t, pool); !reflect.DeepEqual(before, after) {
		t.Fatal("empty user id changed rows")
	}
}
