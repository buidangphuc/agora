package repository

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/interceptor"
)

func callerCtx() context.Context {
	return interceptor.ContextWithPrincipal(context.Background(),
		&commonv1.Principal{Id: "buyer-1", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER})
}

func TestOutbox_FactsWrittenWithTheChange(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	r := NewPostgresRepository(pool)

	if added, err := r.AddFavorite(ctx, "buyer-1", "L1"); err != nil || !added {
		t.Fatalf("add: %v %v", added, err)
	}
	if _, err := r.RemoveFavorite(ctx, "buyer-1", "L1"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Follow(ctx, "buyer-1", "S1"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Unfollow(ctx, "buyer-1", "S1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"FavoriteAdded", "FavoriteRemoved", "SellerFollowed", "SellerUnfollowed"}
	if got := outboxTypes(t, pool); !reflect.DeepEqual(got, want) {
		t.Fatalf("types = %v, want %v", got, want)
	}

	var typ, key, pid, ptype string
	var payload []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT type, key, principal_id, principal_type, payload FROM outbox ORDER BY seq LIMIT 1`).
		Scan(&typ, &key, &pid, &ptype, &payload); err != nil {
		t.Fatal(err)
	}
	var fa engagementv1.FavoriteAdded
	if err := proto.Unmarshal(payload, &fa); err != nil {
		t.Fatal(err)
	}
	if typ != "platform.engagement.v1.FavoriteAdded" || key != "L1" || pid != "buyer-1" ||
		ptype != "PRINCIPAL_TYPE_USER" || fa.GetUserId() != "buyer-1" || fa.GetListingId() != "L1" {
		t.Fatalf("row = %s %s %s %s %v", typ, key, pid, ptype, &fa)
	}
	// follow facts are keyed by seller id
	if err := pool.QueryRow(context.Background(), `SELECT key FROM outbox WHERE type LIKE '%SellerFollowed'`).Scan(&key); err != nil || key != "S1" {
		t.Fatalf("follow key = %q %v", key, err)
	}
}

func TestOutbox_NoOpWritesNoFact(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	r := NewPostgresRepository(pool)

	_, _ = r.AddFavorite(ctx, "u", "L1")
	if added, _ := r.AddFavorite(ctx, "u", "L1"); added {
		t.Fatal("repeat add must be a no-op")
	}
	if removed, _ := r.RemoveFavorite(ctx, "u", "missing"); removed {
		t.Fatal("remove of missing must be a no-op")
	}
	_, _ = r.Follow(ctx, "u", "S1")
	_, _ = r.Follow(ctx, "u", "S1")
	if removed, _ := r.Unfollow(ctx, "u", "nobody"); removed {
		t.Fatal("unfollow of missing must be a no-op")
	}
	if got := outboxTypes(t, pool); !reflect.DeepEqual(got, []string{"FavoriteAdded", "SellerFollowed"}) {
		t.Fatalf("types = %v", got)
	}
}

func TestOutbox_RolledBackTransactionWritesNoFact(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	if _, err := pool.Exec(ctx, `
CREATE FUNCTION boom() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'boom'; END $$ LANGUAGE plpgsql;
CREATE TRIGGER stats_boom BEFORE INSERT ON listing_stats FOR EACH ROW EXECUTE FUNCTION boom();`); err != nil {
		t.Fatal(err)
	}
	r := NewPostgresRepository(pool)
	if _, err := r.AddFavorite(ctx, "u", "L1"); err == nil {
		t.Fatal("expected failure")
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM outbox) + (SELECT count(*) FROM favorites)`).Scan(&n)
	if n != 0 {
		t.Fatalf("rolled-back change left %d rows", n)
	}
}

func TestOutbox_ReviewFactHasRatingNotText(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	if err := NewPostgresRepository(pool).UpsertSellerListing(ctx, "S9", "L1", time.Now()); err != nil {
		t.Fatal(err)
	}
	rev, err := NewPostgresReviewRepository(pool).CreateReview(ctx,
		Review{ListingID: "L1", UserID: "buyer-1", Rating: 4, Comment: "rất tốt"})
	if err != nil {
		t.Fatal(err)
	}
	var key string
	var payload []byte
	if err := pool.QueryRow(ctx, `SELECT key, payload FROM outbox WHERE type LIKE '%ReviewCreated'`).Scan(&key, &payload); err != nil {
		t.Fatal(err)
	}
	var rc engagementv1.ReviewCreated
	if err := proto.Unmarshal(payload, &rc); err != nil {
		t.Fatal(err)
	}
	if key != "L1" || rc.GetRating() != 4 || rc.GetSellerId() != "S9" || rc.GetReviewId() != rev.ID || rc.GetUserId() != "buyer-1" {
		t.Fatalf("fact = %v key=%s", &rc, key)
	}
	if string(payload) != string(mustMarshal(t, &rc)) || contains(payload, "rất tốt") {
		t.Fatal("payload must not contain review text")
	}
}

func mustMarshal(t *testing.T, m proto.Message) []byte {
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func contains(b []byte, s string) bool { return len(s) > 0 && strings.Contains(string(b), s) }

func TestOutbox_RelayOrderStopAndRetention(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	r := NewPostgresRepository(pool)
	for _, l := range []string{"L1", "L2", "L3"} {
		if _, err := r.AddFavorite(ctx, "u", l); err != nil {
			t.Fatal(err)
		}
	}
	ob := NewPgOutbox(pool)

	var got []string
	failAt := "L2"
	pub := func(_ context.Context, m OutboxMessage) error {
		if m.Key == failAt {
			return errors.New("kafka down")
		}
		got = append(got, m.Key)
		return nil
	}
	n, err := ob.Relay(ctx, 10, OutboxRetention, pub)
	if err == nil || n != 1 || !reflect.DeepEqual(got, []string{"L1"}) {
		t.Fatalf("first cycle n=%d err=%v got=%v (must stop at the failure)", n, err, got)
	}
	failAt = ""
	n, err = ob.Relay(ctx, 10, OutboxRetention, pub)
	if err != nil || n != 2 || !reflect.DeepEqual(got, []string{"L1", "L2", "L3"}) {
		t.Fatalf("second cycle n=%d err=%v got=%v", n, err, got)
	}
	if n, _ = ob.Relay(ctx, 10, OutboxRetention, pub); n != 0 {
		t.Fatalf("nothing left to publish, got %d", n)
	}

	// Published rows older than 7 days are deleted on the next cycle; fresh ones stay.
	if _, err := pool.Exec(ctx, `UPDATE outbox SET published_at = now() - interval '8 days' WHERE key = 'L1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := ob.Relay(ctx, 10, OutboxRetention, pub); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&left)
	if left != 2 {
		t.Fatalf("rows left = %d, want 2", left)
	}
}
