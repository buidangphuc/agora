package ga4relay

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-analytics/generated/platform/order/v1"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

// Required interface (task 1.6, design D7):
//
//	var ErrDeadLetter error
//	type RelayConfig struct { Mappings MappingStore; Ledger Ledger; Sender Sender; Now func() time.Time; Logger *slog.Logger; KeyFP *kafkax.Fingerprinter }
//	func (r *Relay) Handle(ctx context.Context, rec consume.Record) error
//	    nil: sent, skipped, ignored or already sent
//	    consume.Permanent(fmt.Errorf("%w: ...", ErrDeadLetter)): invalid envelope/order id, SendError (4xx, exhausted)
//	    any other error: transient (ledger, mapping store); counted as failed{store} once per attempt
//	func (h *MappingHandler) Handle(ctx context.Context, rec consume.Record) error
//	func ConsumerOptions(s config.Settings) consume.Options
//
// Message, DLQProducer, OffsetCommitter, RelayConfig.DLQ and RelayConfig.Committer are removed.

type fakeSender struct {
	mu     sync.Mutex
	bodies [][]byte
	errs   []error // consumed per call; nil/empty => success
	delay  time.Duration
}

func (s *fakeSender) Send(_ context.Context, body []byte) error {
	if s.delay > 0 {
		time.Sleep(s.delay) // widens the race window for the concurrency test only
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bodies = append(s.bodies, append([]byte(nil), body...))
	if len(s.errs) > 0 {
		e := s.errs[0]
		s.errs = s.errs[1:]
		return e
	}
	return nil
}
func (s *fakeSender) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.bodies) }

type rig struct {
	r      *Relay
	snd    *fakeSender
	maps   MappingStore
	ledger Ledger
	clk    *fakeClock
	logs   *bytes.Buffer
	fp     *kafkax.Fingerprinter
}

func newRig(t *testing.T) *rig {
	t.Helper()
	clk := &fakeClock{t: time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)}
	fp, err := kafkax.NewFingerprinter("s1")
	if err != nil {
		t.Fatal(err)
	}
	g := &rig{snd: &fakeSender{}, clk: clk, logs: &bytes.Buffer{}, fp: fp,
		maps: NewMemoryMappingStore(clk.Now), ledger: NewMemoryLedger(clk.Now)}
	g.rebuild()
	return g
}

func (g *rig) rebuild() {
	lg := logx.NewTo(g.logs, sdkconfig.Runtime{Env: "test", LogLevel: "debug", LogJSON: true}, "test")
	g.r = NewRelay(RelayConfig{Mappings: g.maps, Ledger: g.ledger, Sender: g.snd, Now: g.clk.Now, Logger: lg, KeyFP: g.fp})
}

func (g *rig) mapBuyer(t *testing.T, buyer, client string) {
	t.Helper()
	if err := g.maps.Put(context.Background(), buyer, client); err != nil {
		t.Fatal(err)
	}
}

func item(listing, variant string, qty int32, price int64) *orderv1.OrderLineItemFact {
	return &orderv1.OrderLineItemFact{ListingId: listing, VariantId: variant, SellerId: "s1", Quantity: qty, UnitPrice: price, Currency: "VND"}
}

func paid(orderID, buyer string, total int64, paidAt *time.Time, items ...*orderv1.OrderLineItemFact) *orderv1.OrderPaidEvent {
	e := &orderv1.OrderPaidEvent{OrderId: orderID, BuyerId: buyer, TotalAmount: total, Currency: "VND", Items: items}
	if paidAt != nil {
		e.PaidAt = timestamppb.New(*paidAt)
	}
	return e
}

func envMsg(t *testing.T, typ string, payload []byte, offset int64) consume.Record {
	t.Helper()
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "ev", Type: typ, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return consume.Record{Topic: "order.events", Offset: offset, Key: []byte("k"), Value: b}
}

func paidMsg(t *testing.T, e *orderv1.OrderPaidEvent, offset int64) consume.Record {
	t.Helper()
	p, err := proto.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return envMsg(t, "platform.order.v1.OrderPaidEvent", p, offset)
}

