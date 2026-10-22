// Package store holds team-referral's persistence layer. It owns three
// small tables — referral_codes (one code per user), referrals (who redeemed
// whose code), and referral_rewards (mock coin-credit ledger) — behind a single
// ReferralRepository interface. Both a Postgres and an in-memory implementation
// satisfy it, so the service layer and its tests never depend on a live DB.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
)

var (
	// ErrCodeNotFound is returned when a user has no referral code yet, or when a
	// redeem targets a code that does not exist.
	ErrCodeNotFound = errors.New("referral code not found")
	// ErrAlreadyRedeemed is returned when a referee tries to redeem a second code
	// (a user can be referred at most once).
	ErrAlreadyRedeemed = errors.New("user has already redeemed a referral code")
)

// ReferralRepository owns the referral program's storage. Codes are one-per-user
// (user_id PK, code unique); referrals attribute a referee to a referrer exactly
// once; rewards are an append-only mock-coin ledger keyed by user.
type ReferralRepository interface {
	// GetCodeByUser returns the caller's referral code, or ErrCodeNotFound when
	// they have not minted one yet.
	GetCodeByUser(ctx context.Context, userID string) (string, error)
	// CreateCode mints code for userID. Idempotent: if the user already has a
	// code the stored one is returned unchanged (the passed code is ignored).
	CreateCode(ctx context.Context, userID, code string) (string, error)
	// OwnerOfCode returns the user_id that owns code, or ErrCodeNotFound.
	OwnerOfCode(ctx context.Context, code string) (string, error)
	// HasRedeemed reports whether refereeID has already redeemed any code.
	HasRedeemed(ctx context.Context, refereeID string) (bool, error)
	// CreateReferral records that refereeID redeemed code owned by referrerID.
	// Returns ErrAlreadyRedeemed if the referee already redeemed a code.
	CreateReferral(ctx context.Context, referrerID, refereeID, code string) error
	// CountReferralsByReferrer returns how many users referrerID has invited.
	CountReferralsByReferrer(ctx context.Context, referrerID string) (int64, error)
	// AddReward appends a mock coin-credit row for userID and returns it.
	AddReward(ctx context.Context, userID string, amount int64, reason string) (*referralv1.ReferralReward, error)
	// ListRewards returns a user's rewards newest-first, sliced by limit/offset.
	ListRewards(ctx context.Context, userID string, limit, offset int) ([]*referralv1.ReferralReward, error)
	// SumRewards returns the total minor-units credited to userID.
	SumRewards(ctx context.Context, userID string) (int64, error)
}

// NewCode mints a short, human-shareable referral code (uppercased hex).
func NewCode() string {
	return "REF" + uuid.NewString()[:8]
}

func newRewardID() string { return "rwd_" + uuid.NewString()[:8] }

// ── Postgres implementation ──

type PostgresReferralRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresReferralRepo(pool *pgxpool.Pool) *PostgresReferralRepo {
	return &PostgresReferralRepo{pool: pool}
}

func (r *PostgresReferralRepo) GetCodeByUser(ctx context.Context, userID string) (string, error) {
	var code string
	err := r.pool.QueryRow(ctx, `SELECT code FROM referral_codes WHERE user_id = $1`, userID).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCodeNotFound
	}
	if err != nil {
		return "", err
	}
	return code, nil
}

func (r *PostgresReferralRepo) CreateCode(ctx context.Context, userID, code string) (string, error) {
	// One code per user: on a repeat call the existing row wins and RETURNING
	// yields the stored code, so minting is idempotent.
	const q = `
		INSERT INTO referral_codes (user_id, code)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING code`
	var stored string
	if err := r.pool.QueryRow(ctx, q, userID, code).Scan(&stored); err != nil {
		return "", err
	}
	return stored, nil
}

func (r *PostgresReferralRepo) OwnerOfCode(ctx context.Context, code string) (string, error) {
	var userID string
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM referral_codes WHERE code = $1`, code).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCodeNotFound
	}
	if err != nil {
		return "", err
	}
	return userID, nil
}

func (r *PostgresReferralRepo) HasRedeemed(ctx context.Context, refereeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referrals WHERE referee_id = $1)`, refereeID).Scan(&exists)
	return exists, err
}

func (r *PostgresReferralRepo) CreateReferral(ctx context.Context, referrerID, refereeID, code string) error {
	// referee_id is unique, so a second redeem by the same user violates the
	// constraint; translate that into the domain error.
	const q = `
		INSERT INTO referrals (referrer_id, referee_id, code, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (referee_id) DO NOTHING`
	tag, err := r.pool.Exec(ctx, q, referrerID, refereeID, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAlreadyRedeemed
	}
	return nil
}

func (r *PostgresReferralRepo) CountReferralsByReferrer(ctx context.Context, referrerID string) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM referrals WHERE referrer_id = $1`, referrerID).Scan(&n)
	return n, err
}

func (r *PostgresReferralRepo) AddReward(ctx context.Context, userID string, amount int64, reason string) (*referralv1.ReferralReward, error) {
	id := newRewardID()
	var createdAt time.Time
	const q = `
		INSERT INTO referral_rewards (id, user_id, amount, reason, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING created_at`
	if err := r.pool.QueryRow(ctx, q, id, userID, amount, reason).Scan(&createdAt); err != nil {
		return nil, err
	}
	return &referralv1.ReferralReward{
		Id:        id,
		Amount:    amount,
		Reason:    reason,
		CreatedAt: timestamppb.New(createdAt),
	}, nil
}

func (r *PostgresReferralRepo) ListRewards(ctx context.Context, userID string, limit, offset int) ([]*referralv1.ReferralReward, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, amount, reason, created_at
		FROM referral_rewards
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*referralv1.ReferralReward
	for rows.Next() {
		var rw referralv1.ReferralReward
		var createdAt time.Time
		if err := rows.Scan(&rw.Id, &rw.Amount, &rw.Reason, &createdAt); err != nil {
			return nil, err
		}
		rw.CreatedAt = timestamppb.New(createdAt)
		out = append(out, &rw)
	}
	return out, rows.Err()
}

func (r *PostgresReferralRepo) SumRewards(ctx context.Context, userID string) (int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(SUM(amount), 0) FROM referral_rewards WHERE user_id = $1`, userID).Scan(&total)
	return total, err
}
