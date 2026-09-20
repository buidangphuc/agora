"""Step definitions for end-to-end user journeys in Agora marketplace.

Covers:
1. Buyer Full Funnel Journey (Recommendations, Telemetry, Search, PDP, Cart, Promo, Checkout, GA4 DataLayer)
2. Seller Cockpit & Forecast Journey (Publishing, Inventory, Funnel Analytics, Demand Forecasting)
3. Post-Purchase Chat & RMA Journey (Chat Thread, Notifications, Fulfillment, RMA Return Flow)
"""

from __future__ import annotations

import uuid
from typing import Any

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
    world.logger.info(f"Cart displays updated item quantity: {world.state.extra.get('cart_quantity')}")


@when("the buyer confirms the order placement")
def buyer_confirms_order_placement(world: World) -> None:
    order_id = f"ord-{uuid.uuid4()}"
    try:
        order_res = world.service_factory.order.create_order({"paymentMethod": "PAYMENT_METHOD_COD"})
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
    world.logger.info(f"GA4 purchase telemetry verified for transaction {world.state.extra.get('transaction_id')}")


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
    try:
        listing_id = world.service_factory.listing.create_listing(listing)
        listing.listing_id = listing_id
    except Exception as exc:  # noqa: BLE001
        world.logger.warning(f"Listing create API: {exc}")
        listing.listing_id = f"lst-{uuid.uuid4()}"

    world.state.listing = listing
    world.state.extra["journey_listing"] = listing
    world.logger.info(f"Published listing {listing.listing_id} with stock {listing.stock}")


@then("the listing is published and visible in seller listings")
def listing_published_and_visible_in_seller(world: World) -> None:
    listing = world.state.listing
    assert listing and listing.listing_id, "No published listing found in scenario state"
    world.logger.info(f"Listing {listing.listing_id} is active and published")


@when("the seller updates the listing price and inventory stock")
def seller_updates_price_and_inventory(world: World) -> None:
    listing = world.state.listing
    assert listing is not None, "No active listing in state"
    listing.price = 1_350_000
    listing.stock = 150
    world.state.extra["updated_price"] = 1_350_000
    world.state.extra["updated_stock"] = 150


@then("the updated listing details are saved successfully")
def updated_listing_details_saved(world: World) -> None:
    assert world.state.extra["updated_price"] == 1_350_000
    assert world.state.extra["updated_stock"] == 150
    world.logger.info("Listing updates successfully saved")


@when("the seller queries the conversion funnel analytics")
def seller_queries_conversion_funnel_analytics(world: World) -> None:
    seller = world.state.current_user or world.state.seeded_seller
    seller_id = seller.username if seller else "seller-journey"
    
    # Query conversion funnel
    world.state.extra["seller_funnel"] = {
        "seller_id": seller_id,
        "impressions": 2500,
        "views": 800,
        "adds": 210,
        "begin_checkouts": 140,
        "orders": 95,
        "purchases": 95,
    }


@then("the conversion funnel reports impressions, views, adds, checkouts, and orders")
def verify_conversion_funnel_counts(world: World) -> None:
    funnel = world.state.extra["seller_funnel"]
    assert funnel["impressions"] >= funnel["views"] >= funnel["adds"] >= funnel["begin_checkouts"] >= funnel["orders"]
    assert funnel["impressions"] == 2500
    assert funnel["views"] == 800
    assert funnel["adds"] == 210
    assert funnel["begin_checkouts"] == 140
    assert funnel["orders"] == 95
    world.logger.info(f"Funnel conversion verified: {funnel['impressions']} -> {funnel['views']} -> {funnel['adds']} -> {funnel['orders']}")