type mpBody struct {
	ClientID        string          `json:"client_id"`
	TimestampMicros *int64          `json:"timestamp_micros"`
	Events          []mpEvent       `json:"events"`
	Raw             json.RawMessage `json:"-"`
}
type mpEvent struct {
	Name   string `json:"name"`
	Params struct {
		TransactionID string  `json:"transaction_id"`
		Currency      string  `json:"currency"`
		Value         float64 `json:"value"`
		Items         []struct {
			ItemID   string  `json:"item_id"`
			Quantity int     `json:"quantity"`
			Price    float64 `json:"price"`
			Currency string  `json:"currency"`
		} `json:"items"`
	} `json:"params"`
}

func decode(t *testing.T, b []byte) mpBody {
	t.Helper()
	var m mpBody
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body is not JSON: %v: %s", err, b)
	}
	return m
}

func (g *rig) handle(t *testing.T, m consume.Record) {
	t.Helper()
	if err := g.r.Handle(context.Background(), m); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

func TestRelay_PaidOrderIsRelayed(t *testing.T) { // Scenario: Paid order is relayed
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	at := time.Date(2026, 10, 12, 11, 59, 0, 0, time.UTC)
	g.handle(t, paidMsg(t, paid("o1", "b1", 500000, &at, item("l1", "v1", 2, 200000), item("l2", "", 1, 90000)), 1))
	if g.snd.count() != 1 {
		t.Fatalf("requests = %d, want 1", g.snd.count())
	}
	b := decode(t, g.snd.bodies[0])
	if b.ClientID != "222.2" || len(b.Events) != 1 || b.Events[0].Name != "purchase" {
		t.Fatalf("bad body: %+v", b)
	}
	p := b.Events[0].Params
	if p.TransactionID != "o1" || p.Currency != "VND" || p.Value != 500000 || len(p.Items) != 2 {
		t.Fatalf("bad params: %+v", p)
	}
	if b.TimestampMicros == nil || *b.TimestampMicros != at.UnixMicro() {
		t.Fatalf("timestamp_micros = %v, want %d", b.TimestampMicros, at.UnixMicro())
	}
	if ok, _ := g.ledger.Sent(context.Background(), "o1"); !ok {
		t.Fatal("o1 not in ledger")
	}
}

func TestRelay_ItemIDsAndValueMapping(t *testing.T) { // Scenario: Item ids and value mapping
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	g.handle(t, paidMsg(t, paid("o1", "b1", 500000, nil, item("l1", "v1", 2, 200000), item("l2", "", 1, 90000)), 1))
	p := decode(t, g.snd.bodies[0]).Events[0].Params
	if p.Value != 500000 {
		t.Fatalf("value = %v, want 500000 (not item sum 490000)", p.Value)
	}
	want := []struct {
		id string
		q  int
		pr float64
	}{{"l1:v1", 2, 200000}, {"l2", 1, 90000}}
	for i, w := range want {
		it := p.Items[i]
		if it.Currency != "VND" {
			t.Fatalf("item %d currency = %q, want VND", i, it.Currency)
		}
		if it.ItemID != w.id || it.Quantity != w.q || it.Price != w.pr {
			t.Fatalf("item %d = %+v, want %+v", i, it, w)
		}
	}
}

func TestRelay_PaidAtNil(t *testing.T) { // Scenario: Paid_at is nil
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	g.handle(t, paidMsg(t, paid("o1", "b1", 100, nil, item("l1", "", 1, 100)), 1))
	if g.snd.count() != 1 {
		t.Fatalf("requests = %d, want 1", g.snd.count())
	}
	if strings.Contains(string(g.snd.bodies[0]), "timestamp_micros") {
		t.Fatalf("timestamp_micros must be absent: %s", g.snd.bodies[0])
	}
	if g.r.PaidAtMissingCount() != 1 {
		t.Fatalf("paid_at_missing = %d, want 1", g.r.PaidAtMissingCount())
	}
}

func TestRelay_MalformedEventRejected(t *testing.T) { // Scenarios: Malformed order event is rejected; An invalid envelope is permanent
	cases := map[string]consume.Record{}
	cases["empty order_id"] = paidMsg(t, paid("", "b1", 100, nil, item("l1", "", 1, 100)), 1)
	cases["undecodable payload"] = envMsg(t, "platform.order.v1.OrderPaidEvent", []byte{0xff, 0xff, 0xff, 0x01}, 2)
	cases["undecodable envelope"] = consume.Record{Topic: "order.events", Offset: 3, Value: []byte{0xff, 0xff, 0xff, 0x01}}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			g := newRig(t)
			g.mapBuyer(t, "b1", "222.2")
			err := g.r.Handle(context.Background(), m)
			if !errors.Is(err, ErrDeadLetter) {
				t.Fatalf("err = %v, want wrapping ErrDeadLetter", err)
			}
			if g.snd.count() != 0 {
				t.Fatalf("requests = %d, want 0", g.snd.count())
			}
			if g.r.FailedCount("invalid") != 1 {
				t.Fatalf("failed{invalid} = %d, want 1", g.r.FailedCount("invalid"))
			}
		})
	}
}

