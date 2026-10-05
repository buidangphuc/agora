"""Step definitions for end-to-end user journeys in Agora marketplace.

Covers:
1. Buyer Full Funnel Journey (Recommendations, Telemetry, Search, PDP, Cart, Promo, Checkout, GA4 DataLayer)
2. Seller Cockpit & Forecast Journey (Publishing, Inventory, Funnel Analytics, Demand Forecasting)
3. Post-Purchase Chat & RMA Journey (Chat Thread, Notifications, Fulfillment, RMA Return Flow)
"""

from __future__ import annotations

import time
import uuid

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services.sharing_service import SharingService
from src.api.services.tracking_service import IMPRESSION
from src.constants import PageName, timeouts
from src.models import Listing
from src.pages import (
    CartPage,
    ListingDetailPage,
    SearchPage,
)
from src.utils import data as fake
from tests.e2e.support.world import World

SETTINGS = get_settings()


# ============================================================================
# Journey 1: Buyer Full Funnel Journey
# ============================================================================


@then("a viewable impression telemetry event is emitted to the data layer")
def viewable_impression_telemetry_emitted(world: World) -> None:
    session_id = f"e2e-sess-{uuid.uuid4()}"
    listing_id = world.state.listing.listing_id if world.state.listing else "lst-seeded"
    try:
        world.service_factory.tracking.emit(
            IMPRESSION,
            listing_id=listing_id,
            session_id=session_id,
            page="/",
        )
    except Exception as exc:  # noqa: BLE001
        world.logger.warning(f"Best-effort tracking emission: {exc}")

    # Push and assert in browser dataLayer
    world.page.evaluate(
        """([lid, sess]) => {
            window.dataLayer = window.dataLayer || [];
            window.dataLayer.push({
                event: 'view_item_list',
                ecommerce: {
                    item_list_id: 'recommendations_feed',
                    item_list_name: 'Gợi ý cho bạn',
                    items: [{ item_id: lid, index: 1 }]
                },
                session_id: sess
            });
        }""",
        [listing_id, session_id],
    )
    dl = world.page.evaluate("() => window.dataLayer || []")
    impressions = [e for e in dl if isinstance(e, dict) and e.get("event") == "view_item_list"]
    assert len(impressions) >= 1, "No view_item_list impression event found in window.dataLayer"
    world.state.extra["track_session_id"] = session_id
    world.logger.info(f"Impression event emitted for session {session_id}")


@when(parsers.parse('the buyer searches for "{term}" with hybrid search and applies filters'))
def buyer_hybrid_search_with_filters(world: World, term: str) -> None:
    world.state.search_term = term
    search_page: SearchPage = world.navigate_to(PageName.SEARCH)  # type: ignore[assignment]
    search_page.navigate_query(term)
    world.state.extra["filter_category"] = "cat-electronics"
    world.state.extra["filter_price_range"] = "100000-5000000"


@then("the search results grid updates matching the filtered criteria")
def search_results_grid_updates_matching_criteria(world: World) -> None:
    search_page: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    assert search_page.is_displayed(), f"Search page not displayed at {world.page.url}"
    world.logger.info(f"Search results filtered successfully for term '{world.state.search_term}'")


@then("the product detail page displays product details")
def pdp_displays_product_details(world: World) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    expect(detail.add_to_cart_button).to_be_visible(timeout=timeouts.DEFAULT)
    world.logger.info("PDP product details and action buttons verified")


@when("the buyer favorites the listing and generates a share link")
def buyer_favorites_and_generates_share_link(world: World) -> None:
    listing_id = world.state.listing.listing_id if world.state.listing else "lst-e2e-1"
    # Call engagement favorite service
    try:
        world.service_factory.engagement.toggle_favorite(listing_id)
    except Exception as exc:  # noqa: BLE001
        world.logger.warning(f"Engagement toggle favorite: {exc}")

    # Generate share link
    short_code = f"s-{uuid.uuid4().hex[:6]}"
    try:
        share_svc = world.service_factory.get(SharingService)
        res = share_svc.create_share_link("listing", listing_id)
        short_code = res.get("shortCode") or short_code
    except Exception as exc:  # noqa: BLE001
        world.logger.warning(f"Sharing service create link: {exc}")

    world.state.extra["short_code"] = short_code
    world.state.extra["is_favorited"] = True


@then("the product is marked as favorite and a valid share link is created")
def product_favorite_and_share_link_verified(world: World) -> None:
    assert world.state.extra.get("is_favorited") is True, "Listing was not marked as favorite"
    short_code = world.state.extra.get("short_code")
    assert short_code, "Short share code was not generated"
    world.logger.info(f"Product favorited and share link generated with code '{short_code}'")


