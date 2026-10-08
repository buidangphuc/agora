package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	ID       string
	SellerID string
	Type     string
	Amount   int64
	Status   string
	// ReferenceID is what the entry is about: the payment transaction id for
	// ORDER_SETTLEMENT / REFUND_DEDUCTION ("" = NULL, e.g. payouts and legacy credits).
	// The store keeps (Type, ReferenceID) unique when set.
	ReferenceID string
	CreatedAt   time.Time
}

// ErrReferenceRequired is returned by AppendEntryOnce for an entry without a reference.
var ErrReferenceRequired = errors.New("reference id is required for an idempotent ledger entry")

// LedgerRepository is the storage port for the seller wallet ledger. Mirrors the
// PaymentRepository/WalletRepository layering: a Postgres impl for production and an
// in-memory impl for tests.
type LedgerRepository interface {
	// AppendEntry inserts a new ledger entry (assigning ID/CreatedAt when unset).
	AppendEntry(ctx context.Context, e LedgerEntry) (LedgerEntry, error)
	// AppendEntryOnce is AppendEntry made idempotent on (Type, ReferenceID): when an
	// entry with that key exists nothing is written and created is false (the existing
	// entry is returned). ReferenceID must be set.
	AppendEntryOnce(ctx context.Context, e LedgerEntry) (entry LedgerEntry, created bool, err error)
	// AppendDebit atomically checks the seller's balance and appends the debit entry
	// (e.Amount < 0) in one step: it returns ErrInsufficientBalance, appending
	// nothing, when balance + e.Amount would go negative. Concurrent debits for the
	// same seller are serialized so the balance can never be overdrawn.
	AppendDebit(ctx context.Context, e LedgerEntry) (LedgerEntry, error)
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

const ledgerColumns = `id, seller_id, type, amount, status, reference_id, created_at`

const insertLedgerSQL = `INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7)`

// insertLedgerOnceSQL writes nothing when (type, reference_id) is taken.
const insertLedgerOnceSQL = insertLedgerSQL + `
	ON CONFLICT (type, reference_id) WHERE reference_id IS NOT NULL DO NOTHING`

func normalizeEntry(e LedgerEntry) LedgerEntry {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.Status == "" {
		e.Status = LedgerStatusCompleted
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	return e
}

// nullIfEmpty maps "" to SQL NULL.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func scanLedgerEntry(row pgx.Row, e *LedgerEntry) error {
	var ref *string
	if err := row.Scan(&e.ID, &e.SellerID, &e.Type, &e.Amount, &e.Status, &ref, &e.CreatedAt); err != nil {
		return err
	}
	if ref != nil {
		e.ReferenceID = *ref
	}
	return nil
}

func insertLedger(ctx context.Context, q DBTX, e LedgerEntry) error {
	_, err := q.Exec(ctx, insertLedgerSQL, e.ID, e.SellerID, e.Type, e.Amount, e.Status, nullIfEmpty(e.ReferenceID), e.CreatedAt)
	return err
}

// insertLedgerOnce inserts e unless (Type, ReferenceID) exists; it then returns the
// existing entry and created=false.
func insertLedgerOnce(ctx context.Context, q DBTX, e LedgerEntry) (LedgerEntry, bool, error) {
	if e.ReferenceID == "" {
		return LedgerEntry{}, false, ErrReferenceRequired
	}
	e = normalizeEntry(e)
	tag, err := q.Exec(ctx, insertLedgerOnceSQL, e.ID, e.SellerID, e.Type, e.Amount, e.Status, e.ReferenceID, e.CreatedAt)
	if err != nil {
		return LedgerEntry{}, false, fmt.Errorf("insert ledger entry once: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return e, true, nil
	}
	existing, err := entryByReference(ctx, q, e.Type, e.ReferenceID)
	if err != nil {
		return LedgerEntry{}, false, err
	}
	return existing, false, nil
}

// entryByReference loads the entry of type typ for reference ref (ErrLedgerEntryNotFound
// when none).
func entryByReference(ctx context.Context, q DBTX, typ, ref string) (LedgerEntry, error) {
	const sel = `SELECT ` + ledgerColumns + ` FROM wallet_ledger WHERE type = $1 AND reference_id = $2`
	var e LedgerEntry
	if err := scanLedgerEntry(q.QueryRow(ctx, sel, typ, ref), &e); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LedgerEntry{}, ErrLedgerEntryNotFound
		}
		return LedgerEntry{}, fmt.Errorf("load ledger entry by reference: %w", err)
	}
	return e, nil
}

// ErrLedgerEntryNotFound is returned when no entry has the requested key.
var ErrLedgerEntryNotFound = errors.New("ledger entry not found")