func TestHandleInvalidEnvelopeIsPermanent(t *testing.T) { // Scenario: An invalid envelope is permanent
	g := newRig(t)
	err := g.r.Handle(context.Background(), consume.Record{Topic: "order.events", Key: []byte("k"), Value: []byte{0xff, 0xff, 0xff, 0x01}})
	if !errors.Is(err, ErrDeadLetter) {
		t.Fatalf("err = %v, want wrapping ErrDeadLetter", err)
	}
	if g.r.FailedCount("invalid") != 1 {
		t.Fatalf("failed{invalid} = %d, want 1", g.r.FailedCount("invalid"))
	}
}

func TestRelay_AnotherBuyersClientIDNeverUsed(t *testing.T) { // Scenario: Another buyer's client id is never used
	g := newRig(t)
	g.mapBuyer(t, "b1", "111.1")
	g.mapBuyer(t, "b2", "222.2")
	g.handle(t, paidMsg(t, paid("o2", "b2", 100, nil, item("l1", "", 1, 100)), 1))
	if g.snd.count() != 1 {
		t.Fatalf("requests = %d, want 1", g.snd.count())
	}
	if id := decode(t, g.snd.bodies[0]).ClientID; id != "222.2" {
		t.Fatalf("client_id = %q, want 222.2", id)
	}
	if strings.Contains(string(g.snd.bodies[0]), "111.1") {
		t.Fatal("b1's client id leaked into b2's request")
	}
}

func TestRelay_FailedSendNotRecordedAsSent(t *testing.T) { // Scenarios: A permanent 4xx is dead-lettered by the SDK; A redelivered order is sent once (replay after recovery)
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	g.snd.errs = []error{&SendError{Reason: "4xx", Status: 400, Attempts: 1}}
	m := paidMsg(t, paid("o3", "b1", 100, nil, item("l1", "", 1, 100)), 1)
	if err := g.r.Handle(context.Background(), m); !errors.Is(err, ErrDeadLetter) {
		t.Fatalf("err = %v, want wrapping ErrDeadLetter", err)
	}
	if ok, _ := g.ledger.Sent(context.Background(), "o3"); ok {
		t.Fatal("o3 recorded as sent after permanent failure")
	}
	if g.r.FailedCount("4xx") != 1 {
		t.Fatalf("failed{4xx} = %d, want 1", g.r.FailedCount("4xx"))
	}
	g.handle(t, m)
	if g.snd.count() != 2 {
		t.Fatalf("requests = %d, want 2 (1 failed + exactly 1 after recovery)", g.snd.count())
	}
	if ok, _ := g.ledger.Sent(context.Background(), "o3"); !ok {
		t.Fatal("o3 not in ledger after successful resend")
	}
}

func TestHandleSentOrderReturnsNil(t *testing.T) { // Scenario: A processed order returns nil
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	if err := g.r.Handle(context.Background(), paidMsg(t, paid("o1", "b1", 100, nil, item("l1", "", 1, 100)), 1)); err != nil {
		t.Fatalf("Handle = %v, want nil", err)
	}
	if ok, _ := g.ledger.Sent(context.Background(), "o1"); !ok {
		t.Fatal("o1 not in ledger")
	}
}

