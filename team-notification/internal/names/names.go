package names

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	identityv1 "github.com/buidangphuc/team-notification/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-notification/generated/platform/listing/v1"
)

const (
	principalID    = "service-team-notification"
	lookupTimeout  = 2 * time.Second
	lookupOverhead = 500 * time.Millisecond
)

var principalScopes = []string{"listing.read", "identity.read"}

type Resolver struct {
	conns       []*grpc.ClientConn
	storefronts listingv1.ListingServiceClient
	profiles    identityv1.PublicProfileServiceClient
	logger      *slog.Logger
}

func New(domainAddr, identityAddr string, logger *slog.Logger) (*Resolver, error) {
	if logger == nil {
		logger = slog.Default()
	}
	opts := []upstream.Option{upstream.AsService(principalID, principalScopes...), upstream.WithTimeout(lookupTimeout)}
	domain, err := upstream.NewClient(domainAddr, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial domain: %w", err)
	}
	identity, err := upstream.NewClient(identityAddr, opts...)
	if err != nil {
		_ = domain.Close()
		return nil, fmt.Errorf("dial identity: %w", err)
	}
	return &Resolver{
		conns:       []*grpc.ClientConn{domain, identity},
		storefronts: listingv1.NewListingServiceClient(domain),
		profiles:    identityv1.NewPublicProfileServiceClient(identity),
		logger:      logger,
	}, nil
}

func (r *Resolver) Close() {
	for _, c := range r.conns {
		_ = c.Close()
	}
}

func (r *Resolver) ResolveSenderName(ctx context.Context, senderID, sellerID string) string {
	if senderID == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout+lookupOverhead)
	defer cancel()

	if sellerID != "" && sellerID == senderID {
		res, err := r.storefronts.BatchGetStorefronts(ctx, &listingv1.BatchGetStorefrontsRequest{SellerIds: []string{senderID}})
		if err != nil {
			r.logger.WarnContext(ctx, "shop name lookup failed", "seller_id", senderID, "err", err)
		} else {
			for _, shop := range res.GetShops() {
				if shop.GetSellerId() == senderID {
					if name := strings.TrimSpace(shop.GetDisplayName()); name != "" {
						return name
					}
				}
			}
		}
	}
	res, err := r.profiles.GetPublicProfiles(ctx, &identityv1.GetPublicProfilesRequest{UserIds: []string{senderID}})
	if err != nil {
		r.logger.WarnContext(ctx, "user name lookup failed", "user_id", senderID, "err", err)
		return ""
	}
	for _, p := range res.GetProfiles() {
		if p.GetUserId() == senderID {
			return strings.TrimSpace(p.GetDisplayName())
		}
	}
	return ""
}
