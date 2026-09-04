// Package upstream holds the gRPC clients the gateway routes to. The gateway is
// a pure router (Rule 2): it owns no data and no business logic — only these
// connections to the services that do.
package upstream

import (
	"fmt"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	aiv1 "github.com/buidangphuc/team-gateway/generated/platform/ai/v1"
	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	auditv1 "github.com/buidangphuc/team-gateway/generated/platform/audit/v1"
	chatv1 "github.com/buidangphuc/team-gateway/generated/platform/chat/v1"
	engagementv1 "github.com/buidangphuc/team-gateway/generated/platform/engagement/v1"
	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-gateway/generated/platform/listing/v1"
	notificationv1 "github.com/buidangphuc/team-gateway/generated/platform/notification/v1"
	orderv1 "github.com/buidangphuc/team-gateway/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-gateway/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-gateway/generated/platform/promotion/v1"
	recommendationv1 "github.com/buidangphuc/team-gateway/generated/platform/recommendation/v1"
	referralv1 "github.com/buidangphuc/team-gateway/generated/platform/referral/v1"
	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	sharingv1 "github.com/buidangphuc/team-gateway/generated/platform/sharing/v1"
	verificationv1 "github.com/buidangphuc/team-gateway/generated/platform/verification/v1"
)

// Clients is the routing table: one gRPC client per upstream service.
type Clients struct {
	conns []*grpc.ClientConn

	Search   searchv1.SearchServiceClient
	Listing  listingv1.ListingServiceClient
	Identity identityv1.AuthServiceClient
	Address  identityv1.AddressServiceClient
	// Session is team-identity's SessionService (active sessions + login history),
	// reached at the same gRPC endpoint as AuthService/AddressService.
	Session    identityv1.SessionServiceClient
	Engagement engagementv1.EngagementServiceClient
	Cart       orderv1.CartServiceClient
	Order      orderv1.OrderServiceClient
	Payment    paymentv1.PaymentServiceClient
	Chat       chatv1.ChatServiceClient
	AI         aiv1.AIServiceClient
	// AIChat is team-ai's ChatService (StreamChat token streaming), distinct from
	// Chat (team-chat's buyer↔seller messaging on the same contract).
	AIChat chatv1.ChatServiceClient
	// Recommendation is team-ai's RecommendationService (online serving of the
	// offline-built ranking artifacts), reached at the same gRPC endpoint as AI.
	Recommendation recommendationv1.RecommendationServiceClient
	// Voucher and FlashSale are team-promotion's two services (shop/platform
	// vouchers + time-boxed flash-sale campaigns), reached at the same gRPC endpoint.
	Voucher   promotionv1.VoucherServiceClient
	FlashSale promotionv1.FlashSaleServiceClient
	// Notification is team-notification's NotificationService (bell feed +
	// price-drop/back-in-stock alert subscriptions).
	Notification notificationv1.NotificationServiceClient
	// Subscription is team-promotion's SubscriptionService (seller plans +
	// entitlements), reached at the same gRPC endpoint as Voucher/FlashSale.
	Subscription promotionv1.SubscriptionServiceClient
	// Sponsored is team-promotion's SponsoredService (sponsored ad campaigns +
	// sponsored slots), reached at the same gRPC endpoint as Voucher/FlashSale.
	Sponsored promotionv1.SponsoredServiceClient
	// AnalyticsQuery is team-analytics' AnalyticsQueryService (seller-dashboard
	// read-model queries over the warehouse).
	AnalyticsQuery analyticsv1.AnalyticsQueryServiceClient
	// Referral, Verification, Sharing, and Audit are the four new standalone
	// services, each owning its own DB.
	Referral     referralv1.ReferralServiceClient
	Verification verificationv1.VerificationServiceClient
	Sharing      sharingv1.SharingServiceClient
	Audit        auditv1.AuditServiceClient
}

