package edge

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"

	"github.com/buidangphuc/team-gateway/generated/platform/ai/v1/aiv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/analytics/v1/analyticsv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/audit/v1/auditv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/chat/v1/chatv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/engagement/v1/engagementv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/listing/v1/listingv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/notification/v1/notificationv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/order/v1/orderv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/payment/v1/paymentv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/promotion/v1/promotionv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/recommendation/v1/recommendationv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/referral/v1/referralv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/search/v1/searchv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/sharing/v1/sharingv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/verification/v1/verificationv1connect"
	"github.com/buidangphuc/team-gateway/internal/events"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

// NewMux builds the edge HTTP handler: one Connect handler per contract service
// (each speaking gRPC + gRPC-web + JSON), all wrapped with the edge interceptor
// chain (request-id, auth, logging, rate-limit). Handlers forward to upstream
// gRPC services — no business logic here (Rule 2). analytics is the edge
// telemetry producer backing POST /api/track (Kafka when enabled, else no-op).
func NewMux(clients *upstream.Clients, e *Edge, analytics events.AnalyticsPublisher, prometheusURL string, logger *slog.Logger) *http.ServeMux {
	opts := connect.WithInterceptors(e.Interceptors(logger)...)
	mux := http.NewServeMux()

	authPath, authHandler := identityv1connect.NewAuthServiceHandler(NewAuthForwarder(clients.Identity, e), opts)
	mux.Handle(authPath, authHandler)

	addrPath, addrHandler := identityv1connect.NewAddressServiceHandler(NewAddressForwarder(clients.Address, e), opts)
	mux.Handle(addrPath, addrHandler)

	sessionPath, sessionHandler := identityv1connect.NewSessionServiceHandler(NewSessionForwarder(clients.Session, e), opts)
	mux.Handle(sessionPath, sessionHandler)

	searchPath, searchHandler := searchv1connect.NewSearchServiceHandler(NewSearchForwarder(clients.Search, e), opts)
	mux.Handle(searchPath, searchHandler)

	listingPath, listingHandler := listingv1connect.NewListingServiceHandler(NewListingForwarder(clients.Listing, e), opts)
	mux.Handle(listingPath, listingHandler)

	engagementPath, engagementHandler := engagementv1connect.NewEngagementServiceHandler(NewEngagementForwarder(clients.Engagement, e), opts)
	mux.Handle(engagementPath, engagementHandler)

	cartPath, cartHandler := orderv1connect.NewCartServiceHandler(NewCartForwarder(clients.Cart, e), opts)
	mux.Handle(cartPath, cartHandler)

	orderPath, orderHandler := orderv1connect.NewOrderServiceHandler(NewOrderForwarder(clients.Order, e), opts)
	mux.Handle(orderPath, orderHandler)

	paymentPath, paymentHandler := paymentv1connect.NewPaymentServiceHandler(NewPaymentForwarder(clients.Payment, e), opts)
	mux.Handle(paymentPath, paymentHandler)

	chatPath, chatHandler := chatv1connect.NewChatServiceHandler(NewChatForwarder(clients.Chat, clients.AIChat, e), opts)
	mux.Handle(chatPath, chatHandler)

	aiPath, aiHandler := aiv1connect.NewAIServiceHandler(NewAIForwarder(clients.AI, e), opts)
	mux.Handle(aiPath, aiHandler)

	recPath, recHandler := recommendationv1connect.NewRecommendationServiceHandler(NewRecommendationForwarder(clients.Recommendation, e), opts)
	mux.Handle(recPath, recHandler)

	voucherPath, voucherHandler := promotionv1connect.NewVoucherServiceHandler(NewVoucherForwarder(clients.Voucher, e), opts)
	mux.Handle(voucherPath, voucherHandler)

	flashSalePath, flashSaleHandler := promotionv1connect.NewFlashSaleServiceHandler(NewFlashSaleForwarder(clients.FlashSale, e), opts)
	mux.Handle(flashSalePath, flashSaleHandler)

	notificationPath, notificationHandler := notificationv1connect.NewNotificationServiceHandler(NewNotificationForwarder(clients.Notification, e), opts)
	mux.Handle(notificationPath, notificationHandler)

	subscriptionPath, subscriptionHandler := promotionv1connect.NewSubscriptionServiceHandler(NewSubscriptionForwarder(clients.Subscription, e), opts)
	mux.Handle(subscriptionPath, subscriptionHandler)

	sponsoredPath, sponsoredHandler := promotionv1connect.NewSponsoredServiceHandler(NewSponsoredForwarder(clients.Sponsored, e), opts)
	mux.Handle(sponsoredPath, sponsoredHandler)

	analyticsQueryPath, analyticsQueryHandler := analyticsv1connect.NewAnalyticsQueryServiceHandler(NewAnalyticsQueryForwarder(clients.AnalyticsQuery, e), opts)
	mux.Handle(analyticsQueryPath, analyticsQueryHandler)

	referralPath, referralHandler := referralv1connect.NewReferralServiceHandler(NewReferralForwarder(clients.Referral, e), opts)
	mux.Handle(referralPath, referralHandler)

	verificationPath, verificationHandler := verificationv1connect.NewVerificationServiceHandler(NewVerificationForwarder(clients.Verification, e), opts)
	mux.Handle(verificationPath, verificationHandler)

	sharingPath, sharingHandler := sharingv1connect.NewSharingServiceHandler(NewSharingForwarder(clients.Sharing, e), opts)
	mux.Handle(sharingPath, sharingHandler)

	auditPath, auditHandler := auditv1connect.NewAuditServiceHandler(NewAuditForwarder(clients.Audit, e), opts)
	mux.Handle(auditPath, auditHandler)

	reflector := grpcreflect.NewStaticReflector(
		identityv1connect.AuthServiceName,
		identityv1connect.AddressServiceName,
		identityv1connect.SessionServiceName,
		searchv1connect.SearchServiceName,
		listingv1connect.ListingServiceName,
		engagementv1connect.EngagementServiceName,
		orderv1connect.CartServiceName,
		orderv1connect.OrderServiceName,
		paymentv1connect.PaymentServiceName,
		chatv1connect.ChatServiceName,
		aiv1connect.AIServiceName,
		recommendationv1connect.RecommendationServiceName,
		promotionv1connect.VoucherServiceName,
		promotionv1connect.FlashSaleServiceName,
		promotionv1connect.SubscriptionServiceName,
		promotionv1connect.SponsoredServiceName,
		notificationv1connect.NotificationServiceName,
		analyticsv1connect.AnalyticsQueryServiceName,
		referralv1connect.ReferralServiceName,
		verificationv1connect.VerificationServiceName,
		sharingv1connect.SharingServiceName,
		auditv1connect.AuditServiceName,
	)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Edge telemetry collector: a browser beacon becomes a TrackingEvent on the
	// `analytics.events` topic. Pure edge concern (validate → stamp principal →
	// produce), the same class as auth resolution and rate limiting (Rule 2).
	mux.HandleFunc("POST /api/track", HandleTrack(e, analytics, logger))

	// Real-time SSE Multiplexing & Cockpit Metrics (Golden Demo Spine).
	// The cockpit handler queries Prometheus server-side with a fixed PromQL set
	// and shapes the result — it never exposes raw Prometheus to the browser.
	mux.HandleFunc("/api/events/live", HandleSSE)
	mux.Handle("/api/admin/metrics", NewCockpitHandler(prometheusURL))

	return mux
}
