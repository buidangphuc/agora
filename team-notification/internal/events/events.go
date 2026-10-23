package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	eventsv1 "github.com/buidangphuc/team-notification/generated/platform/events/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/store"
)

type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type SenderNames interface {
	ResolveSenderName(ctx context.Context, senderID, sellerID string) string
}

const (
	listingConsumerName = "team-notification.listing"
	chatConsumerName    = "team-notification.chat"
	orderConsumerName   = "team-notification.order"
)

var dedupe consume.Dedupe

func orDefault(l *slog.Logger) *slog.Logger {
	if l == nil {
		return slog.Default()
	}
	return l
}

func decode(raw []byte) (*eventsv1.EventEnvelope, error) {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(raw, &env); err != nil {
		return nil, consume.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}
	return &env, nil
}

func unmarshal(payload []byte, m proto.Message) error {
	if err := proto.Unmarshal(payload, m); err != nil {
		return consume.Permanent(fmt.Errorf("unmarshal %s: %w", m.ProtoReflect().Descriptor().Name(), err))
	}
	return nil
}

func apply(ctx context.Context, pool TxBeginner, name, eventID string, effect func(tx pgx.Tx) error) error {
	if eventID == "" {
		return consume.Permanent(errors.New("envelope missing event_id"))
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	seen, err := dedupe.Seen(ctx, tx, name, eventID)
	if err != nil {
		return err
	}
	if seen {
		return nil
	}
	if err := effect(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func notifyRespectingPrefs(ctx context.Context, tx pgx.Tx, logger *slog.Logger, n *notificationv1.Notification) error {
	prefs, err := store.NewPrefs(tx).Get(ctx, n.GetUserId())
	if err != nil && !errors.Is(err, store.ErrPrefsNotFound) {
		return fmt.Errorf("read prefs: %w", err)
	}
	if enabled, ok := prefs.GetTypeEnabled()[n.GetType().String()]; ok && !enabled {
		logger.InfoContext(ctx, "notification skipped: type disabled by user", "type", n.GetType().String())
		return nil
	}
	if _, err := store.NewNotifications(tx).CreateNotification(ctx, n); err != nil {
		return fmt.Errorf("create notification: %w", err)
	}
	return nil
}

func errMissing(what string) error {
	return fmt.Errorf("missing %s", what)
}
