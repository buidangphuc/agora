package ga4relay

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type sqlMappingStore struct {
	db  *sql.DB
	now func() time.Time
}

// NewSQLMappingStore returns a durable MappingStore backed by ga4_relay_client_map (30d TTL).
func NewSQLMappingStore(ctx context.Context, db *sql.DB, now func() time.Time) (MappingStore, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS ga4_relay_client_map (
		buyer_id VARCHAR PRIMARY KEY,
		client_id VARCHAR NOT NULL,
		expires_micros BIGINT NOT NULL)`); err != nil {
		return nil, err
	}
	return &sqlMappingStore{db: db, now: now}, nil
}

func (s *sqlMappingStore) Get(ctx context.Context, buyerID string) (string, bool, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT client_id FROM ga4_relay_client_map WHERE buyer_id = ? AND expires_micros > ?`,
		buyerID, s.now().UnixMicro()).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

func (s *sqlMappingStore) Put(ctx context.Context, buyerID, clientID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ga4_relay_client_map (buyer_id, client_id, expires_micros) VALUES (?, ?, ?)
		 ON CONFLICT (buyer_id) DO UPDATE SET client_id = excluded.client_id, expires_micros = excluded.expires_micros`,
		buyerID, clientID, s.now().Add(mappingTTL).UnixMicro())
	return err
}

func (s *sqlMappingStore) Delete(ctx context.Context, buyerID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM ga4_relay_client_map WHERE buyer_id = ?`, buyerID)
	return err
}
