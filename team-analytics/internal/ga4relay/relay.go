package ga4relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-analytics/generated/platform/order/v1"
	"github.com/buidangphuc/team-analytics/internal/config"
)

const (
	orderPaidType = "platform.order.v1.OrderPaidEvent"
	maxItems      = 200
)

// Sender delivers a Measurement Protocol body.
type Sender interface {
	Send(ctx context.Context, body []byte) error
}

// ErrDeadLetter marks a record that can never be relayed.
var ErrDeadLetter = errors.New("ga4relay: dead letter")

// RelayConfig wires a Relay.
type RelayConfig struct {
	Mappings MappingStore
	Ledger   Ledger
	Sender   Sender
	Now      func() time.Time
	Logger   *slog.Logger
	KeyFP    *kafkax.Fingerprinter
}

// ConsumerOptions returns the SDK consumer options for the relay: one record
// per poll keeps a GA4 outage under the rebalance timeout.
func ConsumerOptions(s config.Settings) consume.Options {
	return consume.Options{DLQTopic: s.GA4.RelayDLQTopic, MaxPollRecords: 1}
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

func (r *Relay) logger() *slog.Logger {
	if r.cfg.Logger != nil {
		return r.cfg.Logger
	}
	return slog.Default()
}

func (r *Relay) deadLetter(rec consume.Record, reason string, cause error) error {
	r.incFailed(reason)
	args := []any{"topic", rec.Topic, "reason", reason}
	if r.cfg.KeyFP != nil {
		args = append(args, "key_fp", r.cfg.KeyFP.Of(rec.Key))
	}
	r.logger().Warn("ga4 relay dead-lettered record", args...)
	if cause != nil {
		return consume.Permanent(fmt.Errorf("%w: %s: %w", ErrDeadLetter, reason, cause))
	}
	return consume.Permanent(fmt.Errorf("%w: %s", ErrDeadLetter, reason))
}

// Handle processes one record. nil means sent, skipped, ignored or already sent.
func (r *Relay) Handle(ctx context.Context, rec consume.Record) error {
	err := r.handle(ctx, rec)
	if err != nil && !errors.Is(err, ErrDeadLetter) {
		r.incFailed("store")
	}
	return err
}

func (r *Relay) handle(ctx context.Context, rec consume.Record) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		return r.deadLetter(rec, "invalid", nil)
	}
	if env.GetType() != orderPaidType {
		return nil
	}
	var ev orderv1.OrderPaidEvent
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil || ev.GetOrderId() == "" {
		return r.deadLetter(rec, "invalid", nil)
	}
	orderID := ev.GetOrderId()

	unlock := r.lock(orderID)
	defer unlock()

	sent, err := r.cfg.Ledger.Sent(ctx, orderID)
	if err != nil {
		return err
	}
	if sent {
		return nil
	}
	clientID, ok, err := r.cfg.Mappings.Get(ctx, ev.GetBuyerId())
	if err != nil {
		return err
	}
	if !ok || clientID == "" {
		r.incSkipped("no_client_id")
		return nil
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
			return r.deadLetter(rec, se.Reason, err)
		}
		return err
	}
	return r.cfg.Ledger.MarkSent(ctx, orderID)
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
