package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
)

const (
	erasedMessageText = "Tin nhắn đã bị xóa"
	erasedSenderName  = "Người dùng đã xóa"
)

func (r *PostgresChatRepository) Erase(ctx context.Context, userID string) (int64, error) {
	if userID == "" {
		return 0, errors.New("erase: empty user id")
	}
	var total int64
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE chat_messages SET content = $2, sender_name = $3, payload = ''
			WHERE sender_id = $1 AND (content <> $2 OR sender_name <> $3 OR payload <> '')`,
			userID, erasedMessageText, erasedSenderName)
		if err != nil {
			return fmt.Errorf("erase messages: %w", err)
		}
		total = tag.RowsAffected()
		if _, err := tx.Exec(ctx, `UPDATE chat_threads t SET last_message_text = $2
			WHERE (t.buyer_id = $1 OR t.seller_id = $1)
			AND t.last_message_text <> $2
			AND (SELECT m.sender_id FROM chat_messages m WHERE m.thread_id = t.id
			     ORDER BY m.created_at DESC, m.id DESC LIMIT 1) = $1`, userID, erasedMessageText); err != nil {
			return fmt.Errorf("erase thread previews: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}
