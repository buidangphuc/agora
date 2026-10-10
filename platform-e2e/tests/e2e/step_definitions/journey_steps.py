"""Step definitions for end-to-end user journeys in Agora marketplace.

Covers:
1. Buyer Full Funnel Journey (Recommendations, Telemetry, Search, PDP, Cart, Promo, Checkout, GA4 DataLayer)
2. Seller Cockpit & Forecast Journey (Publishing, Inventory, Funnel Analytics, Demand Forecasting)
3. Post-Purchase Chat & RMA Journey (Chat Thread, Notifications, Fulfillment, RMA Return Flow)
"""

from __future__ import annotations

import re
import time
import uuid

from playwright.sync_api import expect
from pytest_bdd import given, then, when

from config.settings import get_settings
from src.api.services import BaseService
from src.api.services.sharing_service import SharingService
from src.constants import PageName, timeouts
from src.constants import gateway_endpoints as ep
from src.models import Listing
from src.pages import (
    CartPage,
    CheckoutPage,
    ListingDetailPage,
    OrdersListPage,
    SearchPage,
)
from src.utils import data as fake
from tests.e2e.support.world import World

SETTINGS = get_settings()


# ============================================================================
# Journey 1: Buyer Full Funnel Journey
#
# Every step drives the running stack: the Next.js UI through Playwright and the
# gateway API for the read-backs, as the @needsBuyer account the browser is logged
# in as. Every Then reads state back from the system (see the feature file notes).
# ============================================================================

_INDEX_WAIT_SECONDS = 60
_FAVORITE_WAIT_SECONDS = 10
_PURCHASE_VOUCHER = "SAVE10"
# The served placement (recs-serving-safeguards: the storefront uses RecommendResponse.placement_id).
_RECS_PLACEMENT = "home_feed"


def _digits(text: str) -> int:
    """All digits of a rendered amount, e.g. '5.000.000 VND' -> 5000000."""
    cleaned = re.sub(r"\D", "", text or "")
    assert cleaned, f"no amount in {text!r}"
    return int(cleaned)


def _data_layer_events(world: World, name: str) -> list[dict]:
    events = world.page.evaluate("() => (window.dataLayer || []).filter((e) => e && e.event)")
    return [e for e in events if e.get("event") == name]


def _impression_item_ids(events: list[dict], placement: str | None = None) -> set[str]:
    ids = set()
    for event in events:
        for item in (event.get("ecommerce") or {}).get("items", []):
            if placement is None or item.get("item_list_id") == placement:
                ids.add(item.get("item_id"))
    return ids


def _card_listing_ids(cards) -> set[str]:  # noqa: ANN001
    hrefs = cards.evaluate_all("els => els.map((e) => e.getAttribute('href') || '')")
    return {h.rsplit("/", 1)[-1] for h in hrefs if h.startswith("/listing/")}


def _seeded_listing_id(world: World) -> str:
    listing = world.state.listing
    assert listing and listing.listing_id, "no seeded listing; tag the scenario @needsListing"
    return listing.listing_id


@then("a viewable impression event is emitted to the data layer for a home listing card")
def home_listing_card_impression(world: World) -> None:
    cards = world.page.locator('a[href^="/listing/"]')
    expect(cards.first).to_be_visible(timeout=timeouts.NAVIGATION)
    cards.first.scroll_into_view_if_needed()
    # The impression fires from an IntersectionObserver once the card is in view.
    world.page.wait_for_function(
        "() => (window.dataLayer || []).some((e) => e && e.event === 'view_item_list')",
        timeout=timeouts.DEFAULT,
    )
    ids = _impression_item_ids(_data_layer_events(world, "view_item_list"))
    on_page = _card_listing_ids(cards)
    assert ids, "view_item_list events carry no item"
    assert ids <= on_page, f"impression for a listing that is not on the page: {ids - on_page}"


