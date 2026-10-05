package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/protobuf/proto"

	chatv1 "github.com/buidangphuc/team-notification/generated/platform/chat/v1"
	eventsv1 "github.com/buidangphuc/team-notification/generated/platform/events/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	orderv1 "github.com/buidangphuc/team-notification/generated/platform/order/v1"
)

// EventEnvelope.type discriminators for the events the user-facing consumers
// react to (ADR-0005). chat.events carries the ChatMessage itself.
const (
	chatMessageType  = "platform.chat.v1.ChatMessage"
	orderShippedType = "platform.order.v1.OrderShipped"

	chatConsumerName  = "team-notification.chat"
	orderConsumerName = "team-notification.order"

	// chatBodyMaxRunes is how much of the message the notification body shows.
	chatBodyMaxRunes = 120
)

// PrefsReader is the slice of the preferences use case the consumers need.
// *service.PrefsService satisfies it (it returns defaults for a user with none).
type PrefsReader interface {
	Get(ctx context.Context, userID string) (*notificationv1.NotificationPrefs, error)
}

// userEventHandler holds what the chat and order consumers share: dedupe by
// event_id, preference gating and creating one notification for one user.
type userEventHandler struct {
	name   string
	notif  NotificationCreator
	prefs  PrefsReader
	dedupe Deduper
	logger *slog.Logger
}

// UserEventOption customizes a ChatConsumer or OrderConsumer.
type UserEventOption func(*userEventHandler)

// WithUserEventDeduper injects a durable dedupe ledger (default: in-memory).
func WithUserEventDeduper(d Deduper) UserEventOption {
	return func(h *userEventHandler) {
		if d != nil {
			h.dedupe = d
		}
	}
}

func newUserEventHandler(name string, notif NotificationCreator, prefs PrefsReader, logger *slog.Logger, opts []UserEventOption) userEventHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := userEventHandler{name: name, notif: notif, prefs: prefs, dedupe: NewInMemoryDeduper(), logger: logger}
	for _, o := range opts {
		o(&h)
	}
	return h
}

// decode unmarshals a record value into an envelope; a malformed one is poison.
func decodeEnvelope(value []byte) (*eventsv1.EventEnvelope, error) {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return nil, fmt.Errorf("%w: unmarshal envelope: %v", ErrPermanent, err)
	}
	return &env, nil
}

// apply runs effect once per event_id: the ledger short-circuits a redelivery, and
// the event is recorded only AFTER the effect succeeded, so a crash in between
// redelivers instead of losing the notification.
func (h *userEventHandler) apply(ctx context.Context, eventID string, effect func() error) error {
	if eventID == "" {
		return fmt.Errorf("%w: envelope missing event_id", ErrPermanent)
	}
	already, err := h.dedupe.IsProcessed(ctx, h.name, eventID)
	if err != nil {
		return fmt.Errorf("dedupe lookup: %w", err)
	}
	if already {
		return nil
	}
	if err := effect(); err != nil {
		return err
	}
	if _, err := h.dedupe.MarkProcessed(ctx, h.name, eventID); err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}
	return nil
}

// notify creates n for n.UserId unless that user switched n.Type off. A type with
// no stored setting counts as enabled.
func (h *userEventHandler) notify(ctx context.Context, n *notificationv1.Notification) error {
	prefs, err := h.prefs.Get(ctx, n.GetUserId())
	if err != nil {
		return fmt.Errorf("read prefs for user %q: %w", n.GetUserId(), err)
	}
	if enabled, ok := prefs.GetTypeEnabled()[n.GetType().String()]; ok && !enabled {
		h.logger.InfoContext(ctx, "notification skipped: type disabled by user",
			slog.String("user_id", n.GetUserId()), slog.String("type", n.GetType().String()))
		return nil
	}
	if _, err := h.notif.CreateNotification(ctx, n); err != nil {
		return fmt.Errorf("create notification for user %q: %w", n.GetUserId(), err)
	}
	h.logger.InfoContext(ctx, "notification created",
		slog.String("user_id", n.GetUserId()), slog.String("type", n.GetType().String()))
	return nil
}

// ── chat.events ──

// ChatConsumer turns a sent chat message into one CHAT notification for the thread
// participant who did not send it. team-chat puts that participant in
// ChatMessage.recipient_id, so no call back to team-chat is needed.
type ChatConsumer struct{ h userEventHandler }

// NewChatConsumer builds the chat.events consumer.
func NewChatConsumer(notif NotificationCreator, prefs PrefsReader, logger *slog.Logger, opts ...UserEventOption) *ChatConsumer {
	return &ChatConsumer{h: newUserEventHandler(chatConsumerName, notif, prefs, logger, opts)}
}