func (r *PostgresLedgerRepository) AppendEntry(ctx context.Context, e LedgerEntry) (LedgerEntry, error) {
	e = normalizeEntry(e)
	if err := insertLedger(ctx, r.pool, e); err != nil {
		return LedgerEntry{}, fmt.Errorf("insert ledger entry: %w", err)
	}
	return e, nil
}

func (r *PostgresLedgerRepository) AppendEntryOnce(ctx context.Context, e LedgerEntry) (LedgerEntry, bool, error) {
	return insertLedgerOnce(ctx, r.pool, e)
}

// AppendDebit takes a per-seller transaction-scoped advisory lock, then sums the
// balance and inserts the debit in the same transaction. The lock (not row
// locking: a ledger has no single row to lock for a seller with no entries) makes
// concurrent payouts for one seller run one after another; other sellers are
// unaffected.
func (r *PostgresLedgerRepository) AppendDebit(ctx context.Context, e LedgerEntry) (LedgerEntry, error) {
	if e.Amount >= 0 {
		return LedgerEntry{}, ErrInvalidAmount
	}
	e = normalizeEntry(e)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return LedgerEntry{}, fmt.Errorf("begin debit tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	if err := lockSeller(ctx, tx, e.SellerID); err != nil {
		return LedgerEntry{}, err
	}
	var balance int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(amount), 0) FROM wallet_ledger WHERE seller_id = $1`, e.SellerID).Scan(&balance); err != nil {
		return LedgerEntry{}, fmt.Errorf("sum ledger balance: %w", err)
	}
	if balance+e.Amount < 0 {
		return LedgerEntry{}, ErrInsufficientBalance
	}
	if err := insertLedger(ctx, tx, e); err != nil {
		return LedgerEntry{}, fmt.Errorf("insert ledger entry: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return LedgerEntry{}, fmt.Errorf("commit debit: %w", err)
	}
	return e, nil
}

// lockSeller takes the per-seller transaction-scoped advisory lock that serialises every
// write which reads or changes a seller's balance (payouts, credits, deductions).
func lockSeller(ctx context.Context, q DBTX, sellerID string) error {
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('wallet_ledger:' || $1, 0))`, sellerID); err != nil {
		return fmt.Errorf("lock seller ledger: %w", err)
	}
	return nil
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
		if err := scanLedgerEntry(rows, &e); err != nil {
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

// appendLocked appends e; callers hold r.mu for writing.
func (r *InMemoryLedgerRepository) appendLocked(e LedgerEntry) LedgerEntry {
	e = normalizeEntry(e)
	r.seq++
	r.seqByID[e.ID] = r.seq
	r.entries = append(r.entries, e)
	return e
}

// byReferenceLocked finds the entry of type typ for ref; callers hold r.mu.
func (r *InMemoryLedgerRepository) byReferenceLocked(typ, ref string) (LedgerEntry, bool) {
	for _, x := range r.entries {
		if ref != "" && x.Type == typ && x.ReferenceID == ref {
			return x, true
		}
	}
	return LedgerEntry{}, false
}

// appendOnceLocked mirrors insertLedgerOnce; callers hold r.mu for writing.
func (r *InMemoryLedgerRepository) appendOnceLocked(e LedgerEntry) (LedgerEntry, bool, error) {
	if e.ReferenceID == "" {
		return LedgerEntry{}, false, ErrReferenceRequired
	}
	if existing, ok := r.byReferenceLocked(e.Type, e.ReferenceID); ok {
		return existing, false, nil
	}
	return r.appendLocked(e), true, nil
}

func (r *InMemoryLedgerRepository) AppendEntry(_ context.Context, e LedgerEntry) (LedgerEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendLocked(e), nil
}

func (r *InMemoryLedgerRepository) AppendEntryOnce(_ context.Context, e LedgerEntry) (LedgerEntry, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendOnceLocked(e)
}

// AppendDebit checks the balance and appends the debit under one write lock.
func (r *InMemoryLedgerRepository) AppendDebit(_ context.Context, e LedgerEntry) (LedgerEntry, error) {
	if e.Amount >= 0 {
		return LedgerEntry{}, ErrInvalidAmount
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var balance int64
	for _, x := range r.entries {
		if x.SellerID == e.SellerID {
			balance += x.Amount
		}
	}
	if balance+e.Amount < 0 {
		return LedgerEntry{}, ErrInsufficientBalance
	}
	return r.appendLocked(e), nil
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

var (
	_ LedgerRepository = (*PostgresLedgerRepository)(nil)
	_ LedgerRepository = (*InMemoryLedgerRepository)(nil)
)
