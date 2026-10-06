// Package upstream holds team-notification's outbound gRPC calls to other services.
// Today that is one thing: resolving a chat sender's display name.
package upstream

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	identityv1 "github.com/buidangphuc/team-notification/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-notification/generated/platform/listing/v1"
)

// Service-principal convention for internal gRPC calls (same wire shape as
// team-order's upstream client): the caller sets x-principal-* metadata itself,
// because a Kafka consumer has no user request to forward. Scopes are the least
// this service needs: listing.read for team-domain's BatchGetStorefronts and
// identity.read for team-identity's GetPublicProfiles. Never write scopes.
const (
	servicePrincipalID     = "service-team-notification"
	servicePrincipalType   = "service"
	servicePrincipalScopes = "listing.read,identity.read"

	// lookupTimeout bounds one name lookup (storefront + profile combined). A slow
	// upstream must never stall the chat.events consumer for long.
	lookupTimeout = 2 * time.Second
)

// withServicePrincipal returns ctx carrying the service principal as outgoing
// gRPC metadata. It replaces any incoming principal: a consumer context has none.
func withServicePrincipal(ctx context.Context) context.Context {
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-principal-id", servicePrincipalID,
		"x-principal-type", servicePrincipalType,
		"x-principal-scopes", servicePrincipalScopes,
	))
}

// StorefrontClient is the slice of team-domain's ListingService used here.
type StorefrontClient interface {
	BatchGetStorefronts(ctx context.Context, in *listingv1.BatchGetStorefrontsRequest, opts ...grpc.CallOption) (*listingv1.BatchGetStorefrontsResponse, error)
}

// ProfileClient is the slice of team-identity's PublicProfileService used here.
type ProfileClient interface {
	GetPublicProfiles(ctx context.Context, in *identityv1.GetPublicProfilesRequest, opts ...grpc.CallOption) (*identityv1.GetPublicProfilesResponse, error)
}

// NameResolver names a chat sender: the shop display name when the sender is the
// thread's seller (team-domain storefronts), otherwise the user's display name
// (team-identity). It never returns an error: any failure yields "" so the caller
// falls back to a neutral label and the notification is still created.
type NameResolver struct {
	storefronts StorefrontClient
	profiles    ProfileClient
	timeout     time.Duration
	logger      *slog.Logger
}

// NewNameResolver builds a resolver over the two clients. Either may be nil
// (that source is then skipped).
func NewNameResolver(storefronts StorefrontClient, profiles ProfileClient, logger *slog.Logger) *NameResolver {
	if logger == nil {
		logger = slog.Default()
	}
	return &NameResolver{storefronts: storefronts, profiles: profiles, timeout: lookupTimeout, logger: logger}
}

// ResolveSenderName returns the name to show for senderID, or "" when unknown.
// sellerID is the thread's seller; when it equals senderID the shop name is tried
// first, then the identity profile.
func (r *NameResolver) ResolveSenderName(ctx context.Context, senderID, sellerID string) string {
	if senderID == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(withServicePrincipal(ctx), r.timeout)
	defer cancel()

	if sellerID != "" && sellerID == senderID && r.storefronts != nil {
		res, err := r.storefronts.BatchGetStorefronts(ctx, &listingv1.BatchGetStorefrontsRequest{SellerIds: []string{senderID}})
		if err != nil {
			r.logger.WarnContext(ctx, "shop name lookup failed", slog.String("seller_id", senderID), slog.Any("err", err))
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
	if r.profiles == nil {
		return ""
	}
	res, err := r.profiles.GetPublicProfiles(ctx, &identityv1.GetPublicProfilesRequest{UserIds: []string{senderID}})
	if err != nil {
		r.logger.WarnContext(ctx, "user name lookup failed", slog.String("user_id", senderID), slog.Any("err", err))
		return ""
	}
	for _, p := range res.GetProfiles() {
		if p.GetUserId() == senderID {
			return strings.TrimSpace(p.GetDisplayName())
		}
	}
	return ""
}

// Clients owns the gRPC connections behind a NameResolver.
type Clients struct {
	conns    []*grpc.ClientConn
	Resolver *NameResolver
}

// Dial opens lazy connections to team-domain and team-identity (no network I/O
// until the first call, so a down upstream cannot block startup).
func Dial(domainAddr, identityAddr string, logger *slog.Logger) (*Clients, error) {
	insec := grpc.WithTransportCredentials(insecure.NewCredentials())
	domainConn, err := grpc.NewClient(domainAddr, insec)
	if err != nil {
		return nil, fmt.Errorf("dial domain %s: %w", domainAddr, err)
	}
	identityConn, err := grpc.NewClient(identityAddr, insec)
	if err != nil {
		_ = domainConn.Close()
		return nil, fmt.Errorf("dial identity %s: %w", identityAddr, err)
	}
	return &Clients{
		conns: []*grpc.ClientConn{domainConn, identityConn},
		Resolver: NewNameResolver(
			listingv1.NewListingServiceClient(domainConn),
			identityv1.NewPublicProfileServiceClient(identityConn),
			logger,
		),
	}, nil
}

// Close releases the connections.
func (c *Clients) Close() {
	for _, conn := range c.conns {
		_ = conn.Close()
	}
}