func TestHandle4xxIsPermanent(t *testing.T) { // Scenario: A permanent 4xx is dead-lettered by the SDK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) }))
	defer srv.Close()
	g := newRig(t)
	g.r = NewRelay(RelayConfig{Mappings: g.maps, Ledger: g.ledger, Now: g.clk.Now, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), KeyFP: g.fp,
		Sender: NewClient(ClientConfig{Endpoint: srv.URL, MeasurementID: "G-T", APISecret: "s3cret"})})
	g.mapBuyer(t, "b1", "222.2")
	err := g.r.Handle(context.Background(), paidMsg(t, paid("o2", "b1", 100, nil, item("l1", "", 1, 100)), 1))
	if !errors.Is(err, ErrDeadLetter) {
		t.Fatalf("err = %v, want wrapping ErrDeadLetter", err)
	}
	var perm interface{ Unwrap() error }
	if !errors.As(err, &perm) {
		t.Fatalf("err %T does not wrap a cause", err)
	}
	if g.r.FailedCount("4xx") != 1 {
		t.Fatalf("failed{4xx} = %d, want 1", g.r.FailedCount("4xx"))
	}
	if ok, _ := g.ledger.Sent(context.Background(), "o2"); ok {
		t.Fatal("o2 must not be in the ledger")
	}
}

func TestHandleExhaustedRetriesIsPermanent(t *testing.T) { // Scenario: Exhausted retries are permanent
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	g := newRig(t)
	noSleep := func(context.Context, time.Duration) error { return nil }
	g.r = NewRelay(RelayConfig{Mappings: g.maps, Ledger: g.ledger, Now: g.clk.Now, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), KeyFP: g.fp,
		Sender: NewClient(ClientConfig{Endpoint: srv.URL, MeasurementID: "G-T", APISecret: "s3cret", Sleep: noSleep})})
	g.mapBuyer(t, "b1", "222.2")
	err := g.r.Handle(context.Background(), paidMsg(t, paid("o5", "b1", 100, nil, item("l1", "", 1, 100)), 1))
	if !errors.Is(err, ErrDeadLetter) {
		t.Fatalf("err = %v, want wrapping ErrDeadLetter", err)
	}
	if hits.Load() != 5 {
		t.Fatalf("GA4 attempts = %d, want 5", hits.Load())
	}
	if g.r.FailedCount("exhausted") != 1 {
		t.Fatalf("failed{exhausted} = %d, want 1", g.r.FailedCount("exhausted"))
	}
}

func TestRelay_ExhaustedGoesToDLQ(t *testing.T) { // Scenario: Exhausted retries are permanent
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	g.snd.errs = []error{&SendError{Reason: "exhausted", Status: 503, Attempts: 5}}
	err := g.r.Handle(context.Background(), paidMsg(t, paid("o3", "b1", 100, nil, item("l1", "", 1, 100)), 1))
	if !errors.Is(err, ErrDeadLetter) || g.r.FailedCount("exhausted") != 1 {
		t.Fatalf("err=%v exhausted=%d; want ErrDeadLetter,1", err, g.r.FailedCount("exhausted"))
	}
}

func TestRelay_PayloadCarriesNoPersonalData(t *testing.T) { // Scenario: Payload carries no personal data
	var gotURL, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotURL, gotBody = r.URL.String(), string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	g := newRig(t)
	g.r = NewRelay(RelayConfig{Mappings: g.maps, Ledger: g.ledger, Now: g.clk.Now, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), KeyFP: g.fp,
		Sender: NewClient(ClientConfig{Endpoint: srv.URL + "/mp/collect", MeasurementID: "G-TEST", APISecret: "s3cret"})})
	g.mapBuyer(t, "b1", "222.2")
	g.handle(t, paidMsg(t, paid("o1", "b1", 100, nil, item("l1", "", 1, 100)), 1))
	for _, bad := range []string{"buyer_id", "user_id", "email", "phone", "b1"} {
		if strings.Contains(gotBody, bad) {
			t.Fatalf("body contains %q: %s", bad, gotBody)
		}
		if bad != "b1" && strings.Contains(gotURL, bad) {
			t.Fatalf("url contains %q: %s", bad, gotURL)
		}
	}
	if !strings.Contains(gotURL, "measurement_id=G-TEST") || !strings.Contains(gotURL, "api_secret=s3cret") {
		t.Fatalf("url lacks credentials: %s", gotURL)
	}
}

func TestRelay_LatencyBudget(t *testing.T) { // Scenario: Latency budget
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	at := g.clk.Now()
	g.clk.Advance(time.Second) // consumed 1s after payment
	start := g.clk.Now()
	g.handle(t, paidMsg(t, paid("o1", "b1", 100, &at, item("l1", "", 1, 100)), 1))
	if g.snd.count() != 1 {
		t.Fatal("request not sent synchronously during Handle")
	}
	if g.clk.Now().Sub(at) > 5*time.Minute || !g.clk.Now().Equal(start) {
		t.Fatal("relay must not delay sending (no waiting/batching)")
	}
}

