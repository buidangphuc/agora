package store_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/store"
)

func TestFactsWrittenWithTheChange(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtxWithRequest("r-9")
	r := store.NewPostgresRepository(pool, testOutbox(t, pool))

	if added, err := r.AddFavorite(ctx, "buyer-1", "L1"); err != nil || !added {
		t.Fatalf("add: %v %v", added, err)
	}
	if removed, err := r.RemoveFavorite(ctx, "buyer-1", "L1"); err != nil || !removed {
		t.Fatalf("remove: %v %v", removed, err)
	}
	if _, err := r.Follow(ctx, "buyer-1", "S1"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Unfollow(ctx, "buyer-1", "S1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"FavoriteAdded", "FavoriteRemoved", "SellerFollowed", "SellerUnfollowed"}
	if got := factTypes(t, pool); !reflect.DeepEqual(got, want) {
		t.Fatalf("types = %v, want %v", got, want)
	}

	first := factRows(t, pool)[0]
	var fa engagementv1.FavoriteAdded
	if err := proto.Unmarshal(first.env.GetPayload(), &fa); err != nil {
		t.Fatal(err)
	}
	if first.aggregateType != "Listing" || first.aggregateID != "L1" ||
		first.eventType != "platform.engagement.v1.FavoriteAdded" || first.env.GetType() != first.eventType ||
		first.env.GetEventId() == "" || first.env.GetRequestId() != "r-9" ||
		first.env.GetPrincipal().GetId() != "buyer-1" || first.env.GetPrincipal().GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER ||
		fa.GetUserId() != "buyer-1" || fa.GetListingId() != "L1" {
		t.Fatalf("row = %+v env=%v payload=%v", first, first.env, &fa)
	}
	var column string
	if err := pool.QueryRow(context.Background(), `SELECT request_id FROM `+factsTable+` ORDER BY seq LIMIT 1`).Scan(&column); err != nil || column != "r-9" {
		t.Fatalf("row request_id = %q err=%v, want r-9", column, err)
	}
}

func TestFollowEnqueuesSellerKeyedFact(t *testing.T) {
	pool := testPool(t)
	r := store.NewPostgresRepository(pool, testOutbox(t, pool))

	if added, err := r.Follow(callerCtx(), "buyer-1", "S1"); err != nil || !added {
		t.Fatalf("follow: %v %v", added, err)
	}
	rows := factsOfType(t, pool, "platform.engagement.v1.SellerFollowed")
	if len(rows) != 1 || rows[0].aggregateType != "Seller" || rows[0].aggregateID != "S1" {
		t.Fatalf("SellerFollowed rows = %+v, want one with aggregate Seller/S1", rows)
	}
	var m engagementv1.SellerFollowed
	if err := proto.Unmarshal(rows[0].env.GetPayload(), &m); err != nil || m.GetUserId() != "buyer-1" || m.GetSellerId() != "S1" {
		t.Fatalf("payload = %v err=%v", &m, err)
	}
	if _, err := r.Unfollow(callerCtx(), "buyer-1", "S1"); err != nil {
		t.Fatal(err)
	}
	un := factsOfType(t, pool, "platform.engagement.v1.SellerUnfollowed")
	if len(un) != 1 || un[0].aggregateType != "Seller" || un[0].aggregateID != "S1" {
		t.Fatalf("SellerUnfollowed rows = %+v, want one with aggregate Seller/S1", un)
	}
}

func TestRepeatedFavouriteWritesOneFact(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	r := store.NewPostgresRepository(pool, testOutbox(t, pool))

	if added, err := r.AddFavorite(ctx, "u", "L1"); err != nil || !added {
		t.Fatalf("first add: %v %v", added, err)
	}
	if added, err := r.AddFavorite(ctx, "u", "L1"); err != nil || added {
		t.Fatalf("repeat add: added=%v err=%v, want false,nil", added, err)
	}
	if n := len(factsOfType(t, pool, "platform.engagement.v1.FavoriteAdded")); n != 1 {
		t.Fatalf("FavoriteAdded rows = %d, want 1", n)
	}
}

func TestNoOpCallsWriteNoFact(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	r := store.NewPostgresRepository(pool, testOutbox(t, pool))

	_, _ = r.AddFavorite(ctx, "u", "L1")
	if removed, _ := r.RemoveFavorite(ctx, "u", "missing"); removed {
		t.Fatal("remove of missing must be a no-op")
	}
	_, _ = r.Follow(ctx, "u", "S1")
	if added, _ := r.Follow(ctx, "u", "S1"); added {
		t.Fatal("repeat follow must be a no-op")
	}
	if removed, _ := r.Unfollow(ctx, "u", "nobody"); removed {
		t.Fatal("unfollow of missing must be a no-op")
	}
	if got := factTypes(t, pool); !reflect.DeepEqual(got, []string{"FavoriteAdded", "SellerFollowed"}) {
		t.Fatalf("types = %v, want [FavoriteAdded SellerFollowed]", got)
	}
}

func TestFactRolledBackWithBusinessWrite(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	if _, err := pool.Exec(ctx, `
CREATE FUNCTION fail_review_fact() RETURNS trigger AS $$ BEGIN
  IF NEW.event_type = 'platform.engagement.v1.ReviewCreated' THEN RAISE EXCEPTION 'outbox down'; END IF;
  RETURN NEW; END $$ LANGUAGE plpgsql;
CREATE TRIGGER fail_review_fact BEFORE INSERT ON engagement_outbox_events FOR EACH ROW EXECUTE FUNCTION fail_review_fact();`); err != nil {
		t.Fatal(err)
	}
	repo := store.NewPostgresReviewRepository(pool, testOutbox(t, pool))
	if _, err := repo.CreateReview(ctx, store.Review{ListingID: "L1", UserID: "buyer-1", OrderID: "O1", Rating: 4, VerifiedPurchase: true, SellerID: "S1"}); err == nil {
		t.Fatal("CreateReview returned nil error although the fact insert failed")
	}
	if n := countRows(t, pool, "reviews"); n != 0 {
		t.Fatalf("reviews rows = %d, want 0", n)
	}
	if n := countRows(t, pool, factsTable); n != 0 {
		t.Fatalf("%s rows = %d, want 0", factsTable, n)
	}
}

func TestFactRolledBackWithFavouriteWrite(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	if _, err := pool.Exec(ctx, `
CREATE FUNCTION boom() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'boom'; END $$ LANGUAGE plpgsql;
CREATE TRIGGER stats_boom BEFORE INSERT ON listing_stats FOR EACH ROW EXECUTE FUNCTION boom();`); err != nil {
		t.Fatal(err)
	}
	r := store.NewPostgresRepository(pool, testOutbox(t, pool))
	if _, err := r.AddFavorite(ctx, "u", "L1"); err == nil {
		t.Fatal("expected failure")
	}
	if n := countRows(t, pool, factsTable) + countRows(t, pool, "favorites"); n != 0 {
		t.Fatalf("rolled-back change left %d rows", n)
	}
}

func TestLegacyOutboxIsNotWritten(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	if _, err := pool.Exec(ctx, `INSERT INTO `+legacyTable+` (event_id, type, key, payload)
		VALUES (gen_random_uuid(), 'old.Type', 'k1', '\x00'), (gen_random_uuid(), 'old.Type', 'k2', '\x00')`); err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}
	ob := testOutbox(t, pool)
	repo := store.NewPostgresRepository(pool, ob)
	reviews := store.NewPostgresReviewRepository(pool, ob)
	disputes := store.NewPostgresDisputeRepository(pool, ob)

	if _, err := repo.AddFavorite(ctx, "u", "L1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Follow(ctx, "u", "S1"); err != nil {
		t.Fatal(err)
	}
	rev, err := reviews.CreateReview(ctx, store.Review{ListingID: "L1", UserID: "u", OrderID: "O1", Rating: 5, VerifiedPurchase: true, SellerID: "S1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviews.HideReview(ctx, rev.ID, "admin-1", "spam"); err != nil {
		t.Fatal(err)
	}
	d := seedInvestigating(t, disputes)
	if _, err := disputes.ResolveWithEvent(ctx, d, store.DisputeStatusInvestigating, resolvedFact(d)); err != nil {
		t.Fatal(err)
	}

	if n := countRows(t, pool, legacyTable); n != 2 {
		t.Fatalf("legacy outbox rows = %d, want the 2 seeded rows unchanged", n)
	}
	if n := countRows(t, pool, factsTable); n != 5 {
		t.Fatalf("%s rows = %d, want 5 (favourite, follow, review, hide, dispute)", factsTable, n)
	}
}

func TestResolveWithEventEnqueuesOneDisputeResolved(t *testing.T) {
	pool := testPool(t)
	repo := store.NewPostgresDisputeRepository(pool, testOutbox(t, pool))
	d := seedInvestigating(t, repo)

	admins := []string{"admin-1", "admin-2"}
	errs := make([]error, len(admins))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, admin := range admins {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := adminCtx(admin)
			fact := resolvedFact(d)
			fact.ResolverId = admin
			<-start
			_, errs[i] = repo.ResolveWithEvent(ctx, d, store.DisputeStatusInvestigating, fact)
		}()
	}
	close(start)
	wg.Wait()

	winner := -1
	for i, err := range errs {
		switch {
		case err == nil && winner == -1:
			winner = i
		case err == nil:
			t.Fatalf("both resolutions succeeded: %v", errs)
		case !errors.Is(err, store.ErrDisputeNotInState):
			t.Fatalf("loser err = %v, want ErrDisputeNotInState", err)
		}
	}
	if winner == -1 {
		t.Fatalf("no resolution succeeded: %v", errs)
	}

	rows := factsOfType(t, pool, disputeResolvedType)
	if len(rows) != 1 || rows[0].aggregateType != "Dispute" || rows[0].aggregateID != d.ID {
		t.Fatalf("DisputeResolved rows = %+v, want exactly one for Dispute/%s", rows, d.ID)
	}
	var got engagementv1.DisputeResolved
	if err := proto.Unmarshal(rows[0].env.GetPayload(), &got); err != nil {
		t.Fatal(err)
	}
	if got.GetDisputeId() != d.ID || got.GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED ||
		got.GetRuling() != engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR || got.GetResolverId() != admins[winner] {
		t.Fatalf("payload = %v, want resolved/buyer-favour by %s", &got, admins[winner])
	}
	if rows[0].env.GetPrincipal().GetId() != admins[winner] {
		t.Fatalf("envelope principal = %q, want the winner %s", rows[0].env.GetPrincipal().GetId(), admins[winner])
	}
}

func TestReviewFactHasRatingNotText(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	ob := testOutbox(t, pool)
	if err := store.NewPostgresRepository(pool, ob).UpsertSellerListing(ctx, "S9", "L1", time.Now()); err != nil {
		t.Fatal(err)
	}
	rev, err := store.NewPostgresReviewRepository(pool, ob).CreateReview(ctx,
		store.Review{ListingID: "L1", UserID: "buyer-1", Rating: 4, Comment: "rất tốt"})
	if err != nil {
		t.Fatal(err)
	}
	rows := factsOfType(t, pool, "platform.engagement.v1.ReviewCreated")
	if len(rows) != 1 || rows[0].aggregateType != "Listing" || rows[0].aggregateID != "L1" {
		t.Fatalf("ReviewCreated rows = %+v, want one keyed Listing/L1", rows)
	}
	var rc engagementv1.ReviewCreated
	if err := proto.Unmarshal(rows[0].env.GetPayload(), &rc); err != nil {
		t.Fatal(err)
	}
	if rc.GetRating() != 4 || rc.GetSellerId() != "S9" || rc.GetReviewId() != rev.ID || rc.GetUserId() != "buyer-1" {
		t.Fatalf("fact = %v", &rc)
	}
	raw, err := proto.Marshal(rows[0].env)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "rất tốt") {
		t.Fatal("fact must not contain the review text")
	}
}
