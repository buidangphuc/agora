package events

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	listingv1 "github.com/buidangphuc/team-notification/generated/platform/listing/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/store"
)

const (
	listingChangedType      = "platform.listing.v1.ListingChanged"
	listingStockChangedType = "platform.listing.v1.ListingStockChanged"
)

func NewListingHandler(pool TxBeginner, lowStockThreshold int32, logger *slog.Logger) consume.Handler {
	logger = orDefault(logger)
	return func(ctx context.Context, r consume.Record) error {
		env, err := decode(r.Value)
		if err != nil {
			return err
		}
		if env.GetType() != listingChangedType && env.GetType() != listingStockChangedType {
			return nil
		}
		return apply(ctx, pool, listingConsumerName, env.GetEventId(), func(tx pgx.Tx) error {
			if env.GetType() == listingStockChangedType {
				return handleStockChanged(ctx, tx, lowStockThreshold, env.GetPayload())
			}
			return handleListingChanged(ctx, tx, lowStockThreshold, logger, env.GetPayload())
		})
	}
}

func handleListingChanged(ctx context.Context, tx pgx.Tx, threshold int32, logger *slog.Logger, payload []byte) error {
	var e listingv1.ListingChanged
	if err := unmarshal(payload, &e); err != nil {
		return err
	}
	l := e.GetListing()
	if l == nil || l.GetId() == "" {
		return consume.Permanent(errMissing("ListingChanged listing"))
	}
	listingID := l.GetId()
	low := store.NewLowStock(tx)
	if e.GetChangeType() == listingv1.ChangeType_CHANGE_TYPE_DELETED {
		return low.Delete(ctx, listingID)
	}

	prices := store.NewListingPrices(tx)
	curPrice := l.GetPrice()
	prevPrice, priceKnown, err := prices.Get(ctx, listingID)
	if err != nil {
		return err
	}
	if priceKnown && curPrice < prevPrice {
		if err := notifySubscribers(ctx, tx, logger, listingID, notificationv1.AlertType_ALERT_TYPE_PRICE_DROP,
			notificationv1.NotificationType_NOTIFICATION_TYPE_PRICE_DROP,
			"Giảm giá!",
			fmt.Sprintf("Sản phẩm bạn theo dõi đã giảm giá còn %d (trước đó %d).", curPrice, prevPrice),
		); err != nil {
			return err
		}
	}
	if err := prices.Set(ctx, listingID, curPrice); err != nil {
		return err
	}

	stocks := store.NewListingStocks(tx)
	curStock := l.GetStock()
	prevStock, stockKnown, err := stocks.Get(ctx, listingID)
	if err != nil {
		return err
	}
	if stockKnown && prevStock == 0 && curStock > 0 {
		if err := notifySubscribers(ctx, tx, logger, listingID, notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK,
			notificationv1.NotificationType_NOTIFICATION_TYPE_BACK_IN_STOCK,
			"Đã có hàng trở lại!",
			"Sản phẩm bạn theo dõi đã có hàng trở lại. Nhanh tay đặt mua nhé!",
		); err != nil {
			return err
		}
	}
	if err := stocks.Set(ctx, listingID, curStock); err != nil {
		return err
	}
	return low.Upsert(ctx, listingID, l.GetSellerId(), l.GetTitle(), curStock <= threshold)
}

func handleStockChanged(ctx context.Context, tx pgx.Tx, threshold int32, payload []byte) error {
	var e listingv1.ListingStockChanged
	if err := unmarshal(payload, &e); err != nil {
		return err
	}
	id := e.GetListingId()
	if id == "" {
		return consume.Permanent(errMissing("ListingStockChanged listing_id"))
	}
	low := store.NewLowStock(tx)
	seller, title, isLow, known, err := low.Get(ctx, id)
	if err != nil {
		return err
	}
	if !known {
		return nil
	}
	stock := e.GetStock()
	switch {
	case stock <= threshold && !isLow:
		n := &notificationv1.Notification{
			UserId:  seller,
			Title:   "Sắp hết hàng",
			Body:    fmt.Sprintf("Sản phẩm %q chỉ còn %d trong kho.", title, stock),
			Type:    notificationv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
			LinkUrl: "/seller/" + id + "/edit",
		}
		if _, err := store.NewNotifications(tx).CreateNotification(ctx, n); err != nil {
			return fmt.Errorf("create low-stock notification: %w", err)
		}
		return low.SetLow(ctx, id, true)
	case stock > threshold && isLow:
		return low.SetLow(ctx, id, false)
	}
	return nil
}

func notifySubscribers(ctx context.Context, tx pgx.Tx, logger *slog.Logger, listingID string, alertType notificationv1.AlertType, notiType notificationv1.NotificationType, title, body string) error {
	subs, err := store.NewAlerts(tx).ListByListingAndType(ctx, listingID, alertType)
	if err != nil {
		return fmt.Errorf("list subscriptions for %q: %w", listingID, err)
	}
	notifs := store.NewNotifications(tx)
	for _, sub := range subs {
		n := &notificationv1.Notification{
			UserId:  sub.GetUserId(),
			Title:   title,
			Body:    body,
			Type:    notiType,
			LinkUrl: "/listing/" + listingID,
		}
		if _, err := notifs.CreateNotification(ctx, n); err != nil {
			return fmt.Errorf("create notification: %w", err)
		}
		logger.InfoContext(ctx, "alert notification created", "listing_id", listingID, "type", notiType.String())
	}
	return nil
}
