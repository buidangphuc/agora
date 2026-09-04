package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrWalletNotFound       = errors.New("seller wallet not found")
	ErrPayoutNotFound       = errors.New("payout request not found")
	ErrInsufficientBalance  = errors.New("insufficient wallet balance")
	ErrInvalidAmount        = errors.New("amount must be positive")
)

type PayoutStatus int32

const (
	PayoutStatusUnspecified PayoutStatus = 0
	PayoutStatusPending     PayoutStatus = 1
	PayoutStatusProcessing  PayoutStatus = 2
	PayoutStatusCompleted   PayoutStatus = 3
	PayoutStatusRejected    PayoutStatus = 4
)

type WalletTransactionType int32

const (
	WalletTxTypeUnspecified      WalletTransactionType = 0
	WalletTxTypeOrderSettlement  WalletTransactionType = 1
	WalletTxTypePayout           WalletTransactionType = 2
	WalletTxTypeRefundDeduction  WalletTransactionType = 3
)

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
	CreatedAt     time.Time
}

type WalletTransaction struct {
	ID          string
	WalletID    string
	Amount      int64
	Type        WalletTransactionType
	ReferenceID string
	CreatedAt   time.Time
}

type WalletRepository interface {
	GetOrCreateWallet(ctx context.Context, sellerID string) (SellerWallet, error)
	GetWalletBySellerID(ctx context.Context, sellerID string) (SellerWallet, error)
	UpdateWalletBalance(ctx context.Context, sellerID string, delta int64) (SellerWallet, error)
	CreatePayoutRequest(ctx context.Context, payout PayoutRequest) (PayoutRequest, error)
	GetPayoutRequest(ctx context.Context, id string) (PayoutRequest, error)
	ListPayoutRequestsBySellerID(ctx context.Context, sellerID string) ([]PayoutRequest, error)
	CreateWalletTransaction(ctx context.Context, tx WalletTransaction) (WalletTransaction, error)
	ListWalletTransactions(ctx context.Context, walletID string) ([]WalletTransaction, error)
}

// ── Postgres Implementation ──────────────────────────────────────────

type PostgresWalletRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresWalletRepository(pool *pgxpool.Pool) *PostgresWalletRepository {
	return &PostgresWalletRepository{pool: pool}
}

const walletColumns = `id, seller_id, balance, currency, updated_at`

func scanWallet(row pgx.Row, w *SellerWallet) error {
	return row.Scan(&w.ID, &w.SellerID, &w.Balance, &w.Currency, &w.UpdatedAt)
}

func (r *PostgresWalletRepository) GetOrCreateWallet(ctx context.Context, sellerID string) (SellerWallet, error) {
	newID := uuid.NewString()
	const q = `INSERT INTO seller_wallets (id, seller_id, balance, currency, updated_at)
		VALUES ($1, $2, 10000000, 'VND', NOW())
		ON CONFLICT (seller_id) DO UPDATE SET updated_at = seller_wallets.updated_at
		RETURNING ` + walletColumns
	var w SellerWallet
	if err := scanWallet(r.pool.QueryRow(ctx, q, newID, sellerID), &w); err != nil {
		return SellerWallet{}, fmt.Errorf("get or create wallet: %w", err)
	}
	return w, nil
}

func (r *PostgresWalletRepository) GetWalletBySellerID(ctx context.Context, sellerID string) (SellerWallet, error) {
	const q = `SELECT ` + walletColumns + ` FROM seller_wallets WHERE seller_id = $1`
	var w SellerWallet
	if err := scanWallet(r.pool.QueryRow(ctx, q, sellerID), &w); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SellerWallet{}, ErrWalletNotFound
		}
		return SellerWallet{}, fmt.Errorf("get wallet by seller id: %w", err)
	}
	return w, nil
}

func (r *PostgresWalletRepository) UpdateWalletBalance(ctx context.Context, sellerID string, delta int64) (SellerWallet, error) {
	const q = `UPDATE seller_wallets
		SET balance = balance + $1, updated_at = NOW()
		WHERE seller_id = $2 AND (balance + $1) >= 0
		RETURNING ` + walletColumns
	var w SellerWallet
	if err := scanWallet(r.pool.QueryRow(ctx, q, delta, sellerID), &w); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Check if wallet exists
			existing, getErr := r.GetWalletBySellerID(ctx, sellerID)
			if getErr != nil {
				return SellerWallet{}, getErr
			}
			if existing.Balance+delta < 0 {
				return SellerWallet{}, ErrInsufficientBalance
			}
			return SellerWallet{}, ErrWalletNotFound
		}
		return SellerWallet{}, fmt.Errorf("update wallet balance: %w", err)
	}
	return w, nil
}

const payoutColumns = `id, seller_id, amount, bank_code, account_number, account_name, status, created_at`

