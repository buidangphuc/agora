// Package service holds team-notification's application logic. AlertService owns
// the price-drop / back-in-stock alert subscriptions: it validates input and
// delegates persistence to the repository. Handlers resolve the caller's user id
// from the principal and pass it in; the service never guesses identity.
package service

import (
	"context"
	"errors"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/repository"
)

// Validation errors surfaced to the handler, which maps them to gRPC codes.
var (
	ErrEmptyUser    = errors.New("user id is required")
	ErrEmptyListing = errors.New("listing_id is required")
	ErrInvalidType  = errors.New("alert type must be PRICE_DROP or BACK_IN_STOCK")
	ErrEmptySubID   = errors.New("subscription_id is required")
)

// AlertService is the subscription use-case boundary.
type AlertService struct {
	repo repository.AlertSubscriptionRepository
}

func NewAlertService(repo repository.AlertSubscriptionRepository) *AlertService {
	return &AlertService{repo: repo}
}

// Subscribe registers (or returns the existing) alert subscription for a user +
// listing + type. Idempotent: repeated calls yield the same subscription.
func (s *AlertService) Subscribe(ctx context.Context, userID, listingID string, alertType notificationv1.AlertType) (*notificationv1.AlertSubscription, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	if listingID == "" {
		return nil, ErrEmptyListing
	}
	if alertType == notificationv1.AlertType_ALERT_TYPE_UNSPECIFIED {
		return nil, ErrInvalidType
	}
	return s.repo.Create(ctx, &notificationv1.AlertSubscription{
		UserId:    userID,
		ListingId: listingID,
		Type:      alertType,
	})
}

// Unsubscribe removes a subscription owned by the user. Idempotent: unsubscribing
// a missing (or another user's) id succeeds as a no-op.
func (s *AlertService) Unsubscribe(ctx context.Context, userID, subscriptionID string) error {
	if userID == "" {
		return ErrEmptyUser
	}
	if subscriptionID == "" {
		return ErrEmptySubID
	}
	return s.repo.Delete(ctx, subscriptionID, userID)
}

// List returns the user's alert subscriptions.
func (s *AlertService) List(ctx context.Context, userID string) ([]*notificationv1.AlertSubscription, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	return s.repo.ListByUser(ctx, userID)
}
