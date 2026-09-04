package service

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"github.com/buidangphuc/team-payment/internal/repository"
)

// Pagination bounds for the wallet ledger list RPC.
const (
	defaultLedgerPageSize = 20
	maxLedgerPageSize     = 100
)

var (
	// ErrLedgerNotConfigured is returned when the ledger store was not wired.
	ErrLedgerNotConfigured = errors.New("wallet ledger repository not configured")
	// ErrInvalidPageToken is returned for a malformed (non-empty) pagination cursor.
	ErrInvalidPageToken = errors.New("invalid page token")
)

// GetWalletBalance returns the seller's mock wallet balance = SUM(ledger.amount).
func (s *PaymentService) GetWalletBalance(ctx context.Context, sellerID string) (int64, error) {
	if sellerID == "" {
		return 0, errors.New("seller id is required")
	}
	if s.ledgerRepo == nil {
		return 0, ErrLedgerNotConfigured
	}
	return s.ledgerRepo.Balance(ctx, sellerID)
}

// ListLedgerEntries returns a page of the seller's ledger entries (newest first),
// the opaque next-page cursor ("" when exhausted) and the seller's total entry count.
func (s *PaymentService) ListLedgerEntries(
	ctx context.Context,
	sellerID string,
	cursor string,
	pageSize int32,
) ([]repository.LedgerEntry, string, int64, error) {
	if sellerID == "" {
		return nil, "", 0, errors.New("seller id is required")
	}
	if s.ledgerRepo == nil {
		return nil, "", 0, ErrLedgerNotConfigured
	}

	limit := int(pageSize)
	if limit <= 0 {
		limit = defaultLedgerPageSize
	}
	if limit > maxLedgerPageSize {
		limit = maxLedgerPageSize
	}

	offset, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", 0, err
	}

	entries, total, err := s.ledgerRepo.ListEntries(ctx, sellerID, offset, limit)
	if err != nil {
		return nil, "", 0, err
	}

	var next string
	if int64(offset+len(entries)) < total {
		next = encodeCursor(offset + len(entries))
	}
	return entries, next, total, nil
}

// RequestWalletPayout books a MOCK payout as a PENDING debit ledger entry (no real
// money moves). The debit reduces the seller's balance immediately (balance = sum of
// ledger). Rejects non-positive amounts and payouts exceeding the current balance.
func (s *PaymentService) RequestWalletPayout(
	ctx context.Context,
	sellerID string,
	amount int64,
) (repository.LedgerEntry, error) {
	if sellerID == "" {
		return repository.LedgerEntry{}, errors.New("seller id is required")
	}
	if amount <= 0 {
		return repository.LedgerEntry{}, repository.ErrInvalidAmount
	}
	if s.ledgerRepo == nil {
		return repository.LedgerEntry{}, ErrLedgerNotConfigured
	}

	balance, err := s.ledgerRepo.Balance(ctx, sellerID)
	if err != nil {
		return repository.LedgerEntry{}, err
	}
	if amount > balance {
		return repository.LedgerEntry{}, repository.ErrInsufficientBalance
	}

	return s.ledgerRepo.AppendEntry(ctx, repository.LedgerEntry{
		SellerID: sellerID,
		Type:     repository.LedgerTypePayout,
		Amount:   -amount, // debit
		Status:   repository.LedgerStatusPending,
	})
}

// CreditWallet records a COMPLETED credit ledger entry for a seller (e.g. an order
// settlement). Not exposed as its own RPC; used by the settlement path and tests.
func (s *PaymentService) CreditWallet(
	ctx context.Context,
	sellerID string,
	amount int64,
	entryType string,
) (repository.LedgerEntry, error) {
	if sellerID == "" {
		return repository.LedgerEntry{}, errors.New("seller id is required")
	}
	if amount <= 0 {
		return repository.LedgerEntry{}, repository.ErrInvalidAmount
	}
	if s.ledgerRepo == nil {
		return repository.LedgerEntry{}, ErrLedgerNotConfigured
	}
	if entryType == "" {
		entryType = repository.LedgerTypeOrderSettlement
	}
	return s.ledgerRepo.AppendEntry(ctx, repository.LedgerEntry{
		SellerID: sellerID,
		Type:     entryType,
		Amount:   amount, // credit
		Status:   repository.LedgerStatusCompleted,
	})
}

// ── Opaque offset cursor ─────────────────────────────────────────────

func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("o:" + strconv.Itoa(offset)))
}

func decodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, ErrInvalidPageToken
	}
	s, ok := strings.CutPrefix(string(raw), "o:")
	if !ok {
		return 0, ErrInvalidPageToken
	}
	offset, err := strconv.Atoi(s)
	if err != nil || offset < 0 {
		return 0, ErrInvalidPageToken
	}
	return offset, nil
}