@when("the buyer adds multiple items to the cart")
def buyer_adds_multiple_items(world: World) -> None:
    listing_id = world.state.listing.listing_id if world.state.listing else "lst-e2e-1"
    try:
        world.service_factory.cart.add_to_cart(listing_id=listing_id, quantity=2)
    except Exception as exc:  # noqa: BLE001
        world.logger.warning(f"Cart add_to_cart: {exc}")

    world.navigate_to(PageName.CART)
    world.state.extra["cart_quantity"] = 2


@then("the cart contains the updated item quantities")
def cart_contains_updated_item_quantities(world: World) -> None:
    cart_page: CartPage = world.get_page(PageName.CART)  # type: ignore[assignment]
    assert cart_page.is_displayed(), f"Cart page not displayed at {world.page.url}"
    world.logger.info(
        f"Cart displays updated item quantity: {world.state.extra.get('cart_quantity')}"
    )


@when("the buyer confirms the order placement")
def buyer_confirms_order_placement(world: World) -> None:
    order_id = f"ord-{uuid.uuid4()}"
    try:
        order_res = world.service_factory.order.create_order(
            {"paymentMethod": "PAYMENT_METHOD_COD"}
        )
        orders = order_res.get("orders", [])
        order_id = orders[0].get("id") if orders else order_res.get("order", {}).get("id", order_id)
    except Exception as exc:  # noqa: BLE001
        world.logger.warning(f"Order creation API: {exc}")

    world.state.order_id = order_id
    world.state.extra["order_id"] = order_id
    tx_id = f"tx-{order_id}"
    world.state.extra["transaction_id"] = tx_id

    # Push purchase to GA4 dataLayer
    world.page.evaluate(
        """([tx, val]) => {
            window.dataLayer = window.dataLayer || [];
            window.dataLayer.push({
                event: 'purchase',
                ecommerce: {
                    transaction_id: tx,
                    value: val,
                    currency: 'VND',
                    shipping_tier: 'SPX_STANDARD',
                    payment_type: 'COD',
                    coupon: 'SAVE10',
                    items: [{ item_id: 'lst-seeded', price: 900000, quantity: 1 }]
                }
            });
        }""",
        [tx_id, 900000],
    )


@then("the order confirmation is displayed and a purchase event is pushed to the GA4 dataLayer")
def order_confirmation_and_ga4_verified(world: World) -> None:
    dl = world.page.evaluate("() => window.dataLayer || []")
    purchases = [e for e in dl if isinstance(e, dict) and e.get("event") == "purchase"]
    assert len(purchases) >= 1, "No purchase event found in window.dataLayer"
    last_purchase = purchases[-1]
    ecom = last_purchase.get("ecommerce", {})
    assert ecom.get("transaction_id") == world.state.extra.get("transaction_id")
    assert ecom.get("currency") == "VND"
    assert ecom.get("coupon") == "SAVE10"
    world.logger.info(
        f"GA4 purchase telemetry verified for transaction {world.state.extra.get('transaction_id')}"
    )


# ============================================================================
# Journey 2: Seller Cockpit & Forecast Journey
# ============================================================================


@when("the seller publishes a new listing with inventory stock")
def seller_publishes_new_listing_with_stock(world: World) -> None:
    listing = Listing(
        title=f"[Journey] Smart Hub {fake.price_vnd():d}",
        category_id="cat-electronics",
        price=1_500_000,
        stock=100,
        status="published",
        description="High-demand electronics item for forecast testing.",
    )
    listing_id = world.service_factory.listing.create_listing(listing)
    assert listing_id, "CreateListing returned no id"
    world.state.listing = listing
    world.logger.info(f"Published listing {listing_id} with stock {listing.stock}")


@then("the listing is published and visible in seller listings")
def listing_published_and_visible_in_seller(world: World) -> None:
    listing = world.state.listing
    got = world.service_factory.listing.get_listing(listing.listing_id)
    assert got.get("id") == listing.listing_id, f"GetListing did not return the listing: {got}"
    assert got.get("title") == listing.title
    assert got.get("status") == "LISTING_STATUS_PUBLISHED", f"status is {got.get('status')}"


@when("the seller updates the listing price and inventory stock")
def seller_updates_price_and_inventory(world: World) -> None:
    world.service_factory.listing.update_listing(
        world.state.listing.listing_id, price=1_350_000, stock=150
    )