@when("the buyer searches for the seeded listing and filters by its price range")
def buyer_searches_seeded_listing_and_filters_price(world: World) -> None:
    listing = world.state.listing
    listing_id = _seeded_listing_id(world)
    # The seeded title ends with a random number: a near-unique keyword on a shared stack.
    keyword = listing.title.split()[-1]
    index = world.service_factory.get(BaseService)
    deadline = time.time() + _INDEX_WAIT_SECONDS
    while True:
        hits = index.post(ep.SEARCH_LISTINGS, {"query": keyword}).get("hits") or []
        if listing_id in {h.get("listingId") for h in hits} or time.time() >= deadline:
            break
        time.sleep(2)
    assert listing_id in {h.get("listingId") for h in hits}, (
        f"seeded listing {listing_id} is not in the search index for {keyword!r} "
        f"after {_INDEX_WAIT_SECONDS}s"
    )
    world.state.search_term = keyword

    world.get_page(PageName.HOME).search_for(keyword)  # type: ignore[attr-defined]
    search: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    expect(search.results_wrapper).to_be_visible(timeout=timeouts.NAVIGATION)
    # Facet keys are "<min>-<max>" or "<min>+"; click the bucket the listing's price falls in.
    # The facet sidebar streams in after the results wrapper; reading the buckets before it
    # renders returned [] under -n 4 load, so wait for the first bucket first.
    expect(search.facet_group("price_ranges").locator("[data-key]").first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    keys = (
        search.facet_group("price_ranges")
        .locator("[data-key]")
        .evaluate_all("els => els.map((e) => e.getAttribute('data-key'))")
    )
    price = listing.price
    in_range = []
    for key in keys:
        low, _, high = key.replace("+", "-").partition("-")
        if int(low) <= price and (not high or price < int(high)):
            in_range.append(key)
    assert in_range, f"no price facet bucket holds {price}: {keys}"
    key = in_range[0]
    search.facet_bucket(key).click()
    world.state.extra["price_bucket"] = key
    world.page.wait_for_url(
        re.compile(rf".*minPrice={key.split('-')[0].rstrip('+')}.*"), timeout=timeouts.NAVIGATION
    )


@then("the filtered search results include the seeded listing")
def filtered_results_include_seeded_listing(world: World) -> None:
    search: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    key = world.state.extra["price_bucket"]
    expect(search.facet_bucket(key)).to_have_attribute(
        "data-active", "true", timeout=timeouts.DEFAULT
    )
    link = search.results_wrapper.locator(f'a[href="/listing/{_seeded_listing_id(world)}"]').first
    expect(link).to_be_visible(timeout=timeouts.DEFAULT)
    assert search.result_count() > 0, "the filtered search rendered no results"


@when("the buyer opens the seeded listing from the search results")
def buyer_opens_seeded_listing_from_results(world: World) -> None:
    listing_id = _seeded_listing_id(world)
    search: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    search.results_wrapper.locator(f'a[href="/listing/{listing_id}"]').first.click()
    world.page.wait_for_url(re.compile(rf".*/listing/{listing_id}$"), timeout=timeouts.NAVIGATION)


@then("the product detail page displays the seeded listing's title and price")
def pdp_displays_seeded_listing(world: World) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    stored = world.service_factory.listing.get_listing(_seeded_listing_id(world))
    assert stored.get("title"), f"GetListing returned no listing: {stored}"
    expect(detail.title).to_have_text(stored["title"], timeout=timeouts.DEFAULT)
    expect(detail.price).to_be_visible(timeout=timeouts.DEFAULT)
    assert _digits(detail.price.inner_text()) == int(
        stored["price"]
    ), f"PDP price {detail.price.inner_text()!r} != stored price {stored['price']}"
    expect(detail.add_to_cart_button).to_be_visible(timeout=timeouts.DEFAULT)


@when("the buyer favorites the listing and generates a share link")
def buyer_favorites_and_shares(world: World) -> None:
    page = world.page
    page.get_by_role("button", name="Yêu thích", exact=True).first.click()
    expect(page.get_by_text("Đã thêm sản phẩm vào mục Yêu Thích")).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    page.get_by_role("button", name=re.compile("Chia sẻ")).click()
    shown = page.get_by_text(re.compile(r"/s/[A-Za-z0-9_-]+$")).first
    expect(shown).to_be_visible(timeout=timeouts.DEFAULT)
    world.state.extra["share_url"] = shown.inner_text().strip()


@then("the listing is a favorite of the buyer and the share link resolves to the listing")
def listing_favorited_and_share_link_resolves(world: World) -> None:
    listing_id = _seeded_listing_id(world)
    deadline = time.time() + _FAVORITE_WAIT_SECONDS
    while True:
        favorites = world.service_factory.engagement.list_favorites().get("listingIds") or []
        if listing_id in favorites or time.time() >= deadline:
            break
        time.sleep(1)
    assert listing_id in favorites, f"ListFavorites does not contain {listing_id}: {favorites}"

    share_url = world.state.extra["share_url"]
    short_code = share_url.rsplit("/s/", 1)[-1]
    resolved = world.service_factory.get(SharingService).resolve_share_link(short_code)
    assert resolved.get("targetType") == "listing", f"share link target: {resolved}"
    assert resolved.get("targetId") == listing_id, f"share link points elsewhere: {resolved}"

    # Open the link like a recipient would: it must land on the listing page.
    recipient = world.context.new_page()
    try:
        recipient.goto(share_url, wait_until="domcontentloaded")
        expect(recipient).to_have_url(
            re.compile(rf".*/listing/{listing_id}$"), timeout=timeouts.NAVIGATION
        )
    finally:
        recipient.close()


@when("the buyer adds the listing to the cart and raises its quantity to 2")
def buyer_adds_listing_and_raises_quantity(world: World) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    detail.add_to_cart_button.click()
    expect(world.page.get_by_text("Đã thêm 1 sản phẩm vào giỏ hàng")).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    cart: CartPage = world.navigate_to(PageName.CART)  # type: ignore[assignment]
    quantity = world.page.get_by_role("spinbutton", name=f"Số lượng: {world.state.listing.title}")
    expect(quantity).to_have_attribute("aria-valuenow", "1", timeout=timeouts.NAVIGATION)
    cart.increase_quantity_button.click()
    expect(quantity).to_have_attribute("aria-valuenow", "2", timeout=timeouts.DEFAULT)


@then("the cart holds 2 units of the listing")
def cart_holds_two_units(world: World) -> None:
    listing = world.state.listing
    cart = world.service_factory.cart.get_cart().get("cart", {})
    items = cart.get("items") or []
    assert len(items) == 1, f"expected one cart line, got {items}"
    line = items[0]
    assert line.get("listingId") == listing.listing_id, f"wrong cart line: {line}"
    assert int(line.get("quantity", 0)) == 2, f"cart quantity: {line}"
    assert int(line.get("unitPrice", 0)) == listing.price, f"cart unit price: {line}"
    subtotal = int(cart.get("subtotal", 0))
    assert subtotal == 2 * listing.price, f"cart subtotal {subtotal}"
    world.state.extra["subtotal"] = subtotal  # the voucher step compares against it

    page_cart: CartPage = world.get_page(PageName.CART)  # type: ignore[assignment]
    quantity = world.page.get_by_role("spinbutton", name=f"Số lượng: {listing.title}")
    expect(quantity).to_have_attribute("aria-valuenow", "2", timeout=timeouts.DEFAULT)
    shown = f"{subtotal:,}".replace(",", ".")  # the UI groups thousands with dots
    expect(page_cart.order_summary).to_contain_text(shown, timeout=timeouts.DEFAULT)


@when("the buyer confirms the order placement")
def buyer_confirms_order_placement(world: World) -> None:
    checkout: CheckoutPage = world.get_page(PageName.CHECKOUT)  # type: ignore[assignment]
    checkout.continue_to("confirm")  # the voucher stays in ?voucher= across the steps
    expect(checkout.place_order_button).to_be_enabled(timeout=timeouts.DEFAULT)
    checkout.place_order_button.click()
    # COD routes to the buyer's order list; a mock-pay method would route to /checkout/pay.
    world.page.wait_for_url(re.compile(r".*/account/orders.*success=1.*"), timeout=timeouts.LONG)


@then("the order is placed with the voucher discount and listed for the buyer")
def order_placed_with_discount_and_listed(world: World) -> None:
    buyer, seller = _buyer(world), _seller(world)
    listing = world.state.listing
    orders = world.service_factory.order.list_buyer_orders().get("orders") or []
    assert len(orders) == 1, f"the buyer should have exactly the one new order: {orders}"
    order_id = orders[0]["id"]
    order = world.service_factory.order.get_order(order_id).get("order", {})
    assert order.get("id") == order_id, f"GetOrder returned the wrong order: {order}"
    world.state.order_id = order_id
    world.state.extra["order_id"] = order_id

    subtotal = world.state.extra["subtotal"]
    discount = subtotal * 10 // 100  # SAVE10: platform voucher, 10%, uncapped
    assert order.get("buyerId") == buyer.user_id, f"order is not the buyer's: {order}"
    assert order.get("sellerId") == seller.user_id, f"order is not the seller's: {order}"
    assert order.get("status") == "ORDER_STATUS_PENDING", f"order status: {order}"
    assert order.get("paymentMethod") == "PAYMENT_METHOD_COD", f"payment method: {order}"
    assert order.get("voucherCode") == _PURCHASE_VOUCHER, f"voucher code: {order}"
    assert int(order.get("itemsSubtotal", 0)) == subtotal, f"items subtotal: {order}"
    assert int(order.get("discountAmount", 0)) == discount, f"discount: {order}"
    assert int(order.get("totalAmount", 0)) == subtotal - discount, f"total: {order}"
    lines = order.get("items") or []
    assert [(i.get("listingId"), int(i.get("quantity", 0))) for i in lines] == [
        (listing.listing_id, 2)
    ], f"order items: {lines}"
    assert (
        order.get("shippingAddress", {}).get("id") == world.state.extra["address_id"]
    ), f"shipping address: {order.get('shippingAddress')}"
    world.state.extra["order_total"] = int(order["totalAmount"])

    # The buyer's order list in the UI shows that same order.
    orders_page: OrdersListPage = world.get_page(PageName.ACCOUNT_ORDERS)  # type: ignore[assignment]
    expect(orders_page.order_cards.first).to_be_visible(timeout=timeouts.DEFAULT)
    detail = world.page.locator(f'a[href="/account/orders/{order_id}"]')
    expect(detail.first).to_be_visible(timeout=timeouts.DEFAULT)


@then("a purchase event for that order is pushed to the GA4 dataLayer")
def purchase_event_pushed_for_order(world: World) -> None:
    listing = world.state.listing
    purchases = _data_layer_events(world, "purchase")
    assert len(purchases) == 1, f"expected exactly one purchase event, got {purchases}"
    ecommerce = purchases[0].get("ecommerce") or {}
    assert (
        ecommerce.get("transaction_id") == world.state.order_id
    ), f"transaction_id {ecommerce.get('transaction_id')!r} is not the order id {world.state.order_id}"
    assert ecommerce.get("currency") == "VND", f"currency: {ecommerce}"
    assert ecommerce.get("value") == world.state.extra["order_total"], f"value: {ecommerce}"
    assert ecommerce.get("coupon") == _PURCHASE_VOUCHER, f"coupon: {ecommerce}"
    items = ecommerce.get("items") or []
    assert [(i.get("item_id"), i.get("quantity")) for i in items] == [
        (listing.listing_id, 2)
    ], f"purchase items: {items}"


@then('the "Gợi ý cho bạn" row on the home page shows product cards')
def home_recommendations_row_shows_cards(world: World) -> None:
    row = world.get_page(PageName.HOME).recommendations  # type: ignore[attr-defined]
    expect(row.heading).to_be_visible(timeout=timeouts.DEFAULT)
    expect(row.cards.first).to_be_visible(timeout=timeouts.DEFAULT)


@then("a viewable impression event is emitted to the data layer for the recommendations row")
def recommendations_row_impression(world: World) -> None:
    row = world.get_page(PageName.HOME).recommendations  # type: ignore[attr-defined]
    row.cards.first.scroll_into_view_if_needed()
    world.page.wait_for_function(
        """(placement) => (window.dataLayer || []).some((e) => e && e.event === 'view_item_list'
            && ((e.ecommerce || {}).items || []).some((i) => i.item_list_id === placement))""",
        arg=_RECS_PLACEMENT,
        timeout=timeouts.DEFAULT,
    )
    ids = _impression_item_ids(_data_layer_events(world, "view_item_list"), _RECS_PLACEMENT)
    assert ids <= _card_listing_ids(row.cards), "impression for a listing that is not in the row"


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
#   * CreateShipment (seller) works on a PENDING or PAID order and moves it to SHIPPED
#     with the tracking code (team-order service/order.go CreateShipment);
#   * a return may be opened on any non-PENDING, non-CANCELLED order, so a
#     SHIPPED one qualifies (service/order.go CreateReturnRequest);
#   * UpdateReturnStatus only changes the return's own status - it does not
#     call team-payment, so approval starts no refund;
#   * team-chat publishes each message to chat.events (with the other participant
#     as recipient_id) and CreateShipment writes OrderShipped to order.events;
#     team-notification consumes both and creates the CHAT / ORDER notification
#     for that one user, unless they switched that type off.
# ============================================================================

_NOTIFICATION_POLL_SECONDS = 30


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


def _find_notifications(world: World, user, kind: str, needle: str) -> list[dict]:
    """The `user`'s notifications of `kind` mentioning `needle` in title, body or link."""
    _act_as(world, user)
    return [
        n
        for n in world.service_factory.notification.list_notifications()
        if n.get("type") == kind
        and needle in f"{n.get('title', '')} {n.get('body', '')} {n.get('linkUrl', '')}"
    ]


def _poll_notification(world: World, user, kind: str, needle: str) -> list[dict]:
    """Bounded poll for a `kind` notification of `user` mentioning `needle`."""
    deadline = time.time() + _NOTIFICATION_POLL_SECONDS
    while True:
        found = _find_notifications(world, user, kind, needle)
        if found or time.time() >= deadline:
            return found
        time.sleep(2)


def _poll_buyer_notification(world: World, kind: str, needle: str) -> list[dict]:
    return _poll_notification(world, _buyer(world), kind, needle)


@then("the buyer has a chat notification for the seller's reply")
def buyer_has_chat_notification(world: World) -> None:
    thread_id = world.state.extra["chat_thread_id"]
    found = _poll_buyer_notification(world, "NOTIFICATION_TYPE_CHAT", f"/chat/{thread_id}")
    assert found, "no NOTIFICATION_TYPE_CHAT notification for the buyer after the seller's reply"
    assert len(found) == 1, f"expected exactly one chat notification for the buyer: {found}"
    assert found[0].get("body") == world.state.extra["chat_reply"], found[0]


@then("the seller has no chat notification for their own reply")
def seller_has_no_chat_notification_for_own_reply(world: World) -> None:
    # The buyer's inquiry notifies the seller; the seller's own reply must not.
    found = _find_notifications(
        world, _seller(world), "NOTIFICATION_TYPE_CHAT", world.state.extra["chat_reply"]
    )
    assert not found, f"the seller was notified of their own message: {found}"


@given("the buyer has disabled chat notifications")
def buyer_disabled_chat_notifications(world: World) -> None:
    _act_as(world, _buyer(world))
    prefs = world.service_factory.notification.set_type_enabled("NOTIFICATION_TYPE_CHAT", False)
    assert prefs.get("typeEnabled", {}).get("NOTIFICATION_TYPE_CHAT") is False, prefs


@when("the buyer sends a follow-up chat message")
def buyer_sends_follow_up_chat_message(world: World) -> None:
    text = f"Cảm ơn shop, mình chờ nhận hàng nhé! ({uuid.uuid4().hex[:6]})"
    _act_as(world, _buyer(world))
    msg = world.service_factory.chat.send_message(world.state.extra["chat_thread_id"], text)
    assert msg.get("id"), f"SendMessage returned no message: {msg}"
    world.state.extra["chat_follow_up"] = text


@then("the seller has a chat notification for the follow-up")
def seller_has_chat_notification_for_follow_up(world: World) -> None:
    found = _poll_notification(
        world, _seller(world), "NOTIFICATION_TYPE_CHAT", world.state.extra["chat_follow_up"]
    )
    assert found, "the seller got no chat notification for the buyer's follow-up"


@then("the buyer has no chat notification for the seller's reply")
def buyer_has_no_chat_notification(world: World) -> None:
    found = _find_notifications(
        world,
        _buyer(world),
        "NOTIFICATION_TYPE_CHAT",
        f"/chat/{world.state.extra['chat_thread_id']}",
    )
    assert not found, f"a chat notification was created despite the disabled preference: {found}"


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
    assert len(found) == 1, f"expected exactly one order notification: {found}"
    assert found[0].get("linkUrl") == f"/account/orders/{world.state.order_id}", found[0]


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