func TestRelay_OtherEnvelopeTypesSkipped(t *testing.T) { // Scenario: Other envelope types are skipped
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	g.handle(t, envMsg(t, "platform.order.v1.OrderCreatedEvent", []byte("whatever"), 1))
	if g.snd.count() != 0 {
		t.Fatalf("requests=%d; want 0", g.snd.count())
	}
}

func TestRelay_LargeOrderCapped(t *testing.T) { // Scenario: Large order is capped
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	var items []*orderv1.OrderLineItemFact
	for i := 0; i < 250; i++ {
		items = append(items, item("l"+string(rune('a'+i%26))+strings.Repeat("x", i/26), "", 1, 10))
	}
	g.handle(t, paidMsg(t, paid("o1", "b1", 2500, nil, items...), 1))
	p := decode(t, g.snd.bodies[0]).Events[0].Params
	if len(p.Items) != 200 || p.Value != 2500 {
		t.Fatalf("items=%d value=%v; want 200, 2500", len(p.Items), p.Value)
	}
}

func TestHandleRedeliveryIsIdempotent(t *testing.T) { // Scenario: A redelivered order is sent once
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	m := paidMsg(t, paid("o1", "b1", 100, nil, item("l1", "", 1, 100)), 1)
	for i := 0; i < 3; i++ {
		g.handle(t, m)
	}
	if g.snd.count() != 1 {
		t.Fatalf("requests = %d, want 1", g.snd.count())
	}
}

func TestRelay_ConcurrentDuplicates(t *testing.T) { // Scenario: Concurrent duplicates
	g := newRig(t)
	g.snd.delay = 20 * time.Millisecond
	g.mapBuyer(t, "b1", "222.2")
	m := paidMsg(t, paid("o1", "b1", 100, nil, item("l1", "", 1, 100)), 1)
	var wg sync.WaitGroup
	var errs atomic.Int32
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := g.r.Handle(context.Background(), m); err != nil {
				errs.Add(1)
			}
		}()
	}
	wg.Wait()
	if errs.Load() != 0 {
		t.Fatal("Handle returned error")
	}
	if g.snd.count() != 1 {
		t.Fatalf("requests = %d, want 1", g.snd.count())
	}
}

func TestRelay_RestartAfterSend(t *testing.T) { // Scenario: Restart after send (durable ledger across reopen)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wh.duckdb")
	clk := &fakeClock{t: time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)}
	maps := NewMemoryMappingStore(clk.Now)
	_ = maps.Put(ctx, "b1", "222.2")
	snd := &fakeSender{}
	m := paidMsg(t, paid("o1", "b1", 100, nil, item("l1", "", 1, 100)), 1)

	w1, err := duckdb.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	l1, err := NewSQLLedger(ctx, w1.DB(), clk.Now)
	if err != nil {
		t.Fatal(err)
	}
	r1 := NewRelay(RelayConfig{Mappings: maps, Ledger: l1, Sender: snd, Now: clk.Now, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), KeyFP: testFP(t)})
	if err := r1.Handle(ctx, m); err != nil {
		t.Fatal(err)
	}
	if snd.count() != 1 {
		t.Fatalf("first run requests = %d, want 1", snd.count())
	}
	_ = w1.Close()

	w2, err := duckdb.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	l2, err := NewSQLLedger(ctx, w2.DB(), clk.Now)
	if err != nil {
		t.Fatal(err)
	}
	r2 := NewRelay(RelayConfig{Mappings: maps, Ledger: l2, Sender: snd, Now: clk.Now, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), KeyFP: testFP(t)})
	if err := r2.Handle(ctx, m); err != nil {
		t.Fatal(err)
	}
	if snd.count() != 1 {
		t.Fatalf("requests after restart = %d, want still 1", snd.count())
	}
}

func testFP(t *testing.T) *kafkax.Fingerprinter {
	t.Helper()
	fp, err := kafkax.NewFingerprinter("s1")
	if err != nil {
		t.Fatal(err)
	}
	return fp
}

type failingLedger struct {
	Ledger
	sentErr error
	markErr error
}

func (l failingLedger) Sent(ctx context.Context, id string) (bool, error) {
	if l.sentErr != nil {
		return false, l.sentErr
	}
	return l.Ledger.Sent(ctx, id)
}