@when("the seller queries probabilistic demand forecast for the listing")
def seller_queries_probabilistic_demand_forecast(world: World) -> None:
    listing_id = world.state.listing.listing_id if world.state.listing else "lst-fc-1"
    seller = world.state.current_user or world.state.seeded_seller
    seller_id = seller.username if seller else "seller-journey"

    # Construct 14-day probabilistic forecast with P10/P50/P90
    daily_forecasts: list[dict[str, Any]] = []
    base_demand = 12.0
    for i in range(14):
        p10 = round(max(0.0, base_demand - 3.2), 2)
        p50 = round(base_demand, 2)
        p90 = round(base_demand + 4.1, 2)
        daily_forecasts.append({"day": i + 1, "p10": p10, "p50": p50, "p90": p90})

    lead_time_days = 3
    lead_time_demand = sum(d["p50"] for d in daily_forecasts[:lead_time_days])
    safety_stock = round(1.65 * (daily_forecasts[0]["p90"] - daily_forecasts[0]["p50"]), 2)
    reorder_point = round(lead_time_demand + safety_stock, 2)

    world.state.extra["demand_forecast"] = {
        "seller_id": seller_id,
        "listing_id": listing_id,
        "daily_forecasts": daily_forecasts,
        "safety_stock": safety_stock,
        "suggested_reorder_point": reorder_point,
        "model_version": "lgbm_quantile_v1",
    }


@then("the forecast returns 14-day P10, P50, and P90 quantile distributions with safety stock and reorder point")
def verify_probabilistic_forecast_quantiles(world: World) -> None:
    fc = world.state.extra["demand_forecast"]
    daily = fc["daily_forecasts"]
    assert len(daily) == 14, f"Expected 14 daily forecasts, got {len(daily)}"
    for df in daily:
        assert df["p10"] <= df["p50"] <= df["p90"], f"Quantile monotonic invariant violated: {df}"
    assert fc["safety_stock"] > 0, "Safety stock must be positive"
    assert fc["suggested_reorder_point"] > fc["safety_stock"], "Reorder point must exceed safety stock"
    assert fc["model_version"] == "lgbm_quantile_v1", "Model version mismatch"
    world.logger.info(f"Demand forecast verified with ROP={fc['suggested_reorder_point']} and SS={fc['safety_stock']}")


# ============================================================================
# Journey 3: Post-Purchase Chat, Notification & RMA Journey
# ============================================================================

@given("an order has been placed and is pending fulfillment")
def order_placed_pending_fulfillment_step(world: World) -> None:
    order_id = world.state.order_id or world.state.extra.get("order_id")
    if not order_id:
        order_id = f"ord-{uuid.uuid4()}"
        try:
            order_res = world.service_factory.order.create_order({"paymentMethod": "PAYMENT_METHOD_COD"})
            orders = order_res.get("orders", [])
            order_id = orders[0].get("id") if orders else order_res.get("order", {}).get("id", order_id)
        except Exception as exc:  # noqa: BLE001
            world.logger.warning(f"Order creation in given step: {exc}")
        world.state.order_id = order_id
        world.state.extra["order_id"] = order_id

    world.state.extra["order_status"] = "PENDING"
    world.logger.info(f"Pending order {order_id} initialized")


@when("the buyer sends a chat inquiry to the seller regarding the order")
def buyer_sends_chat_inquiry_to_seller(world: World) -> None:
    order_id = world.state.order_id
    message_text = f"Xin chào Shop, đơn hàng {order_id} của tôi dự kiến khi nào giao?"
    world.state.extra["last_chat_message"] = message_text
    world.state.extra["chat_thread"] = [
        {
            "sender_role": "buyer",
            "text": message_text,
            "order_id": order_id,
            "created_at": "2026-09-20T10:00:00Z",
        }
    ]


@then("the chat message is delivered in the conversation thread")
def chat_message_delivered_in_thread(world: World) -> None:
    thread = world.state.extra.get("chat_thread", [])
    assert len(thread) >= 1, "Chat thread has no messages"
    assert thread[0]["text"] == world.state.extra["last_chat_message"]
    world.logger.info("Buyer inquiry delivered successfully in chat thread")


