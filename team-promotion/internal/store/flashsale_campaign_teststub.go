package store

import (
	"context"
	"errors"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
)

const (
	CampaignStatusScheduled = "scheduled"
	CampaignStatusActive    = "active"
	CampaignStatusEnded     = "ended"
	CampaignStatusCancelled = "cancelled"
)

var (
	ErrCampaignEnded        = errors.New("flash sale campaign ended")
	ErrCampaignNotScheduled = errors.New("flash sale campaign not scheduled")
	ErrCampaignCancelled    = errors.New("flash sale campaign cancelled")
	ErrIdempotencyMismatch  = errors.New("idempotency key reused with a different payload")
	ErrIdempotencyNotFound  = errors.New("idempotency record not found")
	errTestStub             = errors.New("teststub: not implemented")
)

type IdempotentCampaign struct {
	Campaign    FlashSaleCampaign
	PayloadHash string
}

func (r *PostgresFlashSaleRepository) ListCampaigns(ctx context.Context, statuses []string, now time.Time, offset, limit int) ([]FlashSaleCampaign, error) {
	return nil, errTestStub
}

func (r *PostgresFlashSaleRepository) Cancel(ctx context.Context, id string, now time.Time) (time.Time, error) {
	return time.Time{}, errTestStub
}

func (r *PostgresFlashSaleRepository) UpdateIfScheduled(ctx context.Context, id string, c FlashSaleCampaign, now time.Time) (FlashSaleCampaign, error) {
	return FlashSaleCampaign{}, errTestStub
}

func (r *PostgresFlashSaleRepository) LookupIdempotent(ctx context.Context, actorID, key string, now time.Time) (IdempotentCampaign, error) {
	return IdempotentCampaign{}, errTestStub
}

func (r *PostgresFlashSaleRepository) CreateIdempotent(ctx context.Context, actorID, key, payloadHash string, c FlashSaleCampaign, now time.Time, event func(FlashSaleCampaign) (outbox.Row, error)) (FlashSaleCampaign, bool, error) {
	return FlashSaleCampaign{}, false, errTestStub
}

func (r *PostgresFlashSaleRepository) PurgeIdempotency(ctx context.Context, now time.Time) (int64, error) {
	return 0, errTestStub
}