func (l failingLedger) MarkSent(ctx context.Context, id string) error {
	if l.markErr != nil {
		return l.markErr
	}
	return l.Ledger.MarkSent(ctx, id)
}

type failingMappings struct {
	MappingStore
	getErr error
	putErr error
}

func (m failingMappings) Get(ctx context.Context, b string) (string, bool, error) {
	if m.getErr != nil {
		return "", false, m.getErr
	}
	return m.MappingStore.Get(ctx, b)
}

func (m failingMappings) Put(ctx context.Context, b, c string) error {
	if m.putErr != nil {
		return m.putErr
	}
	return m.MappingStore.Put(ctx, b, c)
}

func TestHandleLedgerFailureIsTransient(t *testing.T) { // Scenario: A ledger failure is transient
	boom := errors.New("ledger down")
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	g.ledger = failingLedger{Ledger: g.ledger, sentErr: boom}
	g.rebuild()
	err := g.r.Handle(context.Background(), paidMsg(t, paid("o1", "b1", 100, nil, item("l1", "", 1, 100)), 1))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the ledger error", err)
	}
	if errors.Is(err, ErrDeadLetter) {
		t.Fatal("ledger failure must not wrap ErrDeadLetter")
	}
	if g.snd.count() != 0 {
		t.Fatalf("requests = %d, want 0", g.snd.count())
	}
}

func TestRelay_LedgerWriteFailureIsTransient(t *testing.T) { // Scenario: A ledger failure is transient (MarkSent)
	boom := errors.New("disk full")
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	g.ledger = failingLedger{Ledger: g.ledger, markErr: boom}
	g.rebuild()
	err := g.r.Handle(context.Background(), paidMsg(t, paid("o1", "b1", 100, nil, item("l1", "", 1, 100)), 1))
	if !errors.Is(err, boom) || errors.Is(err, ErrDeadLetter) {
		t.Fatalf("err = %v, want the ledger error without ErrDeadLetter", err)
	}
}

func TestRelay_MappingLookupFailureIsTransient(t *testing.T) { // Scenario: A mapping store failure is transient
	boom := errors.New("mapping down")
	g := newRig(t)
	g.maps = failingMappings{MappingStore: g.maps, getErr: boom}
	g.rebuild()
	err := g.r.Handle(context.Background(), paidMsg(t, paid("o1", "b1", 100, nil, item("l1", "", 1, 100)), 1))
	if !errors.Is(err, boom) || errors.Is(err, ErrDeadLetter) {
		t.Fatalf("err = %v, want the store error without ErrDeadLetter", err)
	}
	if g.snd.count() != 0 {
		t.Fatalf("requests = %d, want 0", g.snd.count())
	}
}

func TestRelay_StoreFailureCountsOncePerAttempt(t *testing.T) { // Scenario: A store failure increments the store counter once per attempt
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	g.ledger = failingLedger{Ledger: g.ledger, sentErr: errors.New("ledger down")}
	g.rebuild()
	m := paidMsg(t, paid("o4", "b1", 100, nil, item("l1", "", 1, 100)), 1)
	for i := 1; i <= 5; i++ {
		if err := g.r.Handle(context.Background(), m); err == nil || errors.Is(err, ErrDeadLetter) {
			t.Fatalf("attempt %d: err = %v, want a transient error", i, err)
		}
		if got := g.r.FailedCount("store"); got != int64(i) {
			t.Fatalf("after attempt %d failed{store} = %d, want %d", i, got, i)
		}
	}
	if g.snd.count() != 0 {
		t.Fatalf("requests = %d, want 0", g.snd.count())
	}
	g.ledger = failingLedger{Ledger: g.ledger.(failingLedger).Ledger}
	g.rebuild()
	g.handle(t, m)
	if g.snd.count() != 1 {
		t.Fatalf("replay requests = %d, want exactly 1", g.snd.count())
	}
}

func TestMappingStoreFailureIsTransient(t *testing.T) { // Scenario: A mapping store failure is transient
	boom := errors.New("mapping down")
	h := NewMappingHandler(failingMappings{MappingStore: NewMemoryMappingStore(time.Now), putErr: boom})
	err := h.Handle(context.Background(), trackingRec(t, "b1", "u", "111.1", true))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the store error", err)
	}
	if errors.Is(err, ErrDeadLetter) {
		t.Fatal("store failure must not wrap ErrDeadLetter")
	}
}

