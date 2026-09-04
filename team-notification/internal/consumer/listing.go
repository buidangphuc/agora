// Package consumer holds team-notification's Kafka event consumers. The
// ListingConsumer consumes `listing.events` and turns ONE domain event —
// platform.listing.v1.ListingChanged — into in-app notifications for users who
// asked to be alerted. That event is the only listing event team-domain's outbox
// emits: it carries the FULL new listing snapshot plus a change_type, not an
// old/new delta. So the consumer self-diffs against the last state it saw:
//
//   - Price drop: when a last-seen price exists for the listing and the snapshot's
//     current price is strictly below it, a price-drop alert fires to that
//     listing's PRICE_DROP subscribers.
//   - Back in stock: on a 0 → positive stock transition (last-seen stock known and
//     zero, snapshot stock positive), a back-in-stock alert fires to the listing's
//     BACK_IN_STOCK subscribers. A first-ever (unknown-prior) snapshot never fires.
//
// change_type is used only as a hint (a DELETED snapshot is skipped so its zeroed
// price cannot masquerade as a drop); the self-diff is the source of truth.
//
// Delivery discipline mirrors team-order's PaymentSettled consumer (ADR-0002 /
// AD1/AD4): the offset is committed ONLY after a record is applied or dead-lettered,
// poison records go to `<topic>.dlq`, and consumption is idempotent — an event_id
// dedupe ledger short-circuits redelivery, and each transition is additionally
// guarded by its last-seen value (advanced only after notifications succeed) so
// re-applying the same event never double-notifies.
//
// The transport (a real Kafka reader + DLQ producer) is injected as interfaces so
// the consumption logic is unit-testable without a broker; the concrete franz-go
// wiring lives in internal/bootstrap + cmd/server.
package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-notification/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-notification/generated/platform/listing/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

// Fully-qualified proto names the EventEnvelope carries for the listing events we
// react to. Consumers switch on EventEnvelope.type (ADR-0005).
const (
	listingChangedType = "platform.listing.v1.ListingChanged"
)

// consumerName namespaces this consumer's rows in the dedupe ledger.
const consumerName = "team-notification.listing"

// ErrPermanent marks an error that will never succeed on retry (a poison record).
// The loop routes such records to the DLQ instead of retrying indefinitely.
var ErrPermanent = errors.New("permanent consumer error")

// NotificationCreator is the slice of the notification repository the consumer
// needs. *repository.PostgresNotificationRepo satisfies it.
type NotificationCreator interface {
	CreateNotification(ctx context.Context, n *notificationv1.Notification) (*notificationv1.Notification, error)
}

// SubscriptionStore is the slice of the alert-subscription repository the consumer
// needs to fan a matched event out to every subscribed user.
// *repository.PostgresAlertSubscriptionRepo satisfies it.
type SubscriptionStore interface {
	ListByListingAndType(ctx context.Context, listingID string, alertType notificationv1.AlertType) ([]*notificationv1.AlertSubscription, error)
}

// Deduper is the idempotency ledger (AD4): it records the event_id of every event
// already applied so redelivery is a no-op. MarkProcessed reports whether the id
// was newly inserted. The in-memory default is process-local; a durable
// implementation can be injected for cross-restart dedupe.
type Deduper interface {
	IsProcessed(ctx context.Context, consumer, eventID string) (bool, error)
	MarkProcessed(ctx context.Context, consumer, eventID string) (bool, error)
}

// StockStateStore remembers the last-seen stock per listing so the consumer can
// detect a 0 → positive transition from a ListingChanged snapshot, which only
// carries the current level. Get returns (level, known); known is false the first
// time a listing is seen.
type StockStateStore interface {
	Get(listingID string) (int32, bool)
	Set(listingID string, stock int32)
}

// PriceStateStore remembers the last-seen price per listing so the consumer can
// detect a price drop from a ListingChanged snapshot, which carries only the
// current price (not an old/new delta). Get returns (price, known); known is false
// the first time a listing is seen, so a first-ever snapshot never fires a drop.
type PriceStateStore interface {
	Get(listingID string) (int64, bool)
	Set(listingID string, price int64)
}

// ListingConsumer applies listing.events to notifications idempotently.
type ListingConsumer struct {
	notif  NotificationCreator
	subs   SubscriptionStore
	dedupe Deduper
	stock  StockStateStore
	price  PriceStateStore
	logger *slog.Logger
}

