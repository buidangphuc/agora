package alerts

import (
	"context"
	"errors"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

var (
	ErrEmptyUser    = errors.New("user id is required")
	ErrEmptyListing = errors.New("listing_id is required")
	ErrInvalidType  = errors.New("alert type must be PRICE_DROP or BACK_IN_STOCK")
	ErrEmptySubID   = errors.New("subscription_id is required")
)

type Repository interface {
	Create(ctx context.Context, sub *notificationv1.AlertSubscription) (*notificationv1.AlertSubscription, error)
	Delete(ctx context.Context, id, userID string) error
	ListByUser(ctx context.Context, userID string) ([]*notificationv1.AlertSubscription, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Subscribe(ctx context.Context, userID, listingID string, alertType notificationv1.AlertType) (*notificationv1.AlertSubscription, error) {
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

func (s *Service) Unsubscribe(ctx context.Context, userID, subscriptionID string) error {
	if userID == "" {
		return ErrEmptyUser
	}
	if subscriptionID == "" {
		return ErrEmptySubID
	}
	return s.repo.Delete(ctx, subscriptionID, userID)
}

func (s *Service) List(ctx context.Context, userID string) ([]*notificationv1.AlertSubscription, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	return s.repo.ListByUser(ctx, userID)
}