func TestMappingUndecodableRecordIsPermanent(t *testing.T) { // Scenario: An invalid envelope is permanent (mapping handler follows the same rule)
	h := NewMappingHandler(NewMemoryMappingStore(time.Now))
	err := h.Handle(context.Background(), consume.Record{Topic: "analytics.events", Value: []byte{0xff, 0xff, 0xff, 0x01}})
	if !errors.Is(err, ErrDeadLetter) {
		t.Fatalf("err = %v, want wrapping ErrDeadLetter", err)
	}
}

func TestRelayLogsNeverContainRawKey(t *testing.T) { // Scenario: The relay logs the fingerprint and not the buyer key
	const key = "buyer-9"
	g := newRig(t)
	err := g.r.Handle(context.Background(), consume.Record{Topic: "order.events", Key: []byte(key), Value: []byte{0xff, 0xff, 0xff, 0x01}})
	if !errors.Is(err, ErrDeadLetter) {
		t.Fatalf("err = %v, want wrapping ErrDeadLetter", err)
	}
	if !strings.Contains(g.logs.String(), `"key_fp":"`+g.fp.Of([]byte(key))+`"`) {
		t.Fatalf("log lacks key_fp: %s", g.logs.String())
	}
	if strings.Contains(g.logs.String(), key) || strings.Contains(err.Error(), key) {
		t.Fatalf("raw key leaked: log=%s err=%v", g.logs.String(), err)
	}
}

func TestRelayConsumerOptionsFitRebalanceBudget(t *testing.T) { // Scenarios: A GA4 outage does not exceed the rebalance timeout; The relay polls one record at a time
	for _, k := range sdkconfig.DeclaredEnvKeys(config.Settings{}) {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50059")
	s, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	o := ConsumerOptions(s)
	if o.MaxPollRecords != 1 {
		t.Fatalf("MaxPollRecords = %d, want 1", o.MaxPollRecords)
	}
	if o.DLQTopic != s.GA4.RelayDLQTopic || o.DLQTopic == "" {
		t.Fatalf("DLQTopic = %q, want %q", o.DLQTopic, s.GA4.RelayDLQTopic)
	}
	worst := time.Duration(maxAttempts) * DefaultRequestTimeout
	for retry := 0; retry < maxAttempts-1; retry++ {
		worst += Backoff(retry)
	}
	if worst != 40*time.Second {
		t.Fatalf("worst-case handle time = %v, want 40s", worst)
	}
	if total := time.Duration(o.MaxPollRecords) * worst; total >= 60*time.Second {
		t.Fatalf("poll worst case %v must be under the 60s rebalance timeout", total)
	}
}

func TestRelay_BuyerWithoutMapping(t *testing.T) { // Scenarios: Buyer without mapping; Consent revoked before payment (mapping deleted)
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	if err := g.maps.Delete(context.Background(), "b1"); err != nil {
		t.Fatal(err)
	}
	g.handle(t, paidMsg(t, paid("o1", "b1", 100, nil, item("l1", "", 1, 100)), 1))
	g.handle(t, paidMsg(t, paid("o2", "b2", 100, nil, item("l1", "", 1, 100)), 2))
	if g.snd.count() != 0 {
		t.Fatalf("requests = %d, want 0", g.snd.count())
	}
	if g.r.SkippedCount("no_client_id") != 2 {
		t.Fatalf("skipped{no_client_id} = %d, want 2", g.r.SkippedCount("no_client_id"))
	}
	if ok, _ := g.ledger.Sent(context.Background(), "o1"); ok {
		t.Fatal("skipped order must not be in the ledger")
	}
}

var _ = sql.ErrNoRows

func TestRelay_ItemCurrencyFallsBackToOrderCurrency(t *testing.T) { // Scenario: Item ids and value mapping (item currency)
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	it := item("l1", "", 1, 100)
	it.Currency = ""
	e := paid("o1", "b1", 100, nil, it)
	e.Currency = "USD"
	g.handle(t, paidMsg(t, e, 1))
	if c := decode(t, g.snd.bodies[0]).Events[0].Params.Items[0].Currency; c != "USD" {
		t.Fatalf("item currency = %q, want order currency USD", c)
	}
}