// Option customizes a ListingConsumer without breaking the positional constructor.
type Option func(*ListingConsumer)

// WithDeduper injects a durable dedupe ledger (default: in-memory, process-local).
func WithDeduper(d Deduper) Option {
	return func(c *ListingConsumer) {
		if d != nil {
			c.dedupe = d
		}
	}
}

// WithStockStateStore injects a durable last-seen-stock store (default: in-memory).
func WithStockStateStore(s StockStateStore) Option {
	return func(c *ListingConsumer) {
		if s != nil {
			c.stock = s
		}
	}
}

// WithPriceStateStore injects a durable last-seen-price store (default: in-memory).
func WithPriceStateStore(p PriceStateStore) Option {
	return func(c *ListingConsumer) {
		if p != nil {
			c.price = p
		}
	}
}

// NewListingConsumer builds the consumer over a notification creator and a
// subscription store, defaulting the dedupe ledger and stock-state store to
// in-memory implementations.
func NewListingConsumer(notif NotificationCreator, subs SubscriptionStore, logger *slog.Logger, opts ...Option) *ListingConsumer {
	if logger == nil {
		logger = slog.Default()
	}
	c := &ListingConsumer{
		notif:  notif,
		subs:   subs,
		dedupe: NewInMemoryDeduper(),
		stock:  NewInMemoryStockStateStore(),
		price:  NewInMemoryPriceStateStore(),
		logger: logger,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// HandleRaw decodes a Kafka record value (a marshalled EventEnvelope) and applies
// it. It is the entry point the transport loop calls per record.
func (c *ListingConsumer) HandleRaw(ctx context.Context, value []byte) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		// A malformed envelope is a poison record: it can never succeed, so report a
		// permanent error the loop routes to the DLQ rather than retrying forever.
		return fmt.Errorf("%w: unmarshal envelope: %v", ErrPermanent, err)
	}
	return c.HandleEnvelope(ctx, &env)
}

// HandleEnvelope applies one decoded envelope. Envelopes for events we do not
// react to are ignored. Delivery is at-least-once, so the dedupe ledger
// short-circuits a redelivered event_id; the effect is recorded only AFTER it is
// applied, so a crash in between simply redelivers.
func (c *ListingConsumer) HandleEnvelope(ctx context.Context, env *eventsv1.EventEnvelope) error {
	if env.GetType() != listingChangedType {
		return nil // not ours; ignore
	}

	if env.GetEventId() == "" {
		return fmt.Errorf("%w: envelope missing event_id", ErrPermanent)
	}

	already, err := c.dedupe.IsProcessed(ctx, consumerName, env.GetEventId())
	if err != nil {
		return fmt.Errorf("dedupe lookup: %w", err) // transient → retry, don't commit
	}
	if already {
		return nil // duplicate delivery — idempotent no-op
	}

	if err := c.handleListingChanged(ctx, env.GetPayload()); err != nil {
		return err
	}

	// Record the event only AFTER the effect is applied. A crash between apply and
	// mark simply redelivers; the dedupe (and, for stock, the last-seen guard) make
	// the second delivery a no-op.
	if _, err := c.dedupe.MarkProcessed(ctx, consumerName, env.GetEventId()); err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}
	return nil
}

