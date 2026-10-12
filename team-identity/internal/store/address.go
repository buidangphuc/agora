package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrAddressNotFound is returned when an address does not exist or is owned by another user.
var ErrAddressNotFound = errors.New("address not found")

// Address is a shipping address owned by one user.
type Address struct {
	ID            string
	UserID        string
	RecipientName string
	Phone         string
	Street        string
	Ward          string
	District      string
	City          string
	IsDefault     bool
}

// AddressRepository stores a user's shipping addresses; every call is scoped to the owning user id.
type AddressRepository interface {
	List(ctx context.Context, userID string) ([]Address, error)
	Get(ctx context.Context, id, userID string) (Address, error)
	Create(ctx context.Context, addr Address) (Address, error)
	Update(ctx context.Context, addr Address) (Address, error)
	Delete(ctx context.Context, id, userID string) error
	SetDefault(ctx context.Context, id, userID string) (Address, error)
}

// PostgresAddressRepository is the AddressRepository backed by Postgres.
type PostgresAddressRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresAddressRepository builds the PostgresAddressRepository over the shared pool.
func NewPostgresAddressRepository(pool *pgxpool.Pool) *PostgresAddressRepository {
	return &PostgresAddressRepository{pool: pool}
}

const addressColumns = `id, user_id, recipient_name, phone, street, ward, district, city, is_default`

func scanAddress(row pgx.Row, a *Address) error {
	return row.Scan(&a.ID, &a.UserID, &a.RecipientName, &a.Phone, &a.Street, &a.Ward, &a.District, &a.City, &a.IsDefault)
}

// List implements AddressRepository.List in Postgres.
func (r *PostgresAddressRepository) List(ctx context.Context, userID string) ([]Address, error) {
	const q = `SELECT ` + addressColumns + ` FROM user_addresses WHERE user_id = $1 ORDER BY is_default DESC, created_at DESC`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list addresses: %w", err)
	}
	defer rows.Close()

	var items []Address
	for rows.Next() {
		var a Address
		if err := scanAddress(rows, &a); err != nil {
			return nil, fmt.Errorf("scan address: %w", err)
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

// Get implements AddressRepository.Get in Postgres.
func (r *PostgresAddressRepository) Get(ctx context.Context, id, userID string) (Address, error) {
	const q = `SELECT ` + addressColumns + ` FROM user_addresses WHERE id = $1 AND user_id = $2`
	var a Address
	if err := scanAddress(r.pool.QueryRow(ctx, q, id, userID), &a); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return Address{}, ErrAddressNotFound
		}
		return Address{}, fmt.Errorf("get address %q: %w", id, err)
	}
	return a, nil
}

// Create implements AddressRepository.Create in Postgres.
func (r *PostgresAddressRepository) Create(ctx context.Context, a Address) (Address, error) {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Address{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// If this is the first address or marked default, reset other defaults
	if a.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE user_addresses SET is_default = false WHERE user_id = $1`, a.UserID); err != nil {
			return Address{}, fmt.Errorf("reset default: %w", err)
		}
	} else {
		// Check if user has any existing addresses; if none, make this default
		var count int64
		_ = tx.QueryRow(ctx, `SELECT count(*) FROM user_addresses WHERE user_id = $1`, a.UserID).Scan(&count)
		if count == 0 {
			a.IsDefault = true
		}
	}

	const q = `INSERT INTO user_addresses (id, user_id, recipient_name, phone, street, ward, district, city, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING ` + addressColumns
	var out Address
	if err := scanAddress(tx.QueryRow(ctx, q, a.ID, a.UserID, a.RecipientName, a.Phone, a.Street, a.Ward, a.District, a.City, a.IsDefault), &out); err != nil {
		return Address{}, fmt.Errorf("create address: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Address{}, fmt.Errorf("commit tx: %w", err)
	}
	return out, nil
}

// Update implements AddressRepository.Update in Postgres.
func (r *PostgresAddressRepository) Update(ctx context.Context, a Address) (Address, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Address{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if a.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE user_addresses SET is_default = false WHERE user_id = $1`, a.UserID); err != nil {
			return Address{}, fmt.Errorf("reset default: %w", err)
		}
	}

	const q = `UPDATE user_addresses
		SET recipient_name = $3, phone = $4, street = $5, ward = $6, district = $7, city = $8, is_default = $9, updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING ` + addressColumns
	var out Address
	if err := scanAddress(tx.QueryRow(ctx, q, a.ID, a.UserID, a.RecipientName, a.Phone, a.Street, a.Ward, a.District, a.City, a.IsDefault), &out); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return Address{}, ErrAddressNotFound
		}
		return Address{}, fmt.Errorf("update address: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Address{}, fmt.Errorf("commit tx: %w", err)
	}
	return out, nil
}

