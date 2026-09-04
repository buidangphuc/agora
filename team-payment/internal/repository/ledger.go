package repository

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ledger entry types (mirrors WalletTransactionType, carried as strings on the wire).
const (
	LedgerTypeOrderSettlement = "ORDER_SETTLEMENT"
	LedgerTypePayout          = "PAYOUT"
	LedgerTypeRefundDeduction = "REFUND_DEDUCTION"
)

// Ledger entry statuses.
const (
	LedgerStatusPending   = "PENDING"
	LedgerStatusCompleted = "COMPLETED"
	LedgerStatusRejected  = "REJECTED"
)

// LedgerEntry is a single append-only movement in a seller's wallet ledger. A
// seller's balance is SUM(amount) over their entries; sign encodes credit(+)/debit(-).
type LedgerEntry struct {
	ID        string
	SellerID  string
	Type      string
	Amount    int64
	Status    string
	CreatedAt time.Time
}

// LedgerRepository is the storage port for the seller wallet ledger. Mirrors the
// PaymentRepository/WalletRepository layering: a Postgres impl for production and an
// in-memory impl for tests.
type LedgerRepository interface {
	// AppendEntry inserts a new ledger entry (assigning ID/CreatedAt when unset).
	AppendEntry(ctx context.Context, e LedgerEntry) (LedgerEntry, error)
	// Balance returns SUM(amount) over the seller's entries (0 when none).
	Balance(ctx context.Context, sellerID string) (int64, error)
	// ListEntries returns a page of the seller's entries, newest first, plus the
	// total count for the seller. offset/limit are already clamped by the caller.
	ListEntries(ctx context.Context, sellerID string, offset, limit int) ([]LedgerEntry, int64, error)
}

// ── Postgres Implementation ──────────────────────────────────────────

type PostgresLedgerRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresLedgerRepository(pool *pgxpool.Pool) *PostgresLedgerRepository {
	return &PostgresLedgerRepository{pool: pool}
}

const ledgerColumns = `id, seller_id, type, amount, status, created_at`

func (r *PostgresLedgerRepository) AppendEntry(ctx context.Context, e LedgerEntry) (LedgerEntry, error) {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.Status == "" {
		e.Status = LedgerStatusCompleted
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	const q = `INSERT INTO wallet_ledger (id, seller_id, type, amount, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := r.pool.Exec(ctx, q, e.ID, e.SellerID, e.Type, e.Amount, e.Status, e.CreatedAt); err != nil {
		return LedgerEntry{}, fmt.Errorf("insert ledger entry: %w", err)
	}
	return e, nil
}

func (r *PostgresLedgerRepository) Balance(ctx context.Context, sellerID string) (int64, error) {
	const q = `SELECT COALESCE(SUM(amount), 0) FROM wallet_ledger WHERE seller_id = $1`
	var balance int64
	if err := r.pool.QueryRow(ctx, q, sellerID).Scan(&balance); err != nil {
		return 0, fmt.Errorf("sum ledger balance: %w", err)
	}
	return balance, nil
}

func (r *PostgresLedgerRepository) ListEntries(ctx context.Context, sellerID string, offset, limit int) ([]LedgerEntry, int64, error) {
	const countQ = `SELECT COUNT(*) FROM wallet_ledger WHERE seller_id = $1`
	var total int64
	if err := r.pool.QueryRow(ctx, countQ, sellerID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count ledger entries: %w", err)
	}

	const q = `SELECT ` + ledgerColumns + ` FROM wallet_ledger
		WHERE seller_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, sellerID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list ledger entries: %w", err)
	}
	defer rows.Close()

	var result []LedgerEntry
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(&e.ID, &e.SellerID, &e.Type, &e.Amount, &e.Status, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan ledger entry: %w", err)
		}
		result = append(result, e)
	}
	return result, total, rows.Err()
}

// ── InMemory Implementation ───────────────────────────────────────────

type InMemoryLedgerRepository struct {
	mu      sync.RWMutex
	entries []LedgerEntry
	seq     int64 // monotonic tiebreaker so ordering is deterministic in tests
	seqByID map[string]int64
}

func NewInMemoryLedgerRepository() *InMemoryLedgerRepository {
	return &InMemoryLedgerRepository{seqByID: make(map[string]int64)}
}

func (r *InMemoryLedgerRepository) AppendEntry(_ context.Context, e LedgerEntry) (LedgerEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.Status == "" {
		e.Status = LedgerStatusCompleted
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	r.seq++
	r.seqByID[e.ID] = r.seq
	r.entries = append(r.entries, e)
	return e, nil
}

func (r *InMemoryLedgerRepository) Balance(_ context.Context, sellerID string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var balance int64
	for _, e := range r.entries {
		if e.SellerID == sellerID {
			balance += e.Amount
		}
	}
	return balance, nil
}

func (r *InMemoryLedgerRepository) ListEntries(_ context.Context, sellerID string, offset, limit int) ([]LedgerEntry, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var owned []LedgerEntry
	for _, e := range r.entries {
		if e.SellerID == sellerID {
			owned = append(owned, e)
		}
	}
	// Newest first: higher insertion sequence sorts earlier (matches created_at DESC).
	sort.SliceStable(owned, func(i, j int) bool {
		return r.seqByID[owned[i].ID] > r.seqByID[owned[j].ID]
	})

	total := int64(len(owned))
	if offset >= len(owned) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(owned) {
		end = len(owned)
	}
	page := make([]LedgerEntry, end-offset)
	copy(page, owned[offset:end])
	return page, total, nil
}