// handleListingChanged decodes a ListingChanged snapshot and self-diffs it against
// the last state the consumer saw, firing a price-drop and/or a back-in-stock alert
// as warranted. The event carries the full new listing (not an old/new delta), so
// the last-seen price and stock stores ARE the "old" side of the diff.
//
// A DELETED snapshot is skipped outright (change_type as a hint): its price is
// zeroed and would otherwise read as a drop. Each last-seen value is advanced only
// after that transition's notifications succeed, so a transient failure re-detects
// the transition on redelivery rather than swallowing it.
func (c *ListingConsumer) handleListingChanged(ctx context.Context, payload []byte) error {
	var e listingv1.ListingChanged
	if err := proto.Unmarshal(payload, &e); err != nil {
		return fmt.Errorf("%w: unmarshal ListingChanged: %v", ErrPermanent, err)
	}
	l := e.GetListing()
	if l == nil || l.GetId() == "" {
		return fmt.Errorf("%w: ListingChanged missing listing", ErrPermanent)
	}
	if e.GetChangeType() == listingv1.ChangeType_CHANGE_TYPE_DELETED {
		// A delete carries a zeroed snapshot; never treat it as a drop / stock event.
		return nil
	}

	listingID := l.GetId()

	// ── Price drop: current price strictly below the last-seen price. ──
	curPrice := l.GetPrice()
	prevPrice, priceKnown := c.price.Get(listingID)
	if priceKnown && curPrice < prevPrice {
		if err := c.notifySubscribers(ctx, listingID, notificationv1.AlertType_ALERT_TYPE_PRICE_DROP,
			notificationv1.NotificationType_NOTIFICATION_TYPE_PRICE_DROP,
			"Giảm giá!",
			fmt.Sprintf("Sản phẩm bạn theo dõi đã giảm giá còn %d (trước đó %d).", curPrice, prevPrice),
		); err != nil {
			// Do NOT advance the last-seen price: leave the higher value so the drop
			// is re-detected when the event redelivers.
			return err
		}
	}
	// Record the current price (fired or not, first-seen included) for future diffs.
	c.price.Set(listingID, curPrice)

	// ── Back in stock: 0 → positive transition. ──
	curStock := l.GetStock()
	prevStock, stockKnown := c.stock.Get(listingID)
	if stockKnown && prevStock == 0 && curStock > 0 {
		if err := c.notifySubscribers(ctx, listingID, notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK,
			notificationv1.NotificationType_NOTIFICATION_TYPE_BACK_IN_STOCK,
			"Đã có hàng trở lại!",
			"Sản phẩm bạn theo dõi đã có hàng trở lại. Nhanh tay đặt mua nhé!",
		); err != nil {
			// Do NOT advance the last-seen stock: leave the 0 in place so the
			// transition is re-detected when the event redelivers.
			return err
		}
	}
	// Record the current level (fired or not, first-seen included) for future diffs.
	c.stock.Set(listingID, curStock)
	return nil
}

// notifySubscribers fans an alert out to every user subscribed to (listing, alertType),
// creating one in-app notification each. A store/create error is transient: it is
// returned so the offset is not committed and the record redelivers.
func (c *ListingConsumer) notifySubscribers(ctx context.Context, listingID string, alertType notificationv1.AlertType, notiType notificationv1.NotificationType, title, body string) error {
	subs, err := c.subs.ListByListingAndType(ctx, listingID, alertType)
	if err != nil {
		return fmt.Errorf("list subscriptions for %q: %w", listingID, err)
	}
	link := "/listing/" + listingID
	for _, sub := range subs {
		n := &notificationv1.Notification{
			UserId:  sub.GetUserId(),
			Title:   title,
			Body:    body,
			Type:    notiType,
			LinkUrl: link,
		}
		if _, err := c.notif.CreateNotification(ctx, n); err != nil {
			return fmt.Errorf("create notification for user %q: %w", sub.GetUserId(), err)
		}
		c.logger.InfoContext(ctx, "alert notification created",
			slog.String("user_id", sub.GetUserId()),
			slog.String("listing_id", listingID),
			slog.String("type", notiType.String()),
		)
	}
	return nil
}

// ── In-memory dedupe ledger ──

// InMemoryDeduper is a process-local event_id ledger. It is the default when no
// durable Deduper is injected; note dedupe resets on restart.
type InMemoryDeduper struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewInMemoryDeduper() *InMemoryDeduper {
	return &InMemoryDeduper{seen: make(map[string]struct{})}
}

func dedupeKey(consumer, eventID string) string { return consumer + "\x00" + eventID }

func (d *InMemoryDeduper) IsProcessed(_ context.Context, consumer, eventID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.seen[dedupeKey(consumer, eventID)]
	return ok, nil
}

func (d *InMemoryDeduper) MarkProcessed(_ context.Context, consumer, eventID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	k := dedupeKey(consumer, eventID)
	if _, ok := d.seen[k]; ok {
		return false, nil
	}
	d.seen[k] = struct{}{}
	return true, nil
}

// ── In-memory last-seen-stock store ──

// InMemoryStockStateStore tracks the last-seen stock per listing in memory. It is
// the default when no durable StockStateStore is injected; it is process-local, so
// the very first stock event for a listing after a restart is treated as "unknown
// prior" and never fires a back-in-stock alert on its own.
type InMemoryStockStateStore struct {
	mu   sync.Mutex
	last map[string]int32
}

