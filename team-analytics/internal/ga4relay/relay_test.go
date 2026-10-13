package ga4relay

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-analytics/generated/platform/order/v1"
	"github.com/buidangphuc/team-analytics/internal/store/duckdb"
)

// Required interface (implemented in relay*.go by the implementer):
//
//	type Message struct{ Topic string; Partition int32; Offset int64; Key, Value []byte }
//	type Sender interface{ Send(ctx context.Context, body []byte) error }          // *Client satisfies
//	type DLQProducer interface{ Publish(ctx context.Context, msg Message, reason string) error }
//	type OffsetCommitter interface{ Commit(ctx context.Context, msg Message) error }
//	type Ledger interface {
//	    Sent(ctx context.Context, orderID string) (bool, error)
//	    MarkSent(ctx context.Context, orderID string) error // retained 90 days
//	}
//	func NewMemoryLedger(now func() time.Time) Ledger
//	func NewSQLLedger(ctx context.Context, db *sql.DB, now func() time.Time) (Ledger, error) // table ga4_relay_sent
//	type RelayConfig struct{ Mappings MappingStore; Ledger Ledger; Sender Sender; DLQ DLQProducer; Committer OffsetCommitter; Now func() time.Time }
//	func NewRelay(cfg RelayConfig) *Relay
//	func (r *Relay) Handle(ctx context.Context, msg Message) error // nil => offset committed (send/skip/ledger/DLQ done); error => NOT committed
//	func (r *Relay) FailedCount(reason string) int64; SkippedCount(reason string) int64; PaidAtMissingCount() int64

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

type fakeDLQ struct {
	mu   sync.Mutex
	msgs []Message
	why  []string
}

func (d *fakeDLQ) Publish(_ context.Context, m Message, reason string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.msgs = append(d.msgs, m)
	d.why = append(d.why, reason)
	return nil
}

type fakeCommitter struct {
	mu   sync.Mutex
	n    int
	fail error
}

func (c *fakeCommitter) Commit(_ context.Context, _ Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return c.fail
	}
	c.n++
	return nil
}
func (c *fakeCommitter) commits() int { c.mu.Lock(); defer c.mu.Unlock(); return c.n }

type rig struct {
	r      *Relay
	snd    *fakeSender
	dlq    *fakeDLQ
	com    *fakeCommitter
	maps   MappingStore
	ledger Ledger
	clk    *fakeClock
}

func newRig(t *testing.T) *rig {
	t.Helper()
	clk := &fakeClock{t: time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)}
	g := &rig{
		snd: &fakeSender{}, dlq: &fakeDLQ{}, com: &fakeCommitter{}, clk: clk,
		maps: NewMemoryMappingStore(clk.Now), ledger: NewMemoryLedger(clk.Now),
	}
	g.rebuild()
	return g
}

