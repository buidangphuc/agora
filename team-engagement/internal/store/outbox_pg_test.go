package store

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
)

func callerCtx() context.Context {
	return auth.WithPrincipal(context.Background(),
		auth.Principal{ID: "buyer-1", Type: "user", Scopes: []string{"engagement:write"}})
}

// outboxFact is one outbox row as the relayer will publish it: the row columns and
// the EventEnvelope decoded from its payload.
type outboxFact struct {
	key, typ string
	env      *eventsv1.EventEnvelope
	raw      []byte
}

// facts returns the outbox rows in the order they were written.
func facts(t *testing.T, pool *pgxpool.Pool) []outboxFact {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT aggregate_id, event_type, payload FROM `+OutboxTable+` ORDER BY created_at, event_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []outboxFact
	for rows.Next() {
		var f outboxFact
		if err := rows.Scan(&f.key, &f.typ, &f.raw); err != nil {
			t.Fatal(err)
		}
		f.env = &eventsv1.EventEnvelope{}
		if err := proto.Unmarshal(f.raw, f.env); err != nil {
			t.Fatal(err)
		}
		out = append(out, f)
	}
	return out
}

func TestOutbox_FactsWrittenWithTheChange(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	r := NewPostgresRepository(pool, testOutbox(t, pool))

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

	all := facts(t, pool)
	first := all[0]
	var fa engagementv1.FavoriteAdded
	if err := proto.Unmarshal(first.env.GetPayload(), &fa); err != nil {
		t.Fatal(err)
	}
	if first.typ != "platform.engagement.v1.FavoriteAdded" || first.env.GetType() != first.typ || first.key != "L1" ||
		first.env.GetPrincipal().GetId() != "buyer-1" || first.env.GetPrincipal().GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER ||
		fa.GetUserId() != "buyer-1" || fa.GetListingId() != "L1" {
		t.Fatalf("row = %s %s %v %v", first.typ, first.key, first.env, &fa)
	}
	// follow facts are keyed by seller id
	for _, f := range all {
		if strings.HasSuffix(f.typ, "SellerFollowed") && f.key != "S1" {
			t.Fatalf("follow key = %q", f.key)
		}
	}
}

func TestOutbox_NoOpWritesNoFact(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	r := NewPostgresRepository(pool, testOutbox(t, pool))

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
	r := NewPostgresRepository(pool, testOutbox(t, pool))
	if _, err := r.AddFavorite(ctx, "u", "L1"); err == nil {
		t.Fatal("expected failure")
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM `+OutboxTable+`) + (SELECT count(*) FROM favorites)`).Scan(&n)
	if n != 0 {
		t.Fatalf("rolled-back change left %d rows", n)
	}
}

func TestOutbox_ReviewFactHasRatingNotText(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	if err := NewPostgresRepository(pool, testOutbox(t, pool)).UpsertSellerListing(ctx, "S9", "L1", time.Now()); err != nil {
		t.Fatal(err)
	}
	rev, err := NewPostgresReviewRepository(pool, testOutbox(t, pool)).CreateReview(ctx,
		Review{ListingID: "L1", UserID: "buyer-1", Rating: 4, Comment: "rất tốt"})
	if err != nil {
		t.Fatal(err)
	}
	var key string
	var envelope []byte
	if err := pool.QueryRow(ctx, `SELECT aggregate_id, payload FROM `+OutboxTable+` WHERE event_type LIKE '%ReviewCreated'`).Scan(&key, &envelope); err != nil {
		t.Fatal(err)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(envelope, &env); err != nil {
		t.Fatal(err)
	}
	payload := env.GetPayload()
	var rc engagementv1.ReviewCreated
	if err := proto.Unmarshal(payload, &rc); err != nil {
		t.Fatal(err)
	}
	if key != "L1" || rc.GetRating() != 4 || rc.GetSellerId() != "S9" || rc.GetReviewId() != rev.ID || rc.GetUserId() != "buyer-1" {
		t.Fatalf("fact = %v key=%s", &rc, key)
	}
	if string(payload) != string(mustMarshal(t, &rc)) || contains(payload, "rất tốt") || contains(envelope, "rất tốt") {
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

// A failing publish does not hold back the facts of other aggregates: the SDK
// relayer retries the failed row behind a backoff gate and keeps going. go-sdk v1
// orders per aggregate only, so the keys are compared without order. (The order
// within one aggregate is TestOutboxUpgradeKeepsCreationOrder; TestOutboxRelayedViaSDK
// covers parking.)
func TestOutbox_RelayOrderAndFailureIsolation(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	r := NewPostgresRepository(pool, testOutbox(t, pool))
	for _, l := range []string{"L1", "L2", "L3"} {
		if _, err := r.AddFavorite(ctx, "u", l); err != nil {
			t.Fatal(err)
		}
	}
	pub := testkit.NewRecorder()
	pub.FailOn("L2")
	relayer, err := outbox.NewRelayer(testOutbox(t, pool), pub, outbox.RelayerOptions{
		Topic:  "engagement.events",
		Config: sdkconfig.Outbox{PollInterval: time.Second, BatchSize: 10, ClaimLock: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := relayer.RelayOnce(ctx)
	keys := recordedKeys(pub)
	slices.Sort(keys)
	if err != nil || rep.Published != 2 || rep.Retried != 1 || !reflect.DeepEqual(keys, []string{"L1", "L3"}) {
		t.Fatalf("first pass %+v err=%v keys=%v, want L1 and L3 published and L2 retried", rep, err, keys)
	}
	var status string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT status, attempts FROM `+OutboxTable+` WHERE aggregate_id = 'L2'`).Scan(&status, &attempts); err != nil ||
		status != "pending" || attempts != 1 {
		t.Fatalf("failed row = %s/%d err=%v, want pending after one attempt", status, attempts, err)
	}
}

// recordedKeys lists the keys a Recorder captured, in publish order.
func recordedKeys(r *testkit.Recorder) []string {
	var keys []string
	for _, rec := range r.Records() {
		keys = append(keys, string(rec.Key))
	}
	return keys
}
