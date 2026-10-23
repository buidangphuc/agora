package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
)

var errTestStub = errors.New("store test stub: not implemented")

type PaymentStatus int32

const (
	PaymentStatusPending PaymentStatus = 1
	PaymentStatusPaid    PaymentStatus = 2
)

type PaymentTransaction struct {
	ID      string
	OrderID string
	BuyerID string
	Status  PaymentStatus
}

type SettleFn func(settled PaymentTransaction) (outbox.Row, error)

type PgTxWriter struct{}

func NewPgTxWriter(_ *pgxpool.Pool, _ *outbox.Store) *PgTxWriter { return &PgTxWriter{} }

func (w *PgTxWriter) SettleTx(_ context.Context, _ string, _ PaymentStatus, _ string, _ SettleFn) (PaymentTransaction, error) {
	return PaymentTransaction{}, errTestStub
}

func PurgeOutbox(_ context.Context, _ *outbox.Store) (int64, error) { return 0, errTestStub }
