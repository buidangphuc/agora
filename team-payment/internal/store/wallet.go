package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrPayoutNotFound      = errors.New("payout request not found")
	ErrInsufficientBalance = errors.New("insufficient wallet balance")
	ErrInvalidAmount       = errors.New("amount must be positive")
)

type PayoutStatus int32

const (
	PayoutStatusUnspecified PayoutStatus = 0
	PayoutStatusPending     PayoutStatus = 1
	PayoutStatusProcessing  PayoutStatus = 2
	PayoutStatusCompleted   PayoutStatus = 3
	PayoutStatusRejected    PayoutStatus = 4
)

// SellerWallet is a read model: Balance is always SUM(wallet_ledger.amount) for the
// seller. The legacy seller_wallets table is no longer read or written.
type SellerWallet struct {
	ID        string
	SellerID  string
	Balance   int64
	Currency  string
	UpdatedAt time.Time
}

type PayoutRequest struct {
	ID            string
	SellerID      string
	Amount        int64
	BankCode      string
	AccountNumber string
	AccountName   string
	Status        PayoutStatus
	// LedgerEntryID links the request to the wallet_ledger debit that funded it.
	LedgerEntryID string
	CreatedAt     time.Time
}

// WalletRepository stores payout requests (bank details + status). Money lives only
// in the wallet ledger (LedgerRepository); this port never holds a balance.
type WalletRepository interface {
	CreatePayoutRequest(ctx context.Context, payout PayoutRequest) (PayoutRequest, error)
	GetPayoutRequest(ctx context.Context, id string) (PayoutRequest, error)
	ListPayoutRequestsBySellerID(ctx context.Context, sellerID string) ([]PayoutRequest, error)
}

// ── Postgres Implementation ──────────────────────────────────────────

type PostgresWalletRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresWalletRepository(pool *pgxpool.Pool) *PostgresWalletRepository {
	return &PostgresWalletRepository{pool: pool}
}

const payoutColumns = `id, seller_id, amount, bank_code, account_number, account_name, status, COALESCE(ledger_entry_id, ''), created_at`

func scanPayout(row pgx.Row, p *PayoutRequest) error {
	var statusInt int32
	if err := row.Scan(&p.ID, &p.SellerID, &p.Amount, &p.BankCode, &p.AccountNumber, &p.AccountName, &statusInt, &p.LedgerEntryID, &p.CreatedAt); err != nil {
		return err
	}
	p.Status = PayoutStatus(statusInt)
	return nil
}

func (r *PostgresWalletRepository) CreatePayoutRequest(ctx context.Context, p PayoutRequest) (PayoutRequest, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Status == 0 {
		p.Status = PayoutStatusPending
	}
	p.CreatedAt = time.Now()

	const q = `INSERT INTO payout_requests (id, seller_id, amount, bank_code, account_number, account_name, status, ledger_entry_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9)`
	if _, err := r.pool.Exec(ctx, q, p.ID, p.SellerID, p.Amount, p.BankCode, p.AccountNumber, p.AccountName, int32(p.Status), p.LedgerEntryID, p.CreatedAt); err != nil {
		return PayoutRequest{}, fmt.Errorf("insert payout request: %w", err)
	}
	return p, nil
}

func (r *PostgresWalletRepository) GetPayoutRequest(ctx context.Context, id string) (PayoutRequest, error) {
	const q = `SELECT ` + payoutColumns + ` FROM payout_requests WHERE id = $1`
	var p PayoutRequest
	if err := scanPayout(r.pool.QueryRow(ctx, q, id), &p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PayoutRequest{}, ErrPayoutNotFound
		}
		return PayoutRequest{}, fmt.Errorf("get payout request: %w", err)
	}
	return p, nil
}

func (r *PostgresWalletRepository) ListPayoutRequestsBySellerID(ctx context.Context, sellerID string) ([]PayoutRequest, error) {
	const q = `SELECT ` + payoutColumns + ` FROM payout_requests WHERE seller_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, sellerID)
	if err != nil {
		return nil, fmt.Errorf("list payout requests: %w", err)
	}
	defer rows.Close()

	var result []PayoutRequest
	for rows.Next() {
		var p PayoutRequest
		if err := scanPayout(rows, &p); err != nil {
			return nil, fmt.Errorf("scan payout request: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
