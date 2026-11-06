package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-referral/internal/events"
)

func markProcessed(ctx context.Context, tx pgx.Tx, eventID string, at time.Time) (bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO referral_processed_events (event_id, processed_at)
		VALUES ($1, $2) ON CONFLICT (event_id) DO NOTHING`, eventID, at)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func lockCode(ctx context.Context, tx pgx.Tx, userID string) error {
	var one int
	err := tx.QueryRow(ctx, `SELECT 1 FROM referral_codes WHERE user_id = $1 FOR UPDATE`, userID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCodeNotFound
	}
	return err
}

func lockOwner(ctx context.Context, tx pgx.Tx, code string) (string, error) {
	var owner string
	err := tx.QueryRow(ctx, `SELECT user_id FROM referral_codes WHERE code = $1 FOR UPDATE`, code).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCodeNotFound
	}
	return owner, err
}

func countLive(ctx context.Context, tx pgx.Tx, userID string) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM referral_rewards WHERE user_id = $1 AND status <> 'VOID'`, userID).Scan(&n)
	return n, err
}

func hasReferral(ctx context.Context, tx pgx.Tx, refereeID string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referrals WHERE referee_id = $1)`, refereeID).Scan(&exists)
	return exists, err
}

func insertReferral(ctx context.Context, tx pgx.Tx, referrerID, refereeID, code string, at time.Time) (bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO referrals (referrer_id, referee_id, code, created_at, status, attributed_at)
		VALUES ($1, $2, $3, $4, 'ATTRIBUTED', $4)
		ON CONFLICT (referee_id) DO NOTHING`, referrerID, refereeID, code, at)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *PostgresReferralRepo) enqueue(ctx context.Context, tx pgx.Tx, build func() (outbox.Row, error)) error {
	row, err := build()
	if err != nil {
		return err
	}
	return r.outbox.EnqueueTx(ctx, tx, row)
}

func (r *PostgresReferralRepo) ApplyUserRegistered(ctx context.Context, in UserRegisteredInput) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		fresh, err := markProcessed(ctx, tx, in.EventID, in.At)
		if err != nil || !fresh {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO referral_accounts (user_id, registered_at) VALUES ($1, $2)
			ON CONFLICT (user_id) DO UPDATE SET registered_at = EXCLUDED.registered_at`, in.UserID, in.RegisteredAt); err != nil {
			return err
		}
		if in.ReferralCode == "" {
			return nil
		}
		already, err := hasReferral(ctx, tx, in.UserID)
		if err != nil || already {
			return err
		}
		referrer, vErr := r.verdict(ctx, tx, in)
		if vErr != nil {
			reason, rejected := RejectReason(vErr)
			if !rejected {
				return vErr
			}
			return r.enqueue(ctx, tx, func() (outbox.Row, error) { return events.Rejected(in.UserID, reason, in.At) })
		}
		inserted, err := insertReferral(ctx, tx, referrer, in.UserID, in.ReferralCode, in.At)
		if err != nil || !inserted {
			return err
		}
		return r.enqueue(ctx, tx, func() (outbox.Row, error) { return events.Attributed(in.UserID, referrer, in.At) })
	})
}

func (r *PostgresReferralRepo) verdict(ctx context.Context, tx pgx.Tx, in UserRegisteredInput) (string, error) {
	owner, err := lockOwner(ctx, tx, in.ReferralCode)
	if err != nil {
		return "", err
	}
	if owner == in.UserID {
		return "", ErrSelfReferral
	}
	n, err := countLive(ctx, tx, owner)
	if err != nil {
		return "", err
	}
	if n >= in.LifetimeCap {
		return "", ErrReferrerCapped
	}
	return owner, nil
}

func (r *PostgresReferralRepo) Redeem(ctx context.Context, in RedeemInput) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		owner, err := lockOwner(ctx, tx, in.Code)
		if err != nil {
			return err
		}
		if owner == in.RefereeID {
			return ErrSelfReferral
		}
		already, err := hasReferral(ctx, tx, in.RefereeID)
		if err != nil {
			return err
		}
		if already {
			return ErrAlreadyRedeemed
		}
		var registeredAt time.Time
		err = tx.QueryRow(ctx, `SELECT registered_at FROM referral_accounts WHERE user_id = $1`, in.RefereeID).Scan(&registeredAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotEligible
		}
		if err != nil {
			return err
		}
		if in.At.Sub(registeredAt) >= in.MaxAccountAge {
			return ErrNotEligible
		}
		var ordered bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referral_buyer_orders WHERE buyer_id = $1)`, in.RefereeID).Scan(&ordered); err != nil {
			return err
		}
		if ordered {
			return ErrNotEligible
		}
		n, err := countLive(ctx, tx, owner)
		if err != nil {
			return err
		}
		if n >= in.LifetimeCap {
			return ErrReferrerCapped
		}
		inserted, err := insertReferral(ctx, tx, owner, in.RefereeID, in.Code, in.At)
		if err != nil {
			return err
		}
		if !inserted {
			return ErrAlreadyRedeemed
		}
		return r.enqueue(ctx, tx, func() (outbox.Row, error) { return events.Attributed(in.RefereeID, owner, in.At) })
	})
}

