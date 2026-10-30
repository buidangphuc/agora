package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-search/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-search/generated/platform/listing/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/events"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/taxonomy"
)

const (
	idxTopic    = "listing.events"
	idxDLQTopic = "listing.events.dlq"
	idxWait     = 60 * time.Second
	fpSecret    = "indexer-test-fp-secret"
)

var twelveHex = regexp.MustCompile(`^[0-9a-f]{12}$`)

type logBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *logBuf) lines() []map[string]any {
	var out []map[string]any
	for _, l := range strings.Split(b.String(), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func (b *logBuf) find(msg string) []map[string]any {
	var out []map[string]any
	for _, m := range b.lines() {
		if s, _ := m["msg"].(string); strings.Contains(s, msg) {
			out = append(out, m)
		}
	}
	return out
}

func testLogger(w io.Writer) *slog.Logger {
	return logx.NewTo(w, sdkconfig.Runtime{Env: "local", LogLevel: "info", LogJSON: true}, "team-search-indexer")
}

type failingDLQ struct{ calls int }

func (f *failingDLQ) PublishRecord(context.Context, *kgo.Record) error {
	f.calls++
	return errors.New("dlq broker down")
}

type countingStockIndex struct {
	index.Index
	mu    sync.Mutex
	calls int
}

func (c *countingStockIndex) UpdateStock(context.Context, string, int32, int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return nil
}

func (c *countingStockIndex) updates() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type handlerProbe struct {
	mu    sync.Mutex
	calls int
	recs  chan consume.Record
}

func newProbe() *handlerProbe { return &handlerProbe{recs: make(chan consume.Record, 16)} }

func (p *handlerProbe) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *handlerProbe) hit() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.calls
}

