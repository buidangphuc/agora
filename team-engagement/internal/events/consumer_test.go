package events_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-engagement/internal/events"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

const (
	listingTopic = "listing.events"
	listingDLQ   = "listing.events.dlq"
	testSecret   = "engagement-test-secret"
	waitFor      = 60 * time.Second
)

type countingStore struct {
	*storetest.FakeRepository
	calls  atomic.Int32
	failID string
	failAt atomic.Bool
	msg    string
}

func newCountingStore(t *testing.T, failAll bool, failID, msg string) *countingStore {
	t.Helper()
	s := &countingStore{FakeRepository: storetest.NewFakeRepository(), failID: failID, msg: msg}
	s.failAt.Store(failAll)
	return s
}

func (s *countingStore) fails(id string) bool {
	s.calls.Add(1)
	return s.failAt.Load() || (s.failID != "" && s.failID == id)
}

func (s *countingStore) UpsertSellerListing(ctx context.Context, sellerID, listingID string, at time.Time) error {
	if s.fails(listingID) {
		return errors.New(s.msg)
	}
	return s.FakeRepository.UpsertSellerListing(ctx, sellerID, listingID, at)
}

func (s *countingStore) RemoveSellerListing(ctx context.Context, listingID string) error {
	if s.fails(listingID) {
		return errors.New(s.msg)
	}
	return s.FakeRepository.RemoveSellerListing(ctx, listingID)
}

type listingRun struct {
	logs *syncBuffer
	fp   *kafkax.Fingerprinter
	done chan error
	stop func()
}

func (r *listingRun) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-r.done:
		return err
	case <-time.After(waitFor):
		t.Fatal("consumer did not stop")
		return nil
	}
}

