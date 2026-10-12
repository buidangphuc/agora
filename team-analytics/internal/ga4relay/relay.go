package ga4relay

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-analytics/generated/platform/order/v1"
)

const (
	orderPaidType = "platform.order.v1.OrderPaidEvent"
	maxItems      = 200
)

// Message is a consumed Kafka record.
type Message struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Value     []byte
}

// Sender delivers a Measurement Protocol body.
type Sender interface {
	Send(ctx context.Context, body []byte) error
}

// DLQProducer publishes an unprocessable message to the dead-letter topic.
type DLQProducer interface {
	Publish(ctx context.Context, msg Message, reason string) error
}

// OffsetCommitter commits a consumed message's offset.
type OffsetCommitter interface {
	Commit(ctx context.Context, msg Message) error
}

// RelayConfig wires a Relay.
type RelayConfig struct {
	Mappings  MappingStore
	Ledger    Ledger
	Sender    Sender
	DLQ       DLQProducer
	Committer OffsetCommitter
	Now       func() time.Time
}

// Relay turns OrderPaidEvents into GA4 purchase events.
type Relay struct {
	cfg RelayConfig

	mu       sync.Mutex
	failed   map[string]int64
	skipped  map[string]int64
	paidMiss int64
	locks    map[string]*orderLock
}

type orderLock struct {
	mu   sync.Mutex
	refs int
}

// NewRelay builds a Relay.
func NewRelay(cfg RelayConfig) *Relay {
	return &Relay{cfg: cfg, failed: map[string]int64{}, skipped: map[string]int64{}, locks: map[string]*orderLock{}}
}

func (r *Relay) incFailed(reason string)  { r.mu.Lock(); r.failed[reason]++; r.mu.Unlock() }
func (r *Relay) incSkipped(reason string) { r.mu.Lock(); r.skipped[reason]++; r.mu.Unlock() }

// FailedCount returns the failed-event count for reason (invalid, 4xx, exhausted).
func (r *Relay) FailedCount(reason string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failed[reason]
}

// SkippedCount returns the skipped-event count for reason.
func (r *Relay) SkippedCount(reason string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.skipped[reason]
}

// PaidAtMissingCount returns how many paid events had no paid_at.
func (r *Relay) PaidAtMissingCount() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.paidMiss
}

func (r *Relay) lock(orderID string) func() {
	r.mu.Lock()
	l := r.locks[orderID]
	if l == nil {
		l = &orderLock{}
		r.locks[orderID] = l
	}
	l.refs++
	r.mu.Unlock()
	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		r.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(r.locks, orderID)
		}
		r.mu.Unlock()
	}
}

func (r *Relay) commit(ctx context.Context, msg Message) error {
	return r.cfg.Committer.Commit(ctx, msg)
}

func (r *Relay) deadLetter(ctx context.Context, msg Message, reason string) error {
	if err := r.cfg.DLQ.Publish(ctx, msg, reason); err != nil {
		return err
	}
	r.incFailed(reason)
	return r.commit(ctx, msg)
}

// Handle processes one message. nil means the offset was committed.
func (r *Relay) Handle(ctx context.Context, msg Message) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(msg.Value, &env); err != nil {
		return r.deadLetter(ctx, msg, "invalid")
	}
	if env.GetType() != orderPaidType {
		return r.commit(ctx, msg)
	}
	var ev orderv1.OrderPaidEvent
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil || ev.GetOrderId() == "" {
		return r.deadLetter(ctx, msg, "invalid")
	}
	orderID := ev.GetOrderId()

	unlock := r.lock(orderID)
	defer unlock()

	sent, err := r.cfg.Ledger.Sent(ctx, orderID)
	if err != nil {
		return err
	}
	if sent {
		return r.commit(ctx, msg)
	}
	clientID, ok, err := r.cfg.Mappings.Get(ctx, ev.GetBuyerId())
	if err != nil {
		return err
	}
	if !ok || clientID == "" {
		r.incSkipped("no_client_id")
		return r.commit(ctx, msg)
	}
	if ev.GetPaidAt() == nil {
		r.mu.Lock()
		r.paidMiss++
		r.mu.Unlock()
	}
	body, err := buildPayload(clientID, &ev)
	if err != nil {
		return err
	}
	if err := r.cfg.Sender.Send(ctx, body); err != nil {
		var se *SendError
		if errors.As(err, &se) {
			return r.deadLetter(ctx, msg, se.Reason)
		}
		return err
	}
	if err := r.cfg.Ledger.MarkSent(ctx, orderID); err != nil {
		return err
	}
	return r.commit(ctx, msg)
}

type gaItem struct {
	ItemID   string `json:"item_id"`
	Quantity int32  `json:"quantity"`
	Price    int64  `json:"price"`
	Currency string `json:"currency"`
}

type gaParams struct {
	TransactionID string   `json:"transaction_id"`
	Currency      string   `json:"currency"`
	Value         int64    `json:"value"`
	Items         []gaItem `json:"items"`
}

type gaEvent struct {
	Name   string   `json:"name"`
	Params gaParams `json:"params"`
}

type gaPayload struct {
	ClientID        string    `json:"client_id"`
	TimestampMicros *int64    `json:"timestamp_micros,omitempty"`
	Events          []gaEvent `json:"events"`
}

func buildPayload(clientID string, ev *orderv1.OrderPaidEvent) ([]byte, error) {
	items := make([]gaItem, 0, len(ev.GetItems()))
	for _, it := range ev.GetItems() {
		if len(items) == maxItems {
			break
		}
		id := it.GetListingId()
		if v := it.GetVariantId(); v != "" {
			id += ":" + v
		}
		cur := it.GetCurrency()
		if cur == "" {
			cur = ev.GetCurrency()
		}
		items = append(items, gaItem{ItemID: id, Quantity: it.GetQuantity(), Price: it.GetUnitPrice(), Currency: cur})
	}
	p := gaPayload{
		ClientID: clientID,
		Events: []gaEvent{{Name: "purchase", Params: gaParams{
			TransactionID: ev.GetOrderId(), Currency: ev.GetCurrency(), Value: ev.GetTotalAmount(), Items: items,
		}}},
	}
	if ev.GetPaidAt() != nil {
		us := ev.GetPaidAt().AsTime().UnixMicro()
		p.TimestampMicros = &us
	}
	return json.Marshal(p)
}
