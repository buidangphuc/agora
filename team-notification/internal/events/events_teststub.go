package events

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
)

type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type SenderNames interface {
	ResolveSenderName(ctx context.Context, senderID, sellerID string) string
}

var errNotImplemented = errors.New("task 1.7 not implemented")

func NewListingHandler(pool TxBeginner, lowStockThreshold int32, logger *slog.Logger) consume.Handler {
	return func(context.Context, consume.Record) error { return errNotImplemented }
}

func NewChatHandler(pool TxBeginner, names SenderNames, logger *slog.Logger) consume.Handler {
	return func(context.Context, consume.Record) error { return errNotImplemented }
}

func NewOrderHandler(pool TxBeginner, logger *slog.Logger) consume.Handler {
	return func(context.Context, consume.Record) error { return errNotImplemented }
}
