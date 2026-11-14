package store

import (
	"context"
	"fmt"
)

const MaskedText = "Đã xóa"

type PayoutObligations struct {
	Available int64
	Held      int64
	Pending   int32
}

type ObligationsReader interface {
	PayoutObligations(ctx context.Context, userID string, hold Holdback) (PayoutObligations, error)
}

func (r *PostgresLedgerRepository) PayoutObligations(ctx context.Context, userID string, hold Holdback) (PayoutObligations, error) {
	balance, err := r.Balance(ctx, userID)
	if err != nil {
		return PayoutObligations{}, err
	}
	credits, err := loadHeldCredits(ctx, r.pool, userID, hold)
	if err != nil {
		return PayoutObligations{}, err
	}
	w := computeWithdrawal(balance, credits)
	out := PayoutObligations{Available: w.Withdrawable, Held: w.Held}
	const q = `SELECT COUNT(*) FROM payout_requests WHERE seller_id = $1 AND status IN (1, 2)`
	if err := r.pool.QueryRow(ctx, q, userID).Scan(&out.Pending); err != nil {
		return PayoutObligations{}, fmt.Errorf("count pending payouts: %w", err)
	}
	return out, nil
}

func (r *PostgresWalletRepository) EraseSellerPayouts(ctx context.Context, userID string) error {
	const q = `UPDATE payout_requests SET account_number = $2, account_name = $2
		WHERE seller_id = $1 AND (account_number <> $2 OR account_name <> $2)`
	if _, err := r.pool.Exec(ctx, q, userID, MaskedText); err != nil {
		return fmt.Errorf("mask seller payouts: %w", err)
	}
	return nil
}

var _ ObligationsReader = (*PostgresLedgerRepository)(nil)