func scanPayout(row pgx.Row, p *PayoutRequest) error {
	var statusInt int32
	if err := row.Scan(&p.ID, &p.SellerID, &p.Amount, &p.BankCode, &p.AccountNumber, &p.AccountName, &statusInt, &p.CreatedAt); err != nil {
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

	const q = `INSERT INTO payout_requests (id, seller_id, amount, bank_code, account_number, account_name, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	if _, err := r.pool.Exec(ctx, q, p.ID, p.SellerID, p.Amount, p.BankCode, p.AccountNumber, p.AccountName, int32(p.Status), p.CreatedAt); err != nil {
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

const txWalletColumns = `id, wallet_id, amount, type, reference_id, created_at`

func scanWalletTx(row pgx.Row, t *WalletTransaction) error {
	var typeInt int32
	if err := row.Scan(&t.ID, &t.WalletID, &t.Amount, &typeInt, &t.ReferenceID, &t.CreatedAt); err != nil {
		return err
	}
	t.Type = WalletTransactionType(typeInt)
	return nil
}

func (r *PostgresWalletRepository) CreateWalletTransaction(ctx context.Context, tx WalletTransaction) (WalletTransaction, error) {
	if tx.ID == "" {
		tx.ID = uuid.NewString()
	}
	tx.CreatedAt = time.Now()

	const q = `INSERT INTO wallet_transactions (id, wallet_id, amount, type, reference_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := r.pool.Exec(ctx, q, tx.ID, tx.WalletID, tx.Amount, int32(tx.Type), tx.ReferenceID, tx.CreatedAt); err != nil {
		return WalletTransaction{}, fmt.Errorf("insert wallet transaction: %w", err)
	}
	return tx, nil
}

func (r *PostgresWalletRepository) ListWalletTransactions(ctx context.Context, walletID string) ([]WalletTransaction, error) {
	const q = `SELECT ` + txWalletColumns + ` FROM wallet_transactions WHERE wallet_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, walletID)
	if err != nil {
		return nil, fmt.Errorf("list wallet transactions: %w", err)
	}
	defer rows.Close()

	var result []WalletTransaction
	for rows.Next() {
		var tx WalletTransaction
		if err := scanWalletTx(rows, &tx); err != nil {
			return nil, fmt.Errorf("scan wallet tx: %w", err)
		}
		result = append(result, tx)
	}
	return result, rows.Err()
}

// ── InMemory Implementation ───────────────────────────────────────────

type InMemoryWalletRepository struct {
	mu           sync.RWMutex
	wallets      map[string]SellerWallet     // keyed by seller_id
	payouts      map[string]PayoutRequest    // keyed by id
	transactions []WalletTransaction
}

func NewInMemoryWalletRepository() *InMemoryWalletRepository {
	return &InMemoryWalletRepository{
		wallets: make(map[string]SellerWallet),
		payouts: make(map[string]PayoutRequest),
	}
}

func (r *InMemoryWalletRepository) GetOrCreateWallet(_ context.Context, sellerID string) (SellerWallet, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if w, ok := r.wallets[sellerID]; ok {
		return w, nil
	}

	w := SellerWallet{
		ID:        uuid.NewString(),
		SellerID:  sellerID,
		Balance:   0,
		Currency:  "VND",
		UpdatedAt: time.Now(),
	}
	r.wallets[sellerID] = w
	return w, nil
}

func (r *InMemoryWalletRepository) GetWalletBySellerID(_ context.Context, sellerID string) (SellerWallet, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	w, ok := r.wallets[sellerID];
	if !ok {
		return SellerWallet{}, ErrWalletNotFound
	}
	return w, nil
}

func (r *InMemoryWalletRepository) UpdateWalletBalance(_ context.Context, sellerID string, delta int64) (SellerWallet, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	w, ok := r.wallets[sellerID]
	if !ok {
		// Initialize if doesn't exist
		w = SellerWallet{
			ID:        uuid.NewString(),
			SellerID:  sellerID,
			Balance:   0,
			Currency:  "VND",
			UpdatedAt: time.Now(),
		}
	}

	if w.Balance+delta < 0 {
		return SellerWallet{}, ErrInsufficientBalance
	}

	w.Balance += delta
	w.UpdatedAt = time.Now()
	r.wallets[sellerID] = w
	return w, nil
}

func (r *InMemoryWalletRepository) CreatePayoutRequest(_ context.Context, p PayoutRequest) (PayoutRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Status == 0 {
		p.Status = PayoutStatusPending
	}
	p.CreatedAt = time.Now()
	r.payouts[p.ID] = p
	return p, nil
}

func (r *InMemoryWalletRepository) GetPayoutRequest(_ context.Context, id string) (PayoutRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.payouts[id]
	if !ok {
		return PayoutRequest{}, ErrPayoutNotFound
	}
	return p, nil
}

func (r *InMemoryWalletRepository) ListPayoutRequestsBySellerID(_ context.Context, sellerID string) ([]PayoutRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []PayoutRequest
	for _, p := range r.payouts {
		if p.SellerID == sellerID {
			result = append(result, p)
		}
	}
	// Sort by CreatedAt desc
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].CreatedAt.Before(result[j].CreatedAt) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result, nil
}

func (r *InMemoryWalletRepository) CreateWalletTransaction(_ context.Context, tx WalletTransaction) (WalletTransaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if tx.ID == "" {
		tx.ID = uuid.NewString()
	}
	tx.CreatedAt = time.Now()
	r.transactions = append(r.transactions, tx)
	return tx, nil
}

func (r *InMemoryWalletRepository) ListWalletTransactions(_ context.Context, walletID string) ([]WalletTransaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []WalletTransaction
	for _, tx := range r.transactions {
		if tx.WalletID == walletID {
			result = append(result, tx)
		}
	}
	return result, nil
}