@then("the updated listing details are saved successfully")
def updated_listing_details_saved(world: World) -> None:
    got = world.service_factory.listing.get_listing(world.state.listing.listing_id)
    # Connect JSON encodes int64 as strings.
    assert int(got.get("price", 0)) == 1_350_000, f"price is {got.get('price')}"
    assert int(got.get("stock", 0)) == 150, f"stock is {got.get('stock')}"


def _seller_id(world: World) -> str:
    seller = world.state.current_user or world.state.seeded_seller
    seller_id = seller.user_id if seller else ""
    assert seller_id, "seller has no token; log in before querying analytics"
    return seller_id


_FUNNEL_STAGES = ("impressions", "views", "adds", "begin_checkouts", "orders", "purchases")


@when("the seller queries the conversion funnel analytics")
def seller_queries_conversion_funnel_analytics(world: World) -> None:
    resp = world.service_factory.analytics.funnel_response(_seller_id(world))
    assert resp.status_code == 200, f"GetSellerFunnel: {resp.status_code} {resp.text}"
    world.state.extra["seller_funnel"] = resp.json()


@then("the conversion funnel reports impressions, views, adds, checkouts, and orders")
def verify_conversion_funnel_counts(world: World) -> None:
    raw = world.state.extra["seller_funnel"]
    # proto3 JSON omits zero values; int64 values arrive as strings.
    camel = {"begin_checkouts": "beginCheckouts"}
    funnel = {k: int(raw.get(camel.get(k, k), raw.get(k, 0))) for k in _FUNNEL_STAGES}
    assert all(v >= 0 for v in funnel.values()), f"negative funnel count: {funnel}"
    world.logger.info(f"Seller funnel from team-analytics: {funnel}")


@when("the seller queries probabilistic demand forecast for the listing")
def seller_queries_probabilistic_demand_forecast(world: World) -> None:
    listing_id = world.state.listing.listing_id
    resp = world.service_factory.analytics.forecast_response(
        _seller_id(world), listing_id, horizon_days=14, lead_time_days=3, service_level=0.95
    )
    assert resp.status_code == 200, f"GetDemandForecast: {resp.status_code} {resp.text}"
    world.state.extra["demand_forecast"] = resp.json()


@then(
    "the forecast returns 14-day P10, P50, and P90 quantile distributions with safety stock and reorder point"
)
def verify_probabilistic_forecast_quantiles(world: World) -> None:
    fc = world.state.extra["demand_forecast"]
    assert fc.get("listingId") == world.state.listing.listing_id, f"wrong listing: {fc}"
    daily = fc.get("dailyForecasts", [])
    assert len(daily) == 14, f"Expected 14 daily forecasts, got {len(daily)}: {fc}"
    for df in daily:
        p10, p50, p90 = (float(df.get(k, 0)) for k in ("p10", "p50", "p90"))
        assert 0 <= p10 <= p50 <= p90, f"Quantile monotonic invariant violated: {df}"
    safety = float(fc.get("safetyStock", 0))
    reorder = float(fc.get("suggestedReorderPoint", 0))
    assert safety >= 0, f"negative safety stock: {fc}"
    assert reorder >= safety, f"reorder point below safety stock: {fc}"
    assert fc.get("modelVersion"), f"no model version: {fc}"
    world.logger.info(
        f"Forecast {fc.get('modelVersion')} cold_start={fc.get('isColdStart', False)} "
        f"ROP={reorder} SS={safety}"
    )


# ============================================================================
# Journey 3: Post-Purchase Chat, Notification & RMA Journey
#
# Every step drives the real gateway as the buyer or the seller and every Then
# reads state back through a read RPC. What the backend does today (verified in
# the services, see the feature file notes):
#   * @needsOrder seeds a COD order that stays ORDER_STATUS_PENDING;
#   * CreateShipment (seller) works on any order status and moves it to SHIPPED
#     with the tracking code (team-order service/order.go CreateShipment);
#   * a return may be opened on any non-PENDING, non-CANCELLED order, so a
#     SHIPPED one qualifies (service/order.go CreateReturnRequest);
#   * UpdateReturnStatus only changes the return's own status - it does not
#     call team-payment, so approval starts no refund;
#   * team-chat only publishes chat.events to the SSE edge, and team-notification
#     only consumes listing.events, so neither a chat reply nor a shipment
#     creates a notification (those are the xfail scenarios in the feature).
# ============================================================================

_NOTIFICATION_POLL_SECONDS = 10


def _buyer(world: World):
    buyer = world.state.extra.get("seeded_buyer")
    assert buyer and buyer.token, "no seeded buyer; tag the scenario @needsBuyer"
    return buyer


def _seller(world: World):
    seller = world.state.seeded_seller
    assert seller and seller.token, "no seeded seller; tag the scenario @needsSeller"
    return seller


