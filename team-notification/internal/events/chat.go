package events

import (
	"context"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	chatv1 "github.com/buidangphuc/team-notification/generated/platform/chat/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

const (
	chatMessageType    = "platform.chat.v1.ChatMessage"
	chatBodyMaxRunes   = 120
	neutralSenderLabel = "Người dùng"
)

func NewChatHandler(pool TxBeginner, names SenderNames, logger *slog.Logger) consume.Handler {
	logger = orDefault(logger)
	return func(ctx context.Context, r consume.Record) error {
		env, err := decode(r.Value)
		if err != nil {
			return err
		}
		if env.GetType() != chatMessageType {
			return nil
		}
		return apply(ctx, pool, chatConsumerName, env.GetEventId(), func(tx pgx.Tx) error {
			var m chatv1.ChatMessage
			if err := unmarshal(env.GetPayload(), &m); err != nil {
				return err
			}
			if m.GetThreadId() == "" {
				return consume.Permanent(errMissing("ChatMessage thread_id"))
			}
			recipient := m.GetRecipientId()
			if recipient == "" {
				logger.WarnContext(ctx, "chat event without recipient_id; skipping", "thread_id", m.GetThreadId())
				return nil
			}
			if recipient == m.GetSenderId() {
				return nil
			}
			label := neutralSenderLabel
			if names != nil {
				if name := strings.TrimSpace(names.ResolveSenderName(ctx, m.GetSenderId(), m.GetSellerId())); name != "" {
					label = name
				}
			}
			return notifyRespectingPrefs(ctx, tx, logger, &notificationv1.Notification{
				UserId:  recipient,
				Title:   "Tin nhắn mới từ " + label,
				Body:    truncateRunes(m.GetContent(), chatBodyMaxRunes),
				Type:    notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT,
				LinkUrl: "/chat/" + m.GetThreadId(),
			})
		})
	}
}

func truncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}