func (g *rig) rebuild() {
	g.r = NewRelay(RelayConfig{Mappings: g.maps, Ledger: g.ledger, Sender: g.snd, DLQ: g.dlq, Committer: g.com, Now: g.clk.Now})
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

func envMsg(t *testing.T, typ string, payload []byte, offset int64) Message {
	t.Helper()
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "ev", Type: typ, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return Message{Topic: "order.events", Offset: offset, Key: []byte("k"), Value: b}
}

func paidMsg(t *testing.T, e *orderv1.OrderPaidEvent, offset int64) Message {
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

func (g *rig) handle(t *testing.T, m Message) {
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
	if g.com.commits() != 1 {
		t.Fatalf("commits = %d, want 1", g.com.commits())
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

func TestRelay_MalformedEventRejected(t *testing.T) { // Scenario: Malformed order event is rejected
	cases := map[string]Message{}
	cases["empty order_id"] = paidMsg(t, paid("", "b1", 100, nil, item("l1", "", 1, 100)), 1)
	cases["undecodable payload"] = envMsg(t, "platform.order.v1.OrderPaidEvent", []byte{0xff, 0xff, 0xff, 0x01}, 2)
	cases["undecodable envelope"] = Message{Topic: "order.events", Offset: 3, Value: []byte{0xff, 0xff, 0xff, 0x01}}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			g := newRig(t)
			g.mapBuyer(t, "b1", "222.2")
			g.handle(t, m)
			if g.snd.count() != 0 {
				t.Fatalf("requests = %d, want 0", g.snd.count())
			}
			if len(g.dlq.msgs) != 1 {
				t.Fatalf("dlq = %d, want 1", len(g.dlq.msgs))
			}
			if g.r.FailedCount("invalid") != 1 {
				t.Fatalf("failed{invalid} = %d, want 1", g.r.FailedCount("invalid"))
			}
			if g.com.commits() != 1 {
				t.Fatalf("commits = %d, want 1", g.com.commits())
			}
		})
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

func TestRelay_FailedSendNotRecordedAsSent(t *testing.T) { // Scenario: Failed send is not recorded as sent
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	g.snd.errs = []error{&SendError{Reason: "4xx", Status: 400, Attempts: 1}}
	m := paidMsg(t, paid("o3", "b1", 100, nil, item("l1", "", 1, 100)), 1)
	g.handle(t, m)
	if ok, _ := g.ledger.Sent(context.Background(), "o3"); ok {
		t.Fatal("o3 recorded as sent after permanent failure")
	}
	if len(g.dlq.msgs) != 1 || g.r.FailedCount("4xx") != 1 || g.com.commits() != 1 {
		t.Fatalf("dlq=%d failed4xx=%d commits=%d; want 1,1,1", len(g.dlq.msgs), g.r.FailedCount("4xx"), g.com.commits())
	}
	g.handle(t, m) // redelivered after recovery
	if g.snd.count() != 2 {
		t.Fatalf("requests = %d, want 2 (1 failed + exactly 1 after recovery)", g.snd.count())
	}
	if ok, _ := g.ledger.Sent(context.Background(), "o3"); !ok {
		t.Fatal("o3 not in ledger after successful resend")
	}
}

func TestRelay_ExhaustedGoesToDLQ(t *testing.T) { // Scenario: Failed send is not recorded as sent (exhausted reason)
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	g.snd.errs = []error{&SendError{Reason: "exhausted", Status: 503, Attempts: 5}}
	g.handle(t, paidMsg(t, paid("o3", "b1", 100, nil, item("l1", "", 1, 100)), 1))
	if g.r.FailedCount("exhausted") != 1 || len(g.dlq.msgs) != 1 || g.com.commits() != 1 {
		t.Fatalf("exhausted=%d dlq=%d commits=%d; want 1,1,1", g.r.FailedCount("exhausted"), len(g.dlq.msgs), g.com.commits())
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
	g.r = NewRelay(RelayConfig{
		Mappings: g.maps, Ledger: g.ledger, DLQ: g.dlq, Committer: g.com, Now: g.clk.Now,
		Sender: NewClient(ClientConfig{Endpoint: srv.URL + "/mp/collect", MeasurementID: "G-TEST", APISecret: "s3cret"}),
	})
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
	if g.snd.count() != 0 || len(g.dlq.msgs) != 0 {
		t.Fatalf("requests=%d dlq=%d; want 0,0", g.snd.count(), len(g.dlq.msgs))
	}
	if g.com.commits() != 1 {
		t.Fatalf("commits = %d, want 1", g.com.commits())
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

func TestRelay_KafkaRedelivery(t *testing.T) { // Scenario: Kafka redelivery
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	m := paidMsg(t, paid("o1", "b1", 100, nil, item("l1", "", 1, 100)), 1)
	for i := 0; i < 3; i++ {
		g.handle(t, m)
	}
	if g.snd.count() != 1 {
		t.Fatalf("requests = %d, want 1", g.snd.count())
	}
	if g.com.commits() != 3 {
		t.Fatalf("commits = %d, want 3 (duplicates are still committed)", g.com.commits())
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
	crash := &fakeCommitter{fail: errors.New("crash before commit")}
	r1 := NewRelay(RelayConfig{Mappings: maps, Ledger: l1, Sender: snd, DLQ: &fakeDLQ{}, Committer: crash, Now: clk.Now})
	if err := r1.Handle(ctx, m); err == nil {
		t.Fatal("commit failure must surface as an error")
	}
	if snd.count() != 1 {
		t.Fatalf("first run requests = %d, want 1", snd.count())
	}
	_ = w1.Close() // "restart"

	w2, err := duckdb.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	l2, err := NewSQLLedger(ctx, w2.DB(), clk.Now)
	if err != nil {
		t.Fatal(err)
	}
	com := &fakeCommitter{}
	r2 := NewRelay(RelayConfig{Mappings: maps, Ledger: l2, Sender: snd, DLQ: &fakeDLQ{}, Committer: com, Now: clk.Now})
	if err := r2.Handle(ctx, m); err != nil {
		t.Fatal(err)
	}
	if snd.count() != 1 {
		t.Fatalf("requests after restart = %d, want still 1", snd.count())
	}
	if com.commits() != 1 {
		t.Fatalf("commits = %d, want 1", com.commits())
	}
}

func TestRelay_LedgerWriteFailureNotCommitted(t *testing.T) { // Scenario: Kafka redelivery (offset only after ledger write)
	g := newRig(t)
	g.mapBuyer(t, "b1", "222.2")
	g.ledger = failingLedger{Ledger: g.ledger}
	g.rebuild()
	if err := g.r.Handle(context.Background(), paidMsg(t, paid("o1", "b1", 100, nil, item("l1", "", 1, 100)), 1)); err == nil {
		t.Fatal("expected error when ledger write fails")
	}
	if g.com.commits() != 0 {
		t.Fatalf("commits = %d, want 0", g.com.commits())
	}
}

type failingLedger struct{ Ledger }

func (failingLedger) MarkSent(context.Context, string) error { return errors.New("disk full") }

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
	if g.com.commits() != 2 {
		t.Fatalf("commits = %d, want 2", g.com.commits())
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