// HandleRaw applies one Kafka record value (a marshalled EventEnvelope).
func (c *ChatConsumer) HandleRaw(ctx context.Context, value []byte) error {
	env, err := decodeEnvelope(value)
	if err != nil {
		return err
	}
	return c.HandleEnvelope(ctx, env)
}

// HandleEnvelope applies one decoded envelope; other event types are ignored.
func (c *ChatConsumer) HandleEnvelope(ctx context.Context, env *eventsv1.EventEnvelope) error {
	if env.GetType() != chatMessageType {
		return nil
	}
	return c.h.apply(ctx, env.GetEventId(), func() error {
		var m chatv1.ChatMessage
		if err := proto.Unmarshal(env.GetPayload(), &m); err != nil {
			return fmt.Errorf("%w: unmarshal ChatMessage: %v", ErrPermanent, err)
		}
		if m.GetThreadId() == "" {
			return fmt.Errorf("%w: ChatMessage missing thread_id", ErrPermanent)
		}
		recipient := m.GetRecipientId()
		if recipient == "" {
			// Published before recipient_id existed: nobody to notify.
			c.h.logger.WarnContext(ctx, "chat event without recipient_id; skipping",
				slog.String("thread_id", m.GetThreadId()))
			return nil
		}
		if recipient == m.GetSenderId() {
			return nil // never notify a sender of their own message
		}
		label := strings.TrimSpace(m.GetSenderName())
		if label == "" {
			label = "người dùng"
		}
		return c.h.notify(ctx, &notificationv1.Notification{
			UserId:  recipient,
			Title:   "Tin nhắn mới từ " + label,
			Body:    truncateRunes(m.GetContent(), chatBodyMaxRunes),
			Type:    notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT,
			LinkUrl: "/chat/" + m.GetThreadId(),
		})
	})
}

// Run consumes chat.events until ctx is cancelled.
func (c *ChatConsumer) Run(ctx context.Context, reader RecordReader, dlq DeadLetterProducer, cfg RunConfig) error {
	return runLoop(ctx, c.h.logger, c.HandleRaw, reader, dlq, cfg)
}

// ── order.events ──

// OrderConsumer turns OrderShipped into one ORDER notification for the buyer. Every
// other order.events type (OrderPaidEvent, ...) is ignored.
type OrderConsumer struct{ h userEventHandler }

// NewOrderConsumer builds the order.events consumer.
func NewOrderConsumer(notif NotificationCreator, prefs PrefsReader, logger *slog.Logger, opts ...UserEventOption) *OrderConsumer {
	return &OrderConsumer{h: newUserEventHandler(orderConsumerName, notif, prefs, logger, opts)}
}

// HandleRaw applies one Kafka record value (a marshalled EventEnvelope).
func (c *OrderConsumer) HandleRaw(ctx context.Context, value []byte) error {
	env, err := decodeEnvelope(value)
	if err != nil {
		return err
	}
	return c.HandleEnvelope(ctx, env)
}

// HandleEnvelope applies one decoded envelope; other event types are ignored.
func (c *OrderConsumer) HandleEnvelope(ctx context.Context, env *eventsv1.EventEnvelope) error {
	if env.GetType() != orderShippedType {
		return nil
	}
	return c.h.apply(ctx, env.GetEventId(), func() error {
		var e orderv1.OrderShipped
		if err := proto.Unmarshal(env.GetPayload(), &e); err != nil {
			return fmt.Errorf("%w: unmarshal OrderShipped: %v", ErrPermanent, err)
		}
		if e.GetOrderId() == "" || e.GetBuyerId() == "" {
			return fmt.Errorf("%w: OrderShipped missing order_id or buyer_id", ErrPermanent)
		}
		carrier := strings.TrimSpace(e.GetCarrier())
		if carrier == "" {
			carrier = "đơn vị vận chuyển"
		}
		return c.h.notify(ctx, &notificationv1.Notification{
			UserId:  e.GetBuyerId(),
			Title:   "Đơn hàng đã được giao cho " + carrier,
			Body:    "Mã vận đơn: " + e.GetTrackingCode(),
			Type:    notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER,
			LinkUrl: "/account/orders/" + e.GetOrderId(),
		})
	})
}

// Run consumes order.events until ctx is cancelled.
func (c *OrderConsumer) Run(ctx context.Context, reader RecordReader, dlq DeadLetterProducer, cfg RunConfig) error {
	return runLoop(ctx, c.h.logger, c.HandleRaw, reader, dlq, cfg)
}

// truncateRunes returns the first max runes of s (not bytes: Vietnamese text).
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
