package consumer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"

	"github.com/buidangphuc/team-promotion/internal/store"
)

const (
	voucherScopePlatform int32 = 2
	discountTypeFixed    int32 = 2
	codeRandomBytes            = 6
)

func newCode() (string, error) {
	b := make([]byte, codeRandomBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate voucher code: %w", err)
	}
	return "NU-" + strings.ToUpper(hex.EncodeToString(b)), nil
}

func issue(ctx context.Context, tx pgx.Tx, cfg Config, userID string, amount int64, now time.Time) error {
	code, err := newCode()
	if err != nil {
		return err
	}
	const ins = `INSERT INTO vouchers (id, code, scope, discount_type, discount_value, min_spend, quota, starts_at, ends_at,
			owner_user_id, kind, per_user_limit, new_user_only)
		VALUES ($1, $2, $3, $4, $5, $6, 1, $7, $8, $9, $10, 1, false)
		ON CONFLICT (owner_user_id) WHERE kind = 2 DO NOTHING`
	if _, err := tx.Exec(ctx, ins, store.NewID(), code, voucherScopePlatform, discountTypeFixed, amount, cfg.MinSpend,
		now, now.Add(cfg.VoucherTTL), userID, store.VoucherKindNewUser); err != nil {
		return fmt.Errorf("issue new-user voucher: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE new_user_voucher_state SET issued_at = $2, updated_at = $2 WHERE user_id = $1`, userID, now); err != nil {
		return fmt.Errorf("mark voucher issued: %w", err)
	}
	cfg.logger().InfoContext(ctx, "new-user voucher issued", "key_fp", logx.PrincipalFP(userID), "amount", amount)
	return nil
}