// Dial creates (lazy) gRPC connections to the upstream services.
func Dial(searchAddr, listingAddr, identityAddr, engagementAddr, orderAddr, paymentAddr, chatAddr, aiAddr, recAddr, promotionAddr, notificationAddr, analyticsAddr, referralAddr, verificationAddr, sharingAddr, auditAddr string) (*Clients, error) {
	insec := grpc.WithTransportCredentials(insecure.NewCredentials())
	// The gateway is the single edge that dials every upstream (Rules 1 & 2), so
	// one otelgrpc client stats handler here emits per-service gRPC RED metrics
	// (request count + duration histogram, labelled by rpc.service/method/status)
	// for the whole platform — no per-service /metrics endpoint needed. It records
	// into the global MeterProvider, which is the SDK no-op unless OTEL_ENABLED=true.
	otelStats := grpc.WithStatsHandler(otelgrpc.NewClientHandler())
	c := &Clients{}
	dial := func(name, addr string) (*grpc.ClientConn, error) {
		conn, err := grpc.NewClient(addr, insec, otelStats)
		if err != nil {
			c.Close()
			return nil, fmt.Errorf("dial %s %s: %w", name, addr, err)
		}
		c.conns = append(c.conns, conn)
		return conn, nil
	}
	searchConn, err := dial("search", searchAddr)
	if err != nil {
		return nil, err
	}
	listingConn, err := dial("listing", listingAddr)
	if err != nil {
		return nil, err
	}
	identityConn, err := dial("identity", identityAddr)
	if err != nil {
		return nil, err
	}
	engagementConn, err := dial("engagement", engagementAddr)
	if err != nil {
		return nil, err
	}
	orderConn, err := dial("order", orderAddr)
	if err != nil {
		return nil, err
	}
	paymentConn, err := dial("payment", paymentAddr)
	if err != nil {
		return nil, err
	}
	chatConn, err := dial("chat", chatAddr)
	if err != nil {
		return nil, err
	}
	aiConn, err := dial("ai", aiAddr)
	if err != nil {
		return nil, err
	}
	recConn, err := dial("recommendation", recAddr)
	if err != nil {
		return nil, err
	}
	promotionConn, err := dial("promotion", promotionAddr)
	if err != nil {
		return nil, err
	}
	notificationConn, err := dial("notification", notificationAddr)
	if err != nil {
		return nil, err
	}
	analyticsConn, err := dial("analytics", analyticsAddr)
	if err != nil {
		return nil, err
	}
	referralConn, err := dial("referral", referralAddr)
	if err != nil {
		return nil, err
	}
	verificationConn, err := dial("verification", verificationAddr)
	if err != nil {
		return nil, err
	}
	sharingConn, err := dial("sharing", sharingAddr)
	if err != nil {
		return nil, err
	}
	auditConn, err := dial("audit", auditAddr)
	if err != nil {
		return nil, err
	}

	c.Search = searchv1.NewSearchServiceClient(searchConn)
	c.Listing = listingv1.NewListingServiceClient(listingConn)
	c.Identity = identityv1.NewAuthServiceClient(identityConn)
	c.Address = identityv1.NewAddressServiceClient(identityConn)
	c.Session = identityv1.NewSessionServiceClient(identityConn)
	c.Engagement = engagementv1.NewEngagementServiceClient(engagementConn)
	c.Cart = orderv1.NewCartServiceClient(orderConn)
	c.Order = orderv1.NewOrderServiceClient(orderConn)
	c.Payment = paymentv1.NewPaymentServiceClient(paymentConn)
	c.Chat = chatv1.NewChatServiceClient(chatConn)
	c.AI = aiv1.NewAIServiceClient(aiConn)
	c.AIChat = chatv1.NewChatServiceClient(aiConn)
	c.Recommendation = recommendationv1.NewRecommendationServiceClient(recConn)
	c.Voucher = promotionv1.NewVoucherServiceClient(promotionConn)
	c.FlashSale = promotionv1.NewFlashSaleServiceClient(promotionConn)
	c.Notification = notificationv1.NewNotificationServiceClient(notificationConn)
	c.Subscription = promotionv1.NewSubscriptionServiceClient(promotionConn)
	c.Sponsored = promotionv1.NewSponsoredServiceClient(promotionConn)
	c.AnalyticsQuery = analyticsv1.NewAnalyticsQueryServiceClient(analyticsConn)
	c.Referral = referralv1.NewReferralServiceClient(referralConn)
	c.Verification = verificationv1.NewVerificationServiceClient(verificationConn)
	c.Sharing = sharingv1.NewSharingServiceClient(sharingConn)
	c.Audit = auditv1.NewAuditServiceClient(auditConn)
	return c, nil
}

// Close shuts the connections down.
func (c *Clients) Close() {
	for _, conn := range c.conns {
		if conn != nil {
			_ = conn.Close()
		}
	}
}