// Delete implements AddressRepository.Delete in Postgres.
func (r *PostgresAddressRepository) Delete(ctx context.Context, id, userID string) error {
	const q = `DELETE FROM user_addresses WHERE id = $1 AND user_id = $2`
	res, err := r.pool.Exec(ctx, q, id, userID)
	if err != nil {
		return fmt.Errorf("delete address: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrAddressNotFound
	}
	return nil
}

// SetDefault implements AddressRepository.SetDefault in Postgres.
func (r *PostgresAddressRepository) SetDefault(ctx context.Context, id, userID string) (Address, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Address{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `UPDATE user_addresses SET is_default = false WHERE user_id = $1`, userID); err != nil {
		return Address{}, fmt.Errorf("reset default: %w", err)
	}

	const q = `UPDATE user_addresses SET is_default = true, updated_at = now() WHERE id = $1 AND user_id = $2 RETURNING ` + addressColumns
	var out Address
	if err := scanAddress(tx.QueryRow(ctx, q, id, userID), &out); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return Address{}, ErrAddressNotFound
		}
		return Address{}, fmt.Errorf("set default: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Address{}, fmt.Errorf("commit tx: %w", err)
	}
	return out, nil
}

// InMemoryAddressRepository is a deterministic fake store for tests.
type InMemoryAddressRepository struct {
	mu   sync.RWMutex
	byID map[string]Address
}

// NewInMemoryAddressRepository builds the InMemoryAddressRepository with no data.
func NewInMemoryAddressRepository() *InMemoryAddressRepository {
	return &InMemoryAddressRepository{byID: make(map[string]Address)}
}

// List implements AddressRepository.List in memory.
func (r *InMemoryAddressRepository) List(_ context.Context, userID string) ([]Address, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []Address
	for _, a := range r.byID {
		if a.UserID == userID {
			items = append(items, a)
		}
	}
	return items, nil
}

// Get implements AddressRepository.Get in memory.
func (r *InMemoryAddressRepository) Get(_ context.Context, id, userID string) (Address, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.byID[id]
	if !ok || a.UserID != userID {
		return Address{}, ErrAddressNotFound
	}
	return a, nil
}

// Create implements AddressRepository.Create in memory.
func (r *InMemoryAddressRepository) Create(_ context.Context, a Address) (Address, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.IsDefault {
		for id, item := range r.byID {
			if item.UserID == a.UserID {
				item.IsDefault = false
				r.byID[id] = item
			}
		}
	}
	r.byID[a.ID] = a
	return a, nil
}

// Update implements AddressRepository.Update in memory.
func (r *InMemoryAddressRepository) Update(_ context.Context, a Address) (Address, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.byID[a.ID]
	if !ok || existing.UserID != a.UserID {
		return Address{}, ErrAddressNotFound
	}
	if a.IsDefault {
		for id, item := range r.byID {
			if item.UserID == a.UserID {
				item.IsDefault = false
				r.byID[id] = item
			}
		}
	}
	r.byID[a.ID] = a
	return a, nil
}

// Delete implements AddressRepository.Delete in memory.
func (r *InMemoryAddressRepository) Delete(_ context.Context, id, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok || a.UserID != userID {
		return ErrAddressNotFound
	}
	delete(r.byID, id)
	return nil
}

// SetDefault implements AddressRepository.SetDefault in memory.
func (r *InMemoryAddressRepository) SetDefault(_ context.Context, id, userID string) (Address, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok || a.UserID != userID {
		return Address{}, ErrAddressNotFound
	}
	for k, item := range r.byID {
		if item.UserID == userID {
			item.IsDefault = false
			r.byID[k] = item
		}
	}
	a.IsDefault = true
	r.byID[id] = a
	return a, nil
}