func startListing(t *testing.T, brokers, group, secret string, store events.FeedStore, dlq consume.DLQProducer, wrap func(consume.Handler) consume.Handler) *listingRun {
	t.Helper()
	run := &listingRun{logs: &syncBuffer{}, done: make(chan error, 1)}
	fp, err := kafkax.NewFingerprinter(secret)
	if err != nil {
		t.Fatal(err)
	}
	run.fp = fp
	logger := slog.New(slog.NewJSONHandler(run.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client, err := events.NewListingClient(config.Kafka{Brokers: brokers}, group, listingTopic)
	if err != nil {
		t.Fatalf("listing client: %v", err)
	}
	var c *consume.Consumer
	if wrap == nil {
		c, err = events.NewListingConsumer(client, store, dlq, listingTopic, fp, logger)
	} else {
		c, err = consume.NewConsumer(client, wrap(events.ListingEventHandler(store)), dlq, consume.Options{
			DLQTopic: listingDLQ, MaxAttempts: 5, BaseBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
			Logger: logger, KeyFP: fp,
		})
	}
	if err != nil {
		client.Close()
		t.Fatalf("listing consumer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { run.done <- c.Run(ctx) }()
	stopped := false
	run.stop = func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		select {
		case <-run.done:
		case <-time.After(30 * time.Second):
			t.Error("consumer did not stop")
		}
		client.Close()
	}
	t.Cleanup(run.stop)
	return run
}

func publishedFor(t *testing.T, id string) []byte {
	t.Helper()
	return changed(t, id, "seller-1", published, created, t0)
}

func plainFP(key string) string {
	s := sha256.Sum256([]byte(key))
	return hex.EncodeToString(s[:])[:12]
}

// Scenario: Engagement SDK: Undecodable record is dead-lettered after one attempt
func TestUndecodableRecordGoesStraightToDLQ(t *testing.T) {
	brokers := testkit.Redpanda(t)
	poison := []byte("not a proto \xff\xff")
	testkit.Produce(t, brokers, listingTopic, "k-poison", poison)
	testkit.Produce(t, brokers, listingTopic, "k-good", publishedFor(t, "l-after"))

	store := newCountingStore(t, false, "", "")
	if _, err := store.Follow(context.Background(), "buyer", "seller-1"); err != nil {
		t.Fatal(err)
	}
	dlq := testkit.NewRecorder()
	var attempts atomic.Int32
	startListing(t, brokers, "g-undecodable", testSecret, store, dlq, func(inner consume.Handler) consume.Handler {
		return func(ctx context.Context, r consume.Record) error {
			if string(r.Key) == "k-poison" {
				attempts.Add(1)
			}
			return inner(ctx, r)
		}
	})

	testkit.Eventually(t, waitFor, func() bool {
		ids, _, _, _ := store.ListFollowedListings(context.Background(), "buyer", "", 50)
		return len(ids) == 1
	})
	if got := attempts.Load(); got != 1 {
		t.Fatalf("handler attempts for the undecodable record = %d, want 1", got)
	}
	recs := dlq.Records()
	if len(recs) != 1 {
		t.Fatalf("dead-lettered %d records, want 1", len(recs))
	}
	if recs[0].Topic != listingDLQ || string(recs[0].Key) != "k-poison" || string(recs[0].Value) != string(poison) {
		t.Fatalf("dlq record = topic %q key %q value %q", recs[0].Topic, recs[0].Key, recs[0].Value)
	}
}

// Scenario: Engagement SDK: Failing record is parked after five attempts
func TestFailingRecordParkedAfterFiveAttempts(t *testing.T) {
	brokers := testkit.Redpanda(t)
	bad := publishedFor(t, "l-bad")
	testkit.Produce(t, brokers, listingTopic, "k-bad", bad)
	testkit.Produce(t, brokers, listingTopic, "k-good", publishedFor(t, "l-good"))

	store := newCountingStore(t, false, "l-bad", "store down")
	if _, err := store.Follow(context.Background(), "buyer", "seller-1"); err != nil {
		t.Fatal(err)
	}
	dlq := testkit.NewRecorder()
	startListing(t, brokers, "g-five", testSecret, store, dlq, nil)

	testkit.Eventually(t, waitFor, func() bool {
		ids, _, _, _ := store.ListFollowedListings(context.Background(), "buyer", "", 50)
		return len(ids) == 1 && ids[0] == "l-good"
	})
	if got := store.calls.Load(); got != 6 {
		t.Fatalf("store calls = %d, want 5 failing attempts plus the good record", got)
	}
	recs := dlq.Records()
	if len(recs) != 1 || recs[0].Topic != listingDLQ || string(recs[0].Key) != "k-bad" || string(recs[0].Value) != string(bad) {
		t.Fatalf("dlq = %+v, want the failing record only", recs)
	}
}

// Scenario: Engagement SDK: A new group backfills from the start
func TestNewGroupBackfillsFromEarliest(t *testing.T) {
	brokers := testkit.Redpanda(t)
	for _, id := range []string{"l1", "l2", "l3"} {
		testkit.Produce(t, brokers, listingTopic, "seller-1", publishedFor(t, id))
	}
	store := newCountingStore(t, false, "", "")
	if _, err := store.Follow(context.Background(), "buyer", "seller-1"); err != nil {
		t.Fatal(err)
	}
	startListing(t, brokers, "g-backfill-new", testSecret, store, testkit.NewRecorder(), nil)

	testkit.Eventually(t, waitFor, func() bool {
		ids, _, total, _ := store.ListFollowedListings(context.Background(), "buyer", "", 50)
		return len(ids) == 3 && total == 3
	})
}

func failingRecord(t *testing.T, brokers, key string) {
	t.Helper()
	testkit.Produce(t, brokers, listingTopic, key, publishedFor(t, "l-"+key[:min(len(key), 6)]))
}

func awaitDeadLettered(t *testing.T, run *listingRun) {
	t.Helper()
	testkit.Eventually(t, waitFor, func() bool {
		return strings.Contains(run.logs.String(), "consume: record dead-lettered")
	})
}

// Scenario: Engagement key redaction: handler failure logs fingerprint
func TestHandlerFailureLogsKeyFingerprint(t *testing.T) {
	brokers := testkit.Redpanda(t)
	failingRecord(t, brokers, "seller-123")
	run := startListing(t, brokers, "g-fail-fp", testSecret, newCountingStore(t, true, "", "upsert for seller-123 failed"), testkit.NewRecorder(), nil)
	awaitDeadLettered(t, run)

	recs := run.logs.find(t, "consume: handler failed")
	if len(recs) == 0 {
		t.Fatalf("no 'consume: handler failed' record: %s", run.logs.String())
	}
	want := run.fp.Of([]byte("seller-123"))
	for _, r := range recs {
		if r["key_fp"] != want {
			t.Fatalf("key_fp = %v, want %s", r["key_fp"], want)
		}
		if _, ok := r["key"]; ok {
			t.Fatalf("raw key attribute present: %v", r)
		}
	}
	if strings.Contains(run.logs.String(), "seller-123") {
		t.Fatalf("raw key leaked in log output: %s", run.logs.String())
	}
}

// Scenario: Engagement key redaction: DLQ parking logs fingerprint
func TestDLQParkLogsFingerprintKeepsDLQKey(t *testing.T) {
	brokers := testkit.Redpanda(t)
	failingRecord(t, brokers, "seller-123")
	dlq := testkit.NewRecorder()
	run := startListing(t, brokers, "g-park-fp", testSecret, newCountingStore(t, true, "", "upsert for seller-123 failed"), dlq, nil)
	awaitDeadLettered(t, run)

	recs := run.logs.find(t, "consume: record dead-lettered")
	if len(recs) != 1 {
		t.Fatalf("want one parking record, got %d: %s", len(recs), run.logs.String())
	}
	if recs[0]["key_fp"] != run.fp.Of([]byte("seller-123")) {
		t.Fatalf("key_fp = %v", recs[0]["key_fp"])
	}
	if _, ok := recs[0]["key"]; ok {
		t.Fatalf("raw key attribute present")
	}
	if strings.Contains(run.logs.String(), "seller-123") {
		t.Fatalf("raw key leaked: %s", run.logs.String())
	}
	got := dlq.Records()
	if len(got) != 1 || string(got[0].Key) != "seller-123" {
		t.Fatalf("dlq records = %+v, want one with the raw key", got)
	}
}

// Scenario: Engagement key redaction: DLQ produce failure logs fingerprint
func TestDLQProduceFailureLogsFingerprintOffsetHeld(t *testing.T) {
	brokers := testkit.Redpanda(t)
	failingRecord(t, brokers, "seller-123")
	broken := testkit.NewRecorder()
	broken.FailOn("seller-123")
	run := startListing(t, brokers, "g-dlq-down", testSecret, newCountingStore(t, true, "", "upsert for seller-123 failed"), broken, nil)

	err := run.wait(t)
	if err == nil {
		t.Fatal("consumer must return an error when the record can be neither handled nor parked")
	}
	if strings.Contains(err.Error(), "seller-123") {
		t.Fatalf("returned error leaks the key: %v", err)
	}
	recs := run.logs.find(t, "consume: dead-letter failed")
	if len(recs) != 1 {
		t.Fatalf("want one 'consume: dead-letter failed' record, got %d: %s", len(recs), run.logs.String())
	}
	if recs[0]["key_fp"] != run.fp.Of([]byte("seller-123")) {
		t.Fatalf("key_fp = %v", recs[0]["key_fp"])
	}
	if strings.Contains(run.logs.String(), "seller-123") {
		t.Fatalf("raw key leaked: %s", run.logs.String())
	}
	run.stop()

	healthy := testkit.NewRecorder()
	startListing(t, brokers, "g-dlq-down", testSecret, newCountingStore(t, true, "", "still failing"), healthy, nil)
	testkit.Eventually(t, waitFor, func() bool { return len(healthy.Records()) == 1 })
	if string(healthy.Records()[0].Key) != "seller-123" {
		t.Fatalf("the held record must be redelivered to the group, got key %q", healthy.Records()[0].Key)
	}
}

// Scenario: Engagement key redaction: empty key
func TestEmptyKeyFingerprint(t *testing.T) {
	brokers := testkit.Redpanda(t)
	testkit.Produce(t, brokers, listingTopic, "", publishedFor(t, "l-empty"))
	run := startListing(t, brokers, "g-empty-key", testSecret, newCountingStore(t, true, "", "boom"), testkit.NewRecorder(), nil)
	awaitDeadLettered(t, run)

	recs := run.logs.find(t, "consume: handler failed")
	if len(recs) == 0 {
		t.Fatalf("no record: %s", run.logs.String())
	}
	want := run.fp.Of(nil)
	if want == "e3b0c44298fc" {
		t.Fatal("test secret produced the unkeyed digest")
	}
	if recs[0]["key_fp"] != want {
		t.Fatalf("key_fp = %v, want %s", recs[0]["key_fp"], want)
	}
}

// Scenario: Engagement key redaction: fingerprint is exactly 12 hex and stable
func TestKeyFingerprintFormatStable(t *testing.T) {
	brokers := testkit.Redpanda(t)
	long := strings.Repeat("k", 200)
	for i, key := range []string{"seller-123", "seller-123", "seller-456", long} {
		testkit.Produce(t, brokers, listingTopic, key, publishedFor(t, "l-fmt-"+string(rune('a'+i))))
	}
	dlq := testkit.NewRecorder()
	run := startListing(t, brokers, "g-fmt", testSecret, newCountingStore(t, true, "", "boom"), dlq, func(h consume.Handler) consume.Handler { return h })
	testkit.Eventually(t, waitFor, func() bool { return len(dlq.Records()) == 4 })

	byOffset := map[float64]any{}
	for _, r := range run.logs.find(t, "consume: handler failed") {
		if r["attempt"] == float64(1) {
			byOffset[r["offset"].(float64)] = r["key_fp"]
		}
	}
	if len(byOffset) != 4 {
		t.Fatalf("want a first-attempt failure for each of 4 records, got %v", byOffset)
	}
	a1, a2, b, l := byOffset[0], byOffset[1], byOffset[2], byOffset[3]
	if a1 != a2 {
		t.Fatalf("not stable: %v vs %v", a1, a2)
	}
	if a1 == b {
		t.Fatalf("different keys share fingerprint %v", a1)
	}
	re := regexp.MustCompile(`^[0-9a-f]{12}$`)
	for _, v := range []any{a1, b, l} {
		s, _ := v.(string)
		if !re.MatchString(s) {
			t.Fatalf("key_fp %v not 12 lowercase hex", v)
		}
	}
}

// Scenario: Engagement key redaction: fingerprint depends on the secret
func TestKeyFingerprintIsKeyedNotPlainHash(t *testing.T) {
	brokers := testkit.Redpanda(t)
	failingRecord(t, brokers, "seller-123")
	fpUnder := func(group, secret string) string {
		run := startListing(t, brokers, group, secret, newCountingStore(t, true, "", "boom"), testkit.NewRecorder(), nil)
		awaitDeadLettered(t, run)
		recs := run.logs.find(t, "consume: handler failed")
		if len(recs) == 0 {
			t.Fatalf("no failure record under %s", secret)
		}
		s, _ := recs[0]["key_fp"].(string)
		run.stop()
		return s
	}
	a := fpUnder("g-secret-a", "secret-A")
	b := fpUnder("g-secret-b", "secret-B")
	if a == b {
		t.Fatalf("fingerprints under different secrets are equal: %s", a)
	}
	if a == plainFP("seller-123") || b == plainFP("seller-123") {
		t.Fatalf("fingerprint equals the unkeyed SHA-256 prefix: %s / %s", a, b)
	}
}

// Scenario: Engagement key redaction: successful records and key unchanged
func TestSuccessPassesRawKeyToHandler(t *testing.T) {
	brokers := testkit.Redpanda(t)
	failingRecord(t, brokers, "seller-123")
	store := newCountingStore(t, false, "", "")
	if _, err := store.Follow(context.Background(), "buyer", "seller-1"); err != nil {
		t.Fatal(err)
	}
	var gotKey atomic.Value
	run := startListing(t, brokers, "g-success", testSecret, store, testkit.NewRecorder(), func(inner consume.Handler) consume.Handler {
		return func(ctx context.Context, r consume.Record) error {
			gotKey.Store(string(r.Key))
			return inner(ctx, r)
		}
	})
	testkit.Eventually(t, waitFor, func() bool {
		ids, _, _, _ := store.ListFollowedListings(context.Background(), "buyer", "", 50)
		return len(ids) == 1
	})
	if gotKey.Load() != "seller-123" {
		t.Fatalf("handler key = %v, want seller-123", gotKey.Load())
	}
	run.stop()
	for _, r := range run.logs.records(t) {
		if lvl, _ := r["level"].(string); lvl == "WARN" || lvl == "ERROR" {
			t.Fatalf("unexpected failure log on success: %v", r)
		}
	}
}