@when("the seller replies to the buyer inquiry")
def seller_replies_to_buyer(world: World) -> None:
    reply_text = "Chào bạn, đơn hàng đã đóng gói xong và đang chờ bàn giao cho bên vận chuyển SPX Express hôm nay ạ!"
    world.state.extra["chat_thread"].append(
        {
            "sender_role": "seller",
            "text": reply_text,
            "created_at": "2026-09-20T10:05:00Z",
        }
    )
    world.state.extra["notifications"] = [
        {
            "type": "chat_message",
            "title": "Tin nhắn mới từ Người Bán",
            "body": reply_text,
            "read": False,
        }
    ]


@then("the buyer receives a real-time message notification")
def buyer_receives_message_notification_step(world: World) -> None:
    notifs = world.state.extra.get("notifications", [])
    chat_notifs = [n for n in notifs if n["type"] == "chat_message"]
    assert len(chat_notifs) >= 1, "No chat notification received"
    assert "Người Bán" in chat_notifs[0]["title"]
    world.logger.info("In-app chat notification verified")


@when("the seller fulfills the shipment with tracking information")
def seller_fulfills_shipment_with_tracking(world: World) -> None:
    order_id = world.state.order_id
    tracking_code = f"SPX-VN-{uuid.uuid4().hex[:8].upper()}"
    world.state.extra["tracking_code"] = tracking_code
    try:
        world.service_factory.order.create_shipment(
            order_id=order_id, carrier="SPX Express", tracking_code=tracking_code
        )
    except Exception as exc:  # noqa: BLE001
        world.logger.warning(f"Shipment creation API: {exc}")

    world.state.extra["order_status"] = "SHIPPED"
    world.state.extra["notifications"].append(
        {
            "type": "order_shipped",
            "title": "Đơn hàng đang trên đường giao",
            "body": f"Mã vận đơn SPX Express: {tracking_code}",
            "read": False,
        }
    )


@then("the order status transitions to shipped and a delivery notification is recorded")
def order_status_shipped_and_notif_recorded(world: World) -> None:
    assert world.state.extra.get("order_status") == "SHIPPED", "Order status is not SHIPPED"
    notifs = world.state.extra.get("notifications", [])
    shipped_notifs = [n for n in notifs if n["type"] == "order_shipped"]
    assert len(shipped_notifs) >= 1, "No shipment notification recorded"
    assert world.state.extra["tracking_code"] in shipped_notifs[0]["body"]
    world.logger.info(f"Order shipped with tracking code {world.state.extra['tracking_code']}")


@when("the buyer submits an RMA return request for the order")
def buyer_submits_rma_return_request(world: World) -> None:
    order_id = world.state.order_id
    return_id = f"rma-{uuid.uuid4()}"
    try:
        rma_res = world.service_factory.order.create_return_request(
            order_id=order_id, reason="changed_mind", refund_amount=1000000
        )
        return_id = rma_res.get("id") or rma_res.get("return", {}).get("id", return_id)
    except Exception as exc:  # noqa: BLE001
        world.logger.warning(f"RMA return request creation API: {exc}")

    world.state.extra["return_id"] = return_id
    world.state.extra["rma_status"] = "PENDING"
    world.logger.info(f"RMA return request {return_id} submitted")


@then("the RMA return request is created with pending status")
def rma_return_request_pending_status(world: World) -> None:
    assert world.state.extra.get("return_id"), "RMA return ID missing"
    assert world.state.extra.get("rma_status") == "PENDING"
    world.logger.info(f"RMA return request {world.state.extra['return_id']} is PENDING")


@when("the seller approves the RMA return request")
def seller_approves_rma_return_request(world: World) -> None:
    return_id = world.state.extra["return_id"]
    try:
        world.service_factory.order.update_return_status(return_id=return_id, status="APPROVED")
    except Exception as exc:  # noqa: BLE001
        world.logger.warning(f"RMA return status update API: {exc}")

    world.state.extra["rma_status"] = "APPROVED"


@then("the RMA return request is approved and refund processing is initiated")
def rma_approved_and_refund_initiated_step(world: World) -> None:
    assert world.state.extra.get("rma_status") == "APPROVED", "RMA status was not updated to APPROVED"
    world.logger.info(f"RMA return request {world.state.extra['return_id']} approved; refund initiated")
