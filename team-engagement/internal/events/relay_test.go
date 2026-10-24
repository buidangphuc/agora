package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/events"
	"github.com/buidangphuc/team-engagement/internal/store"
)

func newRelayer(t *testing.T, ob *outbox.Store, pub outbox.Publisher, cfg config.Outbox) *outbox.Relayer {
	t.Helper()
	r, err := outbox.NewRelayer(ob, pub, outbox.RelayerOptions{
		Topic: factsTopic, Config: cfg, BaseBackoff: time.Nanosecond, MaxBackoff: 2 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("relayer: %v", err)
	}
	return r
}

func relayUntilQuiet(t *testing.T, r *outbox.Relayer) {
	t.Helper()
	for i := 0; i < 50; i++ {
		rep, err := r.RelayOnce(context.Background())
		if err != nil {
			t.Fatalf("relay pass %d: %v", i, err)
		}
		if rep.Claimed == 0 {
			return
		}
	}
	t.Fatal("relayer never went quiet")
}

func makeDue(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	mustExec(t, pool, `UPDATE `+factsTable+` SET available_at = now() - interval '1 second', locked_until = NULL WHERE status = 'pending'`)
}

func realProducer(t *testing.T, brokers string) *kafkax.Producer {
	t.Helper()
	p, err := kafkax.NewProducer(config.Kafka{Brokers: brokers}, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func statusCount(t *testing.T, pool *pgxpool.Pool, status string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+factsTable+` WHERE status = $1`, status).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func enqueue(t *testing.T, pool *pgxpool.Pool, ob *outbox.Store, aggType, key string, msg proto.Message) string {
	t.Helper()
	row, err := events.FactRow(userCtx("buyer-1"), aggType, key, msg)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := ob.EnqueueTx(context.Background(), tx, row); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	return row.EventID
}

type flakyPublisher struct {
	mu   sync.Mutex
	down bool
	rec  *testkit.Recorder
}

func (f *flakyPublisher) PublishRecord(ctx context.Context, r *kgo.Record) error {
	f.mu.Lock()
	down := f.down
	f.mu.Unlock()
	if down {
		return errors.New("broker unreachable")
	}
	return f.rec.PublishRecord(ctx, r)
}

func (f *flakyPublisher) setDown(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = v
}

func typesOfKey(t *testing.T, recs []*kgo.Record, key string) []string {
	t.Helper()
	var out []string
	for _, r := range recs {
		if string(r.Key) == key {
			out = append(out, envelopeOfRecord(t, r.Value).GetType())
		}
	}
	return out
}

// Scenario: Engagement SDK: Facts of one key arrive in order
// Scenario: Favouriting a listing publishes a fact
// Scenario: Removing a favourite publishes a removal
func TestRelayPublishesPerKeyInOrder(t *testing.T) {
	brokers := testkit.Redpanda(t)
	pool, ob := factsDB(t)
	repo := store.NewPostgresRepository(pool, ob)
	ctx := userCtx("buyer-1")
	if ok, err := repo.AddFavorite(ctx, "buyer-1", "L1"); err != nil || !ok {
		t.Fatalf("add: %v %v", ok, err)
	}
	if ok, err := repo.RemoveFavorite(ctx, "buyer-1", "L1"); err != nil || !ok {
		t.Fatalf("remove: %v %v", ok, err)
	}
	relayUntilQuiet(t, newRelayer(t, ob, realProducer(t, brokers), config.Outbox{BatchSize: 100}))

	recs := testkit.ReadAll(t, brokers, factsTopic, 2, 30*time.Second)
	if got := typesOfKey(t, recs, "L1"); len(got) != 2 ||
		got[0] != "platform.engagement.v1.FavoriteAdded" || got[1] != "platform.engagement.v1.FavoriteRemoved" {
		t.Fatalf("types for key L1 = %v, want FavoriteAdded then FavoriteRemoved", got)
	}
	first := envelopeOfRecord(t, recs[0].Value)
	var fa engagementv1.FavoriteAdded
	if err := proto.Unmarshal(first.GetPayload(), &fa); err != nil || fa.GetUserId() != "buyer-1" || fa.GetListingId() != "L1" {
		t.Fatalf("payload = %v err=%v", &fa, err)
	}
	if first.GetPrincipal().GetId() != "buyer-1" || first.GetPrincipal().GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER {
		t.Fatalf("principal = %v", first.GetPrincipal())
	}
	if n := statusCount(t, pool, "published"); n != 2 {
		t.Fatalf("published rows = %d, want 2", n)
	}
}

// Scenario: Facts of different keys carry no mutual order
func TestRelayOrderIsPerKeyOnly(t *testing.T) {
	brokers := testkit.Redpanda(t)
	pool, ob := factsDB(t)
	repo := store.NewPostgresRepository(pool, ob)
	ctx := userCtx("buyer-1")
	if _, err := repo.AddFavorite(ctx, "buyer-1", "L1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Follow(ctx, "buyer-1", "S1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RemoveFavorite(ctx, "buyer-1", "L1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Unfollow(ctx, "buyer-1", "S1"); err != nil {
		t.Fatal(err)
	}
	relayUntilQuiet(t, newRelayer(t, ob, realProducer(t, brokers), config.Outbox{BatchSize: 100}))

	recs := testkit.ReadAll(t, brokers, factsTopic, 4, 30*time.Second)
	if got := typesOfKey(t, recs, "L1"); len(got) != 2 ||
		got[0] != "platform.engagement.v1.FavoriteAdded" || got[1] != "platform.engagement.v1.FavoriteRemoved" {
		t.Fatalf("key L1 = %v", got)
	}
	if got := typesOfKey(t, recs, "S1"); len(got) != 2 ||
		got[0] != "platform.engagement.v1.SellerFollowed" || got[1] != "platform.engagement.v1.SellerUnfollowed" {
		t.Fatalf("key S1 = %v", got)
	}
	seen := map[string]int{}
	for _, r := range recs {
		seen[envelopeOfRecord(t, r.Value).GetEventId()]++
	}
	if len(seen) != 4 {
		t.Fatalf("event ids = %v, want 4 distinct", seen)
	}
}

// Scenario: Engagement SDK: Broker outage delays but does not lose a fact
// Scenario: A fact written while Kafka is down is published later
func TestRelayTransientErrorsNeverPark(t *testing.T) {
	pool, ob := factsDB(t)
	repo := store.NewPostgresRepository(pool, ob)
	if _, err := repo.Follow(userCtx("buyer-1"), "buyer-1", "S1"); err != nil {
		t.Fatal(err)
	}
	pub := &flakyPublisher{down: true, rec: testkit.NewRecorder()}
	r := newRelayer(t, ob, pub, config.Outbox{BatchSize: 100})

	for i := 0; i < 8; i++ {
		makeDue(t, pool)
		rep, err := r.RelayOnce(context.Background())
		if err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
		if rep.Parked != 0 || rep.Published != 0 {
			t.Fatalf("pass %d report = %+v while the broker is down", i, rep)
		}
	}
	if n := statusCount(t, pool, "failed"); n != 0 {
		t.Fatalf("parked rows = %d after 8 failed passes, want 0", n)
	}
	if n := statusCount(t, pool, "pending"); n != 1 {
		t.Fatalf("pending rows = %d, want 1", n)
	}
	if got := len(pub.rec.Records()); got != 0 {
		t.Fatalf("recorded %d while down", got)
	}

	pub.setDown(false)
	makeDue(t, pool)
	relayUntilQuiet(t, r)
	recs := pub.rec.Records()
	if len(recs) != 1 || string(recs[0].Key) != "S1" || recs[0].Topic != factsTopic {
		t.Fatalf("records = %+v, want one SellerFollowed for S1 on %s", recs, factsTopic)
	}
	env := envelopeOfRecord(t, recs[0].Value)
	var sf engagementv1.SellerFollowed
	if err := proto.Unmarshal(env.GetPayload(), &sf); err != nil || sf.GetUserId() != "buyer-1" || sf.GetSellerId() != "S1" {
		t.Fatalf("payload = %v err=%v", &sf, err)
	}
	if n := statusCount(t, pool, "published"); n != 1 {
		t.Fatalf("published rows = %d, want 1", n)
	}
}

// Scenario: Engagement SDK: Retention keeps pending rows
func TestOutboxRetentionPurgesPublishedOnly(t *testing.T) {
	pool, ob := factsDB(t)
	old := enqueue(t, pool, ob, "Listing", "L-old", &engagementv1.FavoriteAdded{UserId: "u", ListingId: "L-old"})
	recent := enqueue(t, pool, ob, "Listing", "L-recent", &engagementv1.FavoriteAdded{UserId: "u", ListingId: "L-recent"})
	stuck := enqueue(t, pool, ob, "Listing", "L-stuck", &engagementv1.FavoriteAdded{UserId: "u", ListingId: "L-stuck"})
	mustExec(t, pool, `UPDATE `+factsTable+` SET status='published', published_at = now() - interval '8 days' WHERE event_id = $1`, old)
	mustExec(t, pool, `UPDATE `+factsTable+` SET status='published', published_at = now() - interval '6 days' WHERE event_id = $1`, recent)
	mustExec(t, pool, `UPDATE `+factsTable+` SET created_at = now() - interval '30 days' WHERE event_id = $1`, stuck)

	n, err := ob.Purge(context.Background(), 7*24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("purged %d err=%v, want 1", n, err)
	}
	left := map[string]bool{}
	rows, err := pool.Query(context.Background(), `SELECT event_id FROM `+factsTable)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		left[id] = true
	}
	if left[old] || !left[recent] || !left[stuck] || len(left) != 2 {
		t.Fatalf("remaining = %v, want only the 6-day published row and the pending row", left)
	}
}

// Scenario: Engagement SDK: Two relayers publish a row once
func TestTwoRelayersPublishOnce(t *testing.T) {
	pool, ob := factsDB(t)
	want := map[string]bool{}
	for i := 0; i < 50; i++ {
		key := "L" + string(rune('A'+i/26)) + string(rune('a'+i%26))
		want[enqueue(t, pool, ob, "Listing", key, &engagementv1.FavoriteAdded{UserId: "u", ListingId: key})] = true
	}
	rec := testkit.NewRecorder()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		r := newRelayer(t, ob, rec, config.Outbox{BatchSize: 7})
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pass := 0; pass < 200 && statusCount(t, pool, "published") < 50; pass++ {
				if _, err := r.RelayOnce(context.Background()); err != nil {
					t.Errorf("relay: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	seen := map[string]int{}
	for _, r := range rec.Records() {
		seen[envelopeOfRecord(t, r.Value).GetEventId()]++
	}
	if len(seen) != 50 {
		t.Fatalf("distinct event ids on the topic = %d, want 50", len(seen))
	}
	for id, n := range seen {
		if !want[id] || n != 1 {
			t.Fatalf("event %s published %d times (known=%v)", id, n, want[id])
		}
	}
}

// Scenario: Engagement SDK: A committed fact reaches the topic within 2 s
func TestFactReachesTopicWithinTwoSeconds(t *testing.T) {
	brokers := testkit.Redpanda(t)
	pool, ob := factsDB(t)
	repo := store.NewPostgresRepository(pool, ob)

	testkit.Produce(t, brokers, factsTopic, "seed", []byte("seed"))
	reader, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumeTopics(factsTopic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	relayer := newRelayer(t, ob, realProducer(t, brokers), config.Outbox{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- relayer.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Error("relayer did not stop")
		}
	}()
	if _, err := repo.AddFavorite(userCtx("buyer-1"), "buyer-1", "L1"); err != nil {
		t.Fatal(err)
	}
	deadline, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	var got *kgo.Record
	for got == nil {
		fetches := reader.PollRecords(deadline, 1)
		if deadline.Err() != nil {
			t.Fatal("FavoriteAdded did not reach engagement.events within 2 s of the call returning")
		}
		fetches.EachRecord(func(r *kgo.Record) {
			if string(r.Key) != "seed" {
				got = r
			}
		})
	}
	if string(got.Key) != "L1" || envelopeOfRecord(t, got.Value).GetType() != "platform.engagement.v1.FavoriteAdded" {
		t.Fatalf("record key %q type %q", got.Key, envelopeOfRecord(t, got.Value).GetType())
	}
}