func envelope(t *testing.T, typ string, msg proto.Message) []byte {
	t.Helper()
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId: "evt-1", Type: typ, Payload: payload, OccurredAt: timestamppb.New(time.Unix(1700000000, 0)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func negativeStockEvent(t *testing.T) []byte {
	return envelope(t, "platform.listing.v1.ListingStockChanged", &listingv1.ListingStockChanged{ListingId: "listing-1", Stock: -1})
}

func produceRecord(t *testing.T, brokers, topic string, key, value []byte) {
	t.Helper()
	c, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.ProduceSync(ctx, &kgo.Record{Topic: topic, Key: key, Value: value}).FirstErr(); err != nil {
		t.Fatalf("produce: %v", err)
	}
}

func groupClient(t *testing.T, brokers, group string) *kgo.Client {
	t.Helper()
	c, err := kafkax.NewConsumerClient(sdkconfig.Kafka{Brokers: brokers}, group, []string{idxTopic}, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func fingerprinter(t *testing.T) *kafkax.Fingerprinter {
	t.Helper()
	fp, err := kafkax.NewFingerprinter(fpSecret)
	if err != nil {
		t.Fatal(err)
	}
	return fp
}

type runningConsumer struct {
	cancel context.CancelFunc
	done   chan error
	client *kgo.Client
}

func startConsumer(t *testing.T, brokers, group string, h consume.Handler, dlq consume.DLQProducer, logger *slog.Logger) *runningConsumer {
	t.Helper()
	client := groupClient(t, brokers, group)
	c, err := newIndexerConsumer(client, h, dlq, idxTopic, fingerprinter(t), logger)
	if err != nil {
		client.Close()
		t.Fatalf("newIndexerConsumer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &runningConsumer{cancel: cancel, done: make(chan error, 1), client: client}
	go func() { r.done <- c.Run(ctx) }()
	t.Cleanup(func() { r.stop(t) })
	return r
}

func (r *runningConsumer) stop(t *testing.T) error {
	t.Helper()
	r.cancel()
	select {
	case err := <-r.done:
		r.done <- err
		r.client.Close()
		return err
	case <-time.After(idxWait):
		t.Error("consumer did not stop after cancel")
		return errors.New("consumer did not stop")
	}
}

func (r *runningConsumer) waitErr(t *testing.T) error {
	t.Helper()
	select {
	case err := <-r.done:
		r.done <- err
		return err
	case <-time.After(idxWait):
		t.Fatal("consumer did not return")
		return nil
	}
}

func recordingHandler(p *handlerProbe) consume.Handler {
	return func(_ context.Context, r consume.Record) error {
		p.hit()
		p.recs <- r
		return nil
	}
}

// Scenario: A malformed stock event is retried and parked.
func TestIndexerParksMalformedStockEvent(t *testing.T) {
	brokers := testkit.Redpanda(t)
	value := negativeStockEvent(t)
	produceRecord(t, brokers, idxTopic, []byte("listing-1"), value)

	idx := &countingStockIndex{}
	inner := events.NewListingEventHandler(idx, nil, &taxonomy.MockClassifier{})
	probe := newProbe()
	handler := func(ctx context.Context, r consume.Record) error {
		probe.hit()
		return inner(ctx, r)
	}
	dlq, err := kafkax.NewProducer(sdkconfig.Kafka{Brokers: brokers}, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dlq.Close)

	r := startConsumer(t, brokers, "park-group", handler, dlq, testLogger(io.Discard))

	got := testkit.ReadAll(t, brokers, idxDLQTopic, 1, idxWait)
	if len(got) != 1 {
		t.Fatalf("dlq records = %d, want 1", len(got))
	}
	if string(got[0].Key) != "listing-1" {
		t.Errorf("dlq key = %q, want listing-1", got[0].Key)
	}
	if !bytes.Equal(got[0].Value, value) {
		t.Error("dlq value differs from the original record value")
	}
	if n := probe.count(); n != 5 {
		t.Errorf("handler calls = %d, want 5", n)
	}
	if n := idx.updates(); n != 0 {
		t.Errorf("malformed stock event reached the index %d times", n)
	}
	if err := r.stop(t); err != nil {
		t.Errorf("Run after cancel = %v, want nil", err)
	}
}

// Scenario: A failing DLQ publish does not advance the offset.
func TestIndexerDLQFailureDoesNotAdvanceOffset(t *testing.T) {
	brokers := testkit.Redpanda(t)
	value := negativeStockEvent(t)
	produceRecord(t, brokers, idxTopic, []byte("listing-1"), value)

	dlq := &failingDLQ{}
	failing := func(context.Context, consume.Record) error { return errors.New("always fails") }
	first := startConsumer(t, brokers, "offset-group", failing, dlq, testLogger(io.Discard))
	if err := first.waitErr(t); err == nil {
		t.Fatal("Run returned nil although the dead-letter publish failed")
	}
	if dlq.calls == 0 {
		t.Fatal("the dead-letter producer was never called")
	}
	first.client.Close()

	probe := newProbe()
	second := startConsumer(t, brokers, "offset-group", recordingHandler(probe), &failingDLQ{}, testLogger(io.Discard))
	select {
	case rec := <-probe.recs:
		if string(rec.Key) != "listing-1" || !bytes.Equal(rec.Value, value) {
			t.Errorf("redelivered record = key %q, want the original listing-1 record", rec.Key)
		}
	case <-time.After(idxWait):
		t.Fatal("the record was not redelivered to a new consumer of the same group")
	}
	_ = second
}

// Scenario: A recoverable handler error does not park the record.
func TestIndexerRecoversBeforeDLQ(t *testing.T) {
	brokers := testkit.Redpanda(t)
	produceRecord(t, brokers, idxTopic, []byte("listing-r"), []byte("recoverable"))

	probe := newProbe()
	handler := func(_ context.Context, r consume.Record) error {
		if string(r.Value) == "recoverable" && probe.hit() <= 2 {
			return errors.New("transient failure")
		}
		if string(r.Value) != "recoverable" {
			probe.hit()
		}
		probe.recs <- r
		return nil
	}
	dlq := testkit.NewRecorder()
	first := startConsumer(t, brokers, "recover-group", handler, dlq, testLogger(io.Discard))
	select {
	case rec := <-probe.recs:
		if string(rec.Value) != "recoverable" {
			t.Fatalf("first successful record = %q", rec.Value)
		}
	case <-time.After(idxWait):
		t.Fatal("the record was never handled successfully")
	}
	produceRecord(t, brokers, idxTopic, []byte("listing-after"), []byte("after"))
	select {
	case rec := <-probe.recs:
		if string(rec.Value) != "after" {
			t.Fatalf("second record = %q", rec.Value)
		}
	case <-time.After(idxWait):
		t.Fatal("the record after the recovered one was not handled")
	}
	if err := first.stop(t); err != nil {
		t.Fatalf("Run after cancel = %v, want nil", err)
	}
	if n := len(dlq.Records()); n != 0 {
		t.Errorf("dead-letter records = %d, want 0", n)
	}
	if n := probe.count(); n < 3 {
		t.Errorf("handler calls for the recoverable record = %d, want at least 3", n)
	}

	probe2 := newProbe()
	startConsumer(t, brokers, "recover-group", recordingHandler(probe2), testkit.NewRecorder(), testLogger(io.Discard))
	produceRecord(t, brokers, idxTopic, []byte("listing-marker"), []byte("marker"))
	select {
	case rec := <-probe2.recs:
		if string(rec.Value) != "marker" {
			t.Errorf("a new consumer of the group first received %q, want only the marker: the offset was not committed", rec.Value)
		}
	case <-time.After(idxWait):
		t.Fatal("marker record not received")
	}
}

func waitForLine(t *testing.T, logs *logBuf, msg string) {
	t.Helper()
	testkit.Eventually(t, idxWait, func() bool { return len(logs.find(msg)) > 0 })
}

// Scenario: Retry and park logs carry the fingerprint.
func TestIndexerLogsKeyFingerprintOnly(t *testing.T) {
	brokers := testkit.Redpanda(t)
	const key = "listing-123"
	produceRecord(t, brokers, idxTopic, []byte(key), []byte("payload"))

	logs := &logBuf{}
	failing := func(_ context.Context, r consume.Record) error {
		return fmt.Errorf("index write failed for %s", r.Key)
	}
	dlq := testkit.NewRecorder()
	r := startConsumer(t, brokers, "fp-group", failing, dlq, testLogger(logs))
	waitForLine(t, logs, "dead-lettered")
	if err := r.stop(t); err != nil {
		t.Fatalf("Run after cancel = %v, want nil", err)
	}

	if all := logs.String(); strings.Contains(all, key) {
		t.Errorf("a log line contains the raw record key %q:\n%s", key, all)
	}
	want := fingerprinter(t).Of([]byte(key))
	park := logs.find("dead-lettered")
	if len(park) != 1 {
		t.Fatalf("park log lines = %d, want 1", len(park))
	}
	if got := park[0]["key_fp"]; got != want {
		t.Errorf("park line key_fp = %v, want %s", got, want)
	}
	retries := logs.find("handler failed")
	if len(retries) == 0 {
		t.Fatal("no retry log line")
	}
	for _, l := range retries {
		if l["key_fp"] != want {
			t.Errorf("retry line key_fp = %v, want %s", l["key_fp"], want)
		}
	}
}

// Scenario: An empty key is still fingerprinted.
func TestIndexerEmptyKeyIsFingerprinted(t *testing.T) {
	brokers := testkit.Redpanda(t)
	produceRecord(t, brokers, idxTopic, nil, []byte("payload"))

	logs := &logBuf{}
	failing := func(context.Context, consume.Record) error { return errors.New("always fails") }
	r := startConsumer(t, brokers, "empty-key-group", failing, testkit.NewRecorder(), testLogger(logs))
	waitForLine(t, logs, "handler failed")
	if err := r.stop(t); err != nil {
		t.Fatalf("Run after cancel = %v, want nil", err)
	}

	line := logs.find("handler failed")[0]
	got, _ := line["key_fp"].(string)
	if !twelveHex.MatchString(got) {
		t.Fatalf("key_fp = %q, want 12 lowercase hex characters", got)
	}
	if want := fingerprinter(t).Of(nil); got != want {
		t.Errorf("key_fp = %s, want %s", got, want)
	}
}

type indexStub struct {
	mu      sync.Mutex
	updates map[string]string
	ensured bool
}

func (s *indexStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/":
		_, _ = io.WriteString(w, `{"version":{"number":"2.17.1"}}`)
	case r.Method == http.MethodHead && r.URL.Path == "/listings":
		s.mu.Lock()
		s.ensured = true
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPut && r.URL.Path == "/listings/_mapping":
		_, _ = io.WriteString(w, `{"acknowledged":true}`)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/listings/_update/"):
		raw, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.updates[strings.TrimPrefix(r.URL.Path, "/listings/_update/")] = string(raw)
		s.mu.Unlock()
		_, _ = io.WriteString(w, `{"result":"created"}`)
	default:
		http.NotFound(w, r)
	}
}

func (s *indexStub) update(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.updates[id]
	return v, ok
}

func (s *indexStub) isEnsured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensured
}

func indexerSettings(brokers, osURL string) config.IndexerSettings {
	return config.IndexerSettings{
		Runtime:       sdkconfig.Runtime{Env: "local", LogLevel: "info", LogJSON: true},
		OpenSearch:    config.OpenSearch{URL: osURL, Index: "listings"},
		Retrieval:     config.Retrieval{ModelServerURL: "http://127.0.0.1:1"},
		Taxonomy:      config.Taxonomy{AIAddr: "127.0.0.1:1"},
		Kafka:         sdkconfig.Kafka{Brokers: brokers},
		Events:        config.Events{ConsumerGroup: "run-indexer-group", ListingTopic: idxTopic},
		LogKeyFP:      sdkconfig.LogKeyFP{Secret: fpSecret},
		Tombstone:     config.Tombstone{TTL: time.Hour, PurgeInterval: time.Hour},
		ShutdownGrace: 5 * time.Second,
	}
}

func startRunIndexer(t *testing.T, s config.IndexerSettings) (cancel context.CancelFunc, done chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done = make(chan error, 1)
	go func() { done <- RunIndexer(ctx, s) }()
	t.Cleanup(cancel)
	return cancel, done
}

// Scenario: The indexer stops when cancelled.
func TestRunIndexerStopsOnCancel(t *testing.T) {
	brokers := testkit.Redpanda(t)
	stub := &indexStub{updates: map[string]string{}}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	s := indexerSettings(brokers, srv.URL)

	cancel, done := startRunIndexer(t, s)
	testkit.Eventually(t, idxWait, stub.isEnsured)
	select {
	case err := <-done:
		t.Fatalf("RunIndexer returned before cancel: %v", err)
	default:
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunIndexer after cancel = %v, want nil", err)
		}
	case <-time.After(s.ShutdownGrace + 5*time.Second):
		t.Fatal("RunIndexer did not return within SHUTDOWN_GRACE")
	}
}

// Scenario: A classifier outage does not fail ingestion (wiring level: the
// indexer starts with an unreachable team-ai and still writes the document with
// tags_pending).
func TestRunIndexerIngestsWhileClassifierIsDown(t *testing.T) {
	brokers := testkit.Redpanda(t)
	stub := &indexStub{updates: map[string]string{}}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	s := indexerSettings(brokers, srv.URL)

	cancel, done := startRunIndexer(t, s)
	testkit.Eventually(t, idxWait, stub.isEnsured)

	ev := envelope(t, "platform.listing.v1.ListingChanged", &listingv1.ListingChanged{
		ChangeType: listingv1.ChangeType_CHANGE_TYPE_CREATED,
		Listing: &listingv1.Listing{
			Id: "l-outage", Title: "Tai nghe Bluetooth", Description: "ANC", CategoryId: "cat-electronics",
			Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED,
		},
	})
	produceRecord(t, brokers, idxTopic, []byte("l-outage"), ev)

	var body string
	testkit.Eventually(t, idxWait, func() bool {
		var ok bool
		body, ok = stub.update("l-outage")
		return ok
	})
	if !strings.Contains(body, `"tags_pending":true`) {
		t.Errorf("document was written without tags_pending: %s", body)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunIndexer after cancel = %v, want nil", err)
		}
	case <-time.After(s.ShutdownGrace + 5*time.Second):
		t.Fatal("RunIndexer did not stop")
	}
}
