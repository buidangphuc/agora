package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
)

const (
	erasedAuthorID   = "erased"
	erasedAuthorName = "Người dùng đã xóa"
)

func (r *PostgresRepository) Erase(ctx context.Context, userID string) (int64, error) {
	if userID == "" {
		return 0, errors.New("erase: empty user id")
	}
	var total int64
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		steps := []struct {
			name string
			sql  string
		}{
			{"clear moderator", `UPDATE reviews SET hidden_by = '' WHERE hidden_by = $1`},
			{"anonymise reviews", `UPDATE reviews SET user_id = $2, user_name = $3, hide_reason = '' WHERE user_id = $1`},
			{"anonymise questions", `UPDATE product_questions SET user_id = $2 WHERE user_id = $1`},
			{"anonymise answers", `UPDATE product_answers SET user_id = $2 WHERE user_id = $1`},
			{"delete helpful votes", `DELETE FROM review_helpful_votes WHERE user_id = $1`},
			{"delete favorites", `DELETE FROM favorites WHERE user_id = $1`},
			{"delete follows", `DELETE FROM follows WHERE user_id = $1 OR seller_id = $1`},
			{"delete reports", `DELETE FROM reports WHERE reporter_id = $1`},
			{"delete collections", `DELETE FROM collections WHERE user_id = $1`},
			{"delete history", `DELETE FROM view_history WHERE user_id = $1`},
			{"delete checkins", `DELETE FROM checkins WHERE user_id = $1`},
			{"delete loyalty", `DELETE FROM loyalty_accounts WHERE user_id = $1`},
		}
		for _, s := range steps {
			var (
				tag pgconn.CommandTag
				err error
			)
			if strings.Contains(s.sql, "$3") {
				tag, err = tx.Exec(ctx, s.sql, userID, erasedAuthorID, erasedAuthorName)
			} else if strings.Contains(s.sql, "$2") {
				tag, err = tx.Exec(ctx, s.sql, userID, erasedAuthorID)
			} else {
				tag, err = tx.Exec(ctx, s.sql, userID)
			}
			if err != nil {
				return fmt.Errorf("erase %s: %w", s.name, err)
			}
			total += tag.RowsAffected()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}
