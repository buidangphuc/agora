package duckdb

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

type Option func(*Writer)

func WithTombstoneSecret(secret string) Option { return func(w *Writer) { w.tombstoneSecret = secret } }

const erasedUsersDDL = `CREATE TABLE IF NOT EXISTS erased_users (
  user_hmac VARCHAR PRIMARY KEY,
  pseudonym VARCHAR NOT NULL,
  erased_at TIMESTAMP
)`

func (w *Writer) userHMAC(userID string) string {
	m := hmac.New(sha256.New, []byte(w.tombstoneSecret))
	m.Write([]byte(userID))
	return hex.EncodeToString(m.Sum(nil))
}

const (
	selectTombstoneSQL = "SELECT pseudonym FROM " + warehouse.ErasedUsersTableName + " WHERE user_hmac = ?"
	insertTombstoneSQL = "INSERT INTO " + warehouse.ErasedUsersTableName + " (user_hmac, pseudonym, erased_at) VALUES (?, ?, ?)"
)

func newPseudonym() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("pseudonym: %w", err)
	}
	return warehouse.PseudonymPrefix + hex.EncodeToString(b[:]), nil
}

func (w *Writer) tombstones(ctx context.Context, tx *sql.Tx, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		var p string
		err := tx.QueryRowContext(ctx, selectTombstoneSQL, w.userHMAC(id)).Scan(&p)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("lookup tombstones: %w", err)
		}
		out[id] = p
	}
	return out, nil
}

func distinct(ids []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

// EraseUser re-keys or deletes every row of userID in one transaction.
func (w *Writer) EraseUser(ctx context.Context, userID string) (warehouse.EraseCounts, error) {
	if userID == "" {
		return nil, errors.New("erase user: empty user id")
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	counts, err := w.eraseTx(ctx, tx, userID)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit erase: %w", err)
	}
	return counts, nil
}

func (w *Writer) eraseTx(ctx context.Context, tx *sql.Tx, userID string) (warehouse.EraseCounts, error) {
	existing, err := w.tombstones(ctx, tx, []string{userID})
	if err != nil {
		return nil, err
	}
	pseudo, known := existing[userID]
	if !known {
		if pseudo, err = newPseudonym(); err != nil {
			return nil, err
		}
	}
	counts := warehouse.EraseCounts{}
	run := func(table, q string, args ...any) error {
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return fmt.Errorf("erase %s: %w", table, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("erase %s: %w", table, err)
		}
		counts[table] += n
		return nil
	}

	const tr = warehouse.TableName
	if err := run(tr, `UPDATE `+tr+` SET anonymous_id = ?, ga_client_id = NULL
WHERE COALESCE(principal_type, '') <> 'user' AND anonymous_id IN (
  SELECT DISTINCT anonymous_id FROM `+tr+` WHERE principal_type = 'user' AND principal_id = ? AND anonymous_id <> '')`,
		pseudo, userID); err != nil {
		return nil, err
	}
	if err := run(tr, `UPDATE `+tr+` SET principal_id = ?, session_id = ?, anonymous_id = ?, ga_client_id = NULL, properties = NULL
WHERE principal_type = 'user' AND principal_id = ?`, pseudo, pseudo, pseudo, userID); err != nil {
		return nil, err
	}

	const of = warehouse.OrderFactsTableName
	if err := run(of, `UPDATE `+of+` SET buyer_id = CASE WHEN buyer_id = ? THEN ? ELSE buyer_id END,
  seller_id = CASE WHEN seller_id = ? THEN ? ELSE seller_id END
WHERE buyer_id = ? OR seller_id = ?`, userID, pseudo, userID, pseudo, userID, userID); err != nil {
		return nil, err
	}

	const ef = warehouse.EngagementFactsTableName
	if err := run(ef, `DELETE FROM `+ef+` WHERE user_id = ? AND fact IN (?, ?, ?, ?)`, userID,
		warehouse.FactFavoriteAdded, warehouse.FactFavoriteRemoved, warehouse.FactSellerFollowed, warehouse.FactSellerUnfollowed); err != nil {
		return nil, err
	}
	if err := run(ef, `UPDATE `+ef+` SET user_id = CASE WHEN user_id = ? THEN ? ELSE user_id END,
  seller_id = CASE WHEN seller_id = ? THEN ? ELSE seller_id END
WHERE user_id = ? OR seller_id = ?`, userID, pseudo, userID, pseudo, userID, userID); err != nil {
		return nil, err
	}

	const ls = warehouse.ListingSellersTableName
	if err := run(ls, `UPDATE `+ls+` SET seller_id = ? WHERE seller_id = ?`, pseudo, userID); err != nil {
		return nil, err
	}

	if !known {
		if _, err := tx.ExecContext(ctx, insertTombstoneSQL,
			w.userHMAC(userID), pseudo, w.now().UTC()); err != nil {
			return nil, fmt.Errorf("insert tombstone: %w", err)
		}
	}
	for _, t := range []string{tr, of, ef, ls} {
		if _, ok := counts[t]; !ok {
			counts[t] = 0
		}
	}
	return counts, nil
}

var _ warehouse.UserEraser = (*Writer)(nil)