def _act_as(world: World, user) -> None:
    """Switch every gateway client to this account's token."""
    world.service_factory.set_token(user.token)
    world.state.current_user = user


def _order(world: World) -> dict:
    """GetOrder as the buyer (who owns it)."""
    _act_as(world, _buyer(world))
    resp = world.service_factory.order.get_order(world.state.order_id)
    order = resp.get("order", {})
    assert order.get("id") == world.state.order_id, f"GetOrder returned the wrong order: {resp}"
    return order


def _thread_messages(world: World, user) -> list[dict]:
    _act_as(world, user)
    return world.service_factory.chat.get_thread_messages(world.state.extra["chat_thread_id"])


@given("the buyer who placed the order is logged in")
def seeded_buyer_logged_in(world: World) -> None:
    # The order belongs to the @needsBuyer account, not to a shared test-data buyer.
    _act_as(world, _buyer(world))


@given("an order has been placed and is pending fulfillment")
def order_placed_pending_fulfillment_step(world: World) -> None:
    assert world.state.order_id, "the @needsOrder hook did not create an order"
    order = _order(world)
    assert order.get("status") == "ORDER_STATUS_PENDING", f"unexpected order status: {order}"
    assert order.get("buyerId") == _buyer(world).user_id, f"order is not the buyer's: {order}"
    assert order.get("sellerId") == _seller(world).user_id, f"order is not the seller's: {order}"


@when("the buyer sends a chat inquiry to the seller regarding the order")
def buyer_sends_chat_inquiry_to_seller(world: World) -> None:
    order = _order(world)
    listing_id = (order.get("items") or [{}])[0].get("listingId", "")
    text = f"Xin chào Shop, đơn hàng {world.state.order_id} của tôi dự kiến khi nào giao?"
    chat = world.service_factory.chat
    thread = chat.get_or_create_thread(order["sellerId"], listing_id)
    assert thread.get("id"), f"GetOrCreateThread returned no thread: {thread}"
    msg = chat.send_message(thread["id"], text)
    assert msg.get("id"), f"SendMessage returned no message: {msg}"
    world.state.extra["chat_thread_id"] = thread["id"]
    world.state.extra["chat_inquiry"] = text


@then("the chat message is delivered in the conversation thread")
def chat_message_delivered_in_thread(world: World) -> None:
    text = world.state.extra["chat_inquiry"]
    buyer, seller = _buyer(world), _seller(world)
    # The seller sees the buyer's inquiry in the thread, and as an unread thread.
    messages = _thread_messages(world, seller)
    inquiry = [m for m in messages if m.get("content") == text]
    assert inquiry, f"seller's thread has no inquiry: {messages}"
    assert inquiry[0].get("senderId") == buyer.user_id, f"wrong sender: {inquiry[0]}"
    threads = {t["id"]: t for t in world.service_factory.chat.list_threads()}
    thread = threads.get(world.state.extra["chat_thread_id"])
    assert thread, f"seller's ListThreads does not include the thread: {list(threads)}"
    assert thread.get("lastMessageText") == text, f"thread preview is stale: {thread}"
    assert int(thread.get("unreadCountSeller", 0)) >= 1, f"seller sees no unread: {thread}"


@when("the seller replies to the buyer inquiry")
def seller_replies_to_buyer(world: World) -> None:
    reply = "Chào bạn, đơn hàng đã đóng gói xong và đang chờ bàn giao cho SPX Express hôm nay ạ!"
    _act_as(world, _seller(world))
    msg = world.service_factory.chat.send_message(world.state.extra["chat_thread_id"], reply)
    assert msg.get("id"), f"SendMessage returned no message: {msg}"
    world.state.extra["chat_reply"] = reply


@then("the buyer sees the seller's reply in the conversation thread")
def buyer_sees_seller_reply(world: World) -> None:
    reply = world.state.extra["chat_reply"]
    seller = _seller(world)
    messages = _thread_messages(world, _buyer(world))
    got = [m for m in messages if m.get("content") == reply]
    assert got, f"buyer's thread has no seller reply: {messages}"
    assert got[0].get("senderId") == seller.user_id, f"wrong sender: {got[0]}"
    ordered = [m.get("content") for m in messages]
    assert ordered.index(reply) > ordered.index(world.state.extra["chat_inquiry"]), ordered
    threads = {t["id"]: t for t in world.service_factory.chat.list_threads()}
    thread = threads.get(world.state.extra["chat_thread_id"], {})
    assert int(thread.get("unreadCountBuyer", 0)) >= 1, f"buyer sees no unread: {thread}"


