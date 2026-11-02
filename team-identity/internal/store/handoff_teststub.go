package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrHandoffCapReached = errors.New("handoff cap reached")
	ErrHandoffInvalid    = errors.New("handoff code invalid")
)

type HandoffCode struct {
	CodeHash   []byte
	UserID     string
	SourceSID  string
	TargetApp  string
	ExpiresAt  time.Time
	RedeemedAt time.Time
	CreatedAt  time.Time
}

type PostgresHandoffRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresHandoffRepository(pool *pgxpool.Pool) *PostgresHandoffRepository {
	return &PostgresHandoffRepository{pool: pool}
}

var errTestStub = errors.New("TEST-STUB: handoff store not implemented")

func (r *PostgresHandoffRepository) InsertHandoffCode(ctx context.Context, c HandoffCode, maxLive int, now time.Time, inTx func(context.Context) error) error {
	return errTestStub
}

func (r *PostgresHandoffRepository) RedeemHandoffCode(ctx context.Context, codeHash []byte, targetApp string, now time.Time, inTx func(context.Context, HandoffCode) error) (HandoffCode, error) {
	return HandoffCode{}, errTestStub
}

func (r *PostgresHandoffRepository) PeekHandoffCode(ctx context.Context, codeHash []byte, targetApp string, now time.Time) (HandoffCode, error) {
	return HandoffCode{}, errTestStub
}

func (r *PostgresHandoffRepository) CountLiveHandoffCodes(ctx context.Context, userID string, now time.Time) (int, error) {
	return 0, errTestStub
}

func (r *PostgresHandoffRepository) PurgeHandoffCodes(ctx context.Context, now time.Time, limit int) (int, error) {
	return 0, errTestStub
}
