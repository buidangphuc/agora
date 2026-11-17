package listing

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	promotionv1 "github.com/buidangphuc/team-domain/generated/platform/promotion/v1"
)

const entitlementsTimeout = 2 * time.Second

var ErrEntitlementsUnavailable = errors.New("entitlements unavailable")

type Entitlements struct {
	MaxListings      int64
	FeaturedListings int64
}

type EntitlementsPort interface {
	Get(ctx context.Context, sellerID string) (Entitlements, error)
}

type grpcEntitlements struct {
	client promotionv1.SubscriptionServiceClient
}

func NewGRPCEntitlements(client promotionv1.SubscriptionServiceClient) EntitlementsPort {
	return &grpcEntitlements{client: client}
}

func (g *grpcEntitlements) Get(ctx context.Context, sellerID string) (Entitlements, error) {
	ctx, cancel := context.WithTimeout(ctx, entitlementsTimeout)
	defer cancel()
	resp, err := g.client.GetEntitlements(ctx, &promotionv1.GetEntitlementsRequest{SellerId: sellerID})
	if err != nil {
		return Entitlements{}, fmt.Errorf("%w: %w", ErrEntitlementsUnavailable, err)
	}
	maxL, err := parseLimit(resp.GetLimits(), "max_listings")
	if err != nil {
		return Entitlements{}, err
	}
	featured, err := parseLimit(resp.GetLimits(), "featured_listings")
	if err != nil {
		return Entitlements{}, err
	}
	return Entitlements{MaxListings: maxL, FeaturedListings: featured}, nil
}

func parseLimit(limits map[string]string, key string) (int64, error) {
	raw, ok := limits[key]
	if !ok {
		return 0, fmt.Errorf("%w: limit %s missing", ErrEntitlementsUnavailable, key)
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < -1 {
		return 0, fmt.Errorf("%w: limit %s malformed", ErrEntitlementsUnavailable, key)
	}
	return n, nil
}