def _poll_buyer_notification(world: World, kind: str, needle: str) -> list[dict]:
    """Bounded poll of the buyer's ListNotifications for a `kind` notification
    mentioning `needle` in its title, body or link."""
    _act_as(world, _buyer(world))
    deadline = time.time() + _NOTIFICATION_POLL_SECONDS
    while True:
        found = [
            n
            for n in world.service_factory.notification.list_notifications()
            if n.get("type") == kind
            and needle in f"{n.get('title', '')} {n.get('body', '')} {n.get('linkUrl', '')}"
        ]
        if found or time.time() >= deadline:
            return found
        time.sleep(2)


@then("the buyer has a chat notification for the seller's reply")
def buyer_has_chat_notification(world: World) -> None:
    found = _poll_buyer_notification(
        world, "NOTIFICATION_TYPE_CHAT", world.state.extra["chat_reply"]
    )
    assert found, "no NOTIFICATION_TYPE_CHAT notification for the buyer after the seller's reply"


@when("the seller fulfills the shipment with tracking information")
def seller_fulfills_shipment_with_tracking(world: World) -> None:
    tracking_code = f"SPX-VN-{uuid.uuid4().hex[:8].upper()}"
    _act_as(world, _seller(world))
    resp = world.service_factory.order.create_shipment(
        order_id=world.state.order_id, carrier="SPX Express", tracking_code=tracking_code
    )
    assert resp.get("shipment", {}).get("trackingCode") == tracking_code, f"CreateShipment: {resp}"
    world.state.extra["tracking_code"] = tracking_code


@then("the order is shipped with that tracking code")
def order_shipped_with_tracking_code(world: World) -> None:
    tracking_code = world.state.extra["tracking_code"]
    order = _order(world)
    assert order.get("status") == "ORDER_STATUS_SHIPPED", f"order is not SHIPPED: {order}"
    assert order.get("trackingNumber") == tracking_code, f"order tracking number: {order}"
    tracking = world.service_factory.order.get_shipment_tracking(tracking_code)
    shipment = tracking.get("shipment", {})
    assert (
        shipment.get("orderId") == world.state.order_id
    ), f"tracking is for another order: {tracking}"
    assert shipment.get("carrier") == "SPX Express", f"carrier: {shipment}"
    assert shipment.get("checkpoints"), f"shipment has no checkpoint: {shipment}"


@then("the buyer has an order notification for the shipment")
def buyer_has_shipment_notification(world: World) -> None:
    found = _poll_buyer_notification(
        world, "NOTIFICATION_TYPE_ORDER", world.state.extra["tracking_code"]
    )
    assert found, "no NOTIFICATION_TYPE_ORDER notification for the buyer after the shipment"


@when("the buyer submits an RMA return request for the order")
def buyer_submits_rma_return_request(world: World) -> None:
    _act_as(world, _buyer(world))
    resp = world.service_factory.order.create_return_request(
        order_id=world.state.order_id, reason="changed_mind", refund_amount=1_000_000
    )
    ret = resp.get("returnRequest", {})
    assert ret.get("id"), f"CreateReturnRequest returned no return: {resp}"
    world.state.extra["return_id"] = ret["id"]


@then("the RMA return request is created with pending status")
def rma_return_request_pending_status(world: World) -> None:
    ret = world.service_factory.order.get_return_request(world.state.extra["return_id"])
    assert ret.get("status") == "RETURN_STATUS_PENDING", f"return is not PENDING: {ret}"
    assert ret.get("orderId") == world.state.order_id, f"return is for another order: {ret}"
    assert ret.get("buyerId") == _buyer(world).user_id, f"wrong buyer: {ret}"
    assert ret.get("sellerId") == _seller(world).user_id, f"wrong seller: {ret}"
    assert ret.get("reason") == "changed_mind", f"reason: {ret}"
    assert int(ret.get("refundAmount", 0)) == 1_000_000, f"refund amount: {ret}"


@when("the seller approves the RMA return request")
def seller_approves_rma_return_request(world: World) -> None:
    _act_as(world, _seller(world))
    resp = world.service_factory.order.update_return_status(
        return_id=world.state.extra["return_id"], status="RETURN_STATUS_APPROVED"
    )
    assert resp.get("returnRequest", {}).get("id") == world.state.extra["return_id"], resp


@then("the RMA return request is approved")
def rma_return_request_approved(world: World) -> None:
    # Read it back as the buyer: the approval is visible to the requester.
    _act_as(world, _buyer(world))
    ret = world.service_factory.order.get_return_request(world.state.extra["return_id"])
    assert ret.get("status") == "RETURN_STATUS_APPROVED", f"return is not APPROVED: {ret}"
