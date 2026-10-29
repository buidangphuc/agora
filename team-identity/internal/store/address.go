package repository

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

var ErrAddressNotFound = errors.New("address not found")

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

type AddressRepository interface {
	List(ctx context.Context, userID string) ([]Address, error)
	Get(ctx context.Context, id, userID string) (Address, error)
	Create(ctx context.Context, addr Address) (Address, error)
	Update(ctx context.Context, addr Address) (Address, error)
	Delete(ctx context.Context, id, userID string) error
	SetDefault(ctx context.Context, id, userID string) (Address, error)
}

type PostgresAddressRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresAddressRepository(pool *pgxpool.Pool) *PostgresAddressRepository {
	return &PostgresAddressRepository{pool: pool}
}

const addressColumns = `id, user_id, recipient_name, phone, street, ward, district, city, is_default`

func scanAddress(row pgx.Row, a *Address) error {
	return row.Scan(&a.ID, &a.UserID, &a.RecipientName, &a.Phone, &a.Street, &a.Ward, &a.District, &a.City, &a.IsDefault)
}

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

func (r *PostgresAddressRepository) Create(ctx context.Context, a Address) (Address, error) {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Address{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

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

func (r *PostgresAddressRepository) Update(ctx context.Context, a Address) (Address, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Address{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

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

func (r *PostgresAddressRepository) SetDefault(ctx context.Context, id, userID string) (Address, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Address{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

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

func NewInMemoryAddressRepository() *InMemoryAddressRepository {
	return &InMemoryAddressRepository{byID: make(map[string]Address)}
}

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

func (r *InMemoryAddressRepository) Get(_ context.Context, id, userID string) (Address, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.byID[id]
	if !ok || a.UserID != userID {
		return Address{}, ErrAddressNotFound
	}
	return a, nil
}

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