func NewInMemoryStockStateStore() *InMemoryStockStateStore {
	return &InMemoryStockStateStore{last: make(map[string]int32)}
}

func (s *InMemoryStockStateStore) Get(listingID string) (int32, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.last[listingID]
	return v, ok
}

func (s *InMemoryStockStateStore) Set(listingID string, stock int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last[listingID] = stock
}

// ── In-memory last-seen-price store ──

// InMemoryPriceStateStore tracks the last-seen price per listing in memory. It is
// the default when no durable PriceStateStore is injected; it is process-local, so
// the very first snapshot for a listing after a restart is treated as "unknown
// prior" and never fires a price-drop alert on its own.
type InMemoryPriceStateStore struct {
	mu   sync.Mutex
	last map[string]int64
}

func NewInMemoryPriceStateStore() *InMemoryPriceStateStore {
	return &InMemoryPriceStateStore{last: make(map[string]int64)}
}

func (s *InMemoryPriceStateStore) Get(listingID string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.last[listingID]
	return v, ok
}

func (s *InMemoryPriceStateStore) Set(listingID string, price int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last[listingID] = price
}

// ── Transport loop (AD1 offset discipline) ──

// Record is one Kafka record the transport hands to the loop.
type Record struct {
	Key   string
	Value []byte
}

// RecordReader is the transport the loop reads from. A real franz-go reader
// satisfies it; tests use a fake. Fetch blocks until the next record (or ctx is
// done); Commit advances the committed offset past rec.
type RecordReader interface {
	Fetch(ctx context.Context) (Record, error)
	Commit(ctx context.Context, rec Record) error
}

// DeadLetterProducer publishes poison / max-retried records to a DLQ topic (AD1).
type DeadLetterProducer interface {
	Produce(ctx context.Context, topic string, rec Record) error
}

// RunConfig tunes the consume loop.
type RunConfig struct {
	DLQTopic    string        // e.g. "listing.events.dlq"
	MaxAttempts int           // in-process attempts before DLQ (default 5)
	BaseBackoff time.Duration // first retry delay; scales per attempt (default 200ms)
}

func (c RunConfig) withDefaults() RunConfig {
	if c.DLQTopic == "" {
		c.DLQTopic = "listing.events.dlq"
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 200 * time.Millisecond
	}
	return c
}

// Run consumes until ctx is cancelled. Per record it applies the handler with
// bounded in-process retries; on a permanent error or once retries are exhausted
// it produces the record to the DLQ. Crucially it commits the offset ONLY after
// the record is either applied or DLQ'd — it never advances past an unprocessed
// record (AD1).
func (c *ListingConsumer) Run(ctx context.Context, reader RecordReader, dlq DeadLetterProducer, cfg RunConfig) error {
	cfg = cfg.withDefaults()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rec, err := reader.Fetch(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			c.logger.WarnContext(ctx, "fetch record failed", slog.Any("err", err))
			continue
		}

		if perr := c.processWithRetry(ctx, rec, dlq, cfg); perr != nil {
			// Could not process AND could not DLQ: do NOT commit — leave the offset so
			// the record is redelivered (never lose it).
			c.logger.ErrorContext(ctx, "record neither applied nor DLQ'd; not committing",
				slog.Any("err", perr))
			continue
		}
		if err := reader.Commit(ctx, rec); err != nil {
			c.logger.WarnContext(ctx, "commit offset failed; record may redeliver", slog.Any("err", err))
		}
	}
}

func (c *ListingConsumer) processWithRetry(ctx context.Context, rec Record, dlq DeadLetterProducer, cfg RunConfig) error {
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := c.HandleRaw(ctx, rec.Value)
		if err == nil {
			return nil
		}
		lastErr = err
		if errors.Is(err, ErrPermanent) {
			break // no point retrying a poison record
		}
		if attempt < cfg.MaxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(cfg.BaseBackoff * time.Duration(attempt)):
			}
		}
	}
	c.logger.WarnContext(ctx, "routing record to DLQ",
		slog.String("dlq_topic", cfg.DLQTopic), slog.Any("err", lastErr))
	if err := dlq.Produce(ctx, cfg.DLQTopic, rec); err != nil {
		return fmt.Errorf("produce to DLQ: %w (original: %v)", err, lastErr)
	}
	return nil
}