func (r *PostgresReferralRepo) ApplyOrderCompleted(ctx context.Context, in OrderCompletedInput) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		fresh, err := markProcessed(ctx, tx, in.EventID, in.At)
		if err != nil || !fresh {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO referral_buyer_orders (buyer_id, first_completed_at) VALUES ($1, $2)
			ON CONFLICT (buyer_id) DO UPDATE
			SET first_completed_at = LEAST(referral_buyer_orders.first_completed_at, EXCLUDED.first_completed_at)`,
			in.BuyerID, in.CompletedAt); err != nil {
			return err
		}
		var referrer string
		err = tx.QueryRow(ctx, `SELECT referrer_id FROM referrals WHERE referee_id = $1`, in.BuyerID).Scan(&referrer)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := lockCode(ctx, tx, referrer); err != nil {
			return err
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM referrals WHERE referee_id = $1 FOR UPDATE`, in.BuyerID).Scan(&status); err != nil {
			return err
		}
		if status != StatusAttributed {
			return nil
		}
		n, err := countLive(ctx, tx, referrer)
		if err != nil {
			return err
		}
		if n >= in.LifetimeCap {
			_, err := tx.Exec(ctx, `UPDATE referrals SET status = 'CAPPED' WHERE referee_id = $1`, in.BuyerID)
			return err
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO referral_rewards (id, user_id, amount, reason, created_at, status, release_at, order_id, referee_id)
			VALUES ($1, $2, $3, $4, $5, 'PENDING', $6, $7, $8)
			ON CONFLICT (order_id) WHERE order_id IS NOT NULL DO NOTHING`,
			NewRewardID(), referrer, in.RewardXU, ReasonFirstOrder, in.At, in.ReleaseAt, in.OrderID, in.BuyerID)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE referrals SET status = 'QUALIFIED' WHERE referee_id = $1`, in.BuyerID)
		return err
	})
}

func (r *PostgresReferralRepo) ApplyReturnRequested(ctx context.Context, in ReturnRequestedInput) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		fresh, err := markProcessed(ctx, tx, in.EventID, in.At)
		if err != nil || !fresh {
			return err
		}
		var referrer string
		err = tx.QueryRow(ctx, `SELECT user_id FROM referral_rewards WHERE order_id = $1`, in.OrderID).Scan(&referrer)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := lockCode(ctx, tx, referrer); err != nil {
			return err
		}
		var referee *string
		err = tx.QueryRow(ctx, `
			UPDATE referral_rewards SET status = 'VOID'
			WHERE order_id = $1 AND status = 'PENDING'
			RETURNING referee_id`, in.OrderID).Scan(&referee)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if referee == nil {
			return nil
		}
		_, err = tx.Exec(ctx, `UPDATE referrals SET status = 'ATTRIBUTED' WHERE referee_id = $1 AND status = 'QUALIFIED'`, *referee)
		return err
	})
}

type dueReward struct {
	id, referrer string
}

func (r *PostgresReferralRepo) ReleaseDue(ctx context.Context, in ReleaseInput) (int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id FROM referral_rewards
		WHERE status = 'PENDING' AND release_at <= $1
		ORDER BY release_at, id`, in.At)
	if err != nil {
		return 0, err
	}
	var due []dueReward
	for rows.Next() {
		var d dueReward
		if err := rows.Scan(&d.id, &d.referrer); err != nil {
			rows.Close()
			return 0, err
		}
		due = append(due, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	released := 0
	var errs []error
	for _, d := range due {
		ok, err := r.releaseOne(ctx, d, in)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if ok {
			released++
		}
	}
	return released, errors.Join(errs...)
}

func (r *PostgresReferralRepo) releaseOne(ctx context.Context, d dueReward, in ReleaseInput) (released bool, err error) {
	err = r.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockCode(ctx, tx, d.referrer); err != nil {
			return err
		}
		var referee *string
		var amount int64
		err := tx.QueryRow(ctx, `
			SELECT referee_id, amount FROM referral_rewards
			WHERE id = $1 AND status = 'PENDING' FOR UPDATE`, d.id).Scan(&referee, &amount)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		start, end := DayBounds(in.At, in.Location)
		var today, total int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE released_at >= $2 AND released_at < $3), count(*)
			FROM referral_rewards WHERE user_id = $1 AND status = 'RELEASED'`, d.referrer, start, end).Scan(&today, &total); err != nil {
			return err
		}
		if today >= in.DailyCap || total >= in.LifetimeCap {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE referral_rewards SET status = 'RELEASED', released_at = $2 WHERE id = $1 AND status = 'PENDING'`, d.id, in.At); err != nil {
			return err
		}
		refereeID := ""
		if referee != nil {
			refereeID = *referee
			if _, err := tx.Exec(ctx, `UPDATE referrals SET status = 'REWARDED' WHERE referee_id = $1`, refereeID); err != nil {
				return err
			}
		}
		if err := r.enqueue(ctx, tx, func() (outbox.Row, error) {
			return events.Released(d.id, d.referrer, refereeID, amount, in.At)
		}); err != nil {
			return err
		}
		released = true
		return nil
	})
	if err != nil {
		released = false
	}
	return released, err
}
