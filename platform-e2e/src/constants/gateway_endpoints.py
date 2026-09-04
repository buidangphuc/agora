"""team-gateway Connect/JSON endpoint paths (:8080).

Every call is `POST /<package>.<Service>/<Method>` with `Content-Type:
application/json` and (when authenticated) `Authorization: bearer <token>`.
"""

from __future__ import annotations

HEALTHZ = "/healthz"

# Edge / raw REST handlers (NOT Connect RPCs) served directly by the gateway mux.
TRACK = "/api/track"  # POST browsing beacon -> TrackingEvent on analytics.events
ADMIN_METRICS = "/api/admin/metrics"  # GET shaped CockpitMetricsResponse (Prometheus-sourced)
PROM_RAW_QUERY = "/api/v1/query"  # raw Prometheus query path — must NOT be exposed by the gateway

# platform.identity.v1
AUTH_REGISTER = "/platform.identity.v1.AuthService/Register"
AUTH_LOGIN = "/platform.identity.v1.AuthService/Login"
ADDRESS_CREATE = "/platform.identity.v1.AddressService/CreateAddress"
ADDRESS_LIST = "/platform.identity.v1.AddressService/ListAddresses"

# platform.listing.v1
LISTING_CREATE = "/platform.listing.v1.ListingService/CreateListing"
LISTING_GET = "/platform.listing.v1.ListingService/GetListing"
LISTING_LIST = "/platform.listing.v1.ListingService/ListListings"
LISTING_LIST_MINE = "/platform.listing.v1.ListingService/ListMyListings"
LISTING_CATEGORIES = "/platform.listing.v1.ListingService/ListCategories"
LISTING_RESERVE_STOCK = "/platform.listing.v1.ListingService/ReserveStock"
LISTING_RELEASE_STOCK = "/platform.listing.v1.ListingService/ReleaseStock"

# platform.search.v1
SEARCH_LISTINGS = "/platform.search.v1.SearchService/SearchListings"
SEARCH_SUGGEST = "/platform.search.v1.SearchService/Suggest"

# platform.recommendation.v1 (surface-recommendations: gateway forwards to team-ai)
RECOMMENDATION_RECOMMEND = "/platform.recommendation.v1.RecommendationService/Recommend"

# platform.order.v1
CART_GET = "/platform.order.v1.CartService/GetCart"
CART_ADD = "/platform.order.v1.CartService/AddToCart"
CART_UPDATE = "/platform.order.v1.CartService/UpdateCartItem"
CART_CLEAR = "/platform.order.v1.CartService/ClearCart"
ORDER_CREATE = "/platform.order.v1.OrderService/CreateOrder"
ORDER_GET = "/platform.order.v1.OrderService/GetOrder"
ORDER_SAGA_STATE = "/platform.order.v1.OrderService/GetSagaState"
ORDER_FORCE_FAIL_SAGA = "/platform.order.v1.OrderService/ForceFailSaga"
ORDER_RETURN_CREATE = "/platform.order.v1.OrderService/CreateReturnRequest"
ORDER_RETURN_UPDATE = "/platform.order.v1.OrderService/UpdateReturnStatus"
ORDER_SHIPMENT_CREATE = "/platform.order.v1.OrderService/CreateShipment"
ORDER_SHIPMENT_TRACKING = "/platform.order.v1.OrderService/GetShipmentTracking"

# platform.payment.v1
PAYMENT_PROCESS_MOCK = "/platform.payment.v1.PaymentService/ProcessMockPayment"
PAYMENT_REFUND = "/platform.payment.v1.PaymentService/RefundPayment"
PAYMENT_WALLET = "/platform.payment.v1.PaymentService/GetSellerWallet"
PAYMENT_PAYOUT_REQUEST = "/platform.payment.v1.PaymentService/RequestPayout"
PAYMENT_PAYOUT_HISTORY = "/platform.payment.v1.PaymentService/ListPayoutHistory"

# platform.engagement.v1
ENGAGEMENT_QA_ASK = "/platform.engagement.v1.EngagementService/AskQuestion"
ENGAGEMENT_QA_ANSWER = "/platform.engagement.v1.EngagementService/AnswerQuestion"
ENGAGEMENT_QA_LIST = "/platform.engagement.v1.EngagementService/ListQuestionsByListing"
ENGAGEMENT_DISPUTE_CREATE = "/platform.engagement.v1.EngagementService/CreateDispute"
ENGAGEMENT_DISPUTE_GET = "/platform.engagement.v1.EngagementService/GetDispute"
ENGAGEMENT_DISPUTE_RESOLVE = "/platform.engagement.v1.EngagementService/ResolveDispute"
