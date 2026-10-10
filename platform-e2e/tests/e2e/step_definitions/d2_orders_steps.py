"""Steps for the buyer order screens (OpenSpec change ui-phase-orders), features
frontend/ui_orders_*.feature. Seeding goes through the gateway as the logged-in buyer and
the order's seller; assertions read the rendered page and, where the spec promises state,
the gateway."""

from __future__ import annotations

import re
import time

from playwright.sync_api import Page, expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from src.pages import OrderDetailPage, OrdersListPage
from tests.e2e.step_definitions.d2_cart_steps import toast
from tests.e2e.step_definitions.d2_checkout_steps import slow_actions, track_actions
from tests.e2e.support import d2_support as d2
from tests.e2e.support.world import World

TONE_CLASS = {
    "warning": "text-accent-promo-dark",
    "info": "bg-surface-muted",
    "success": "text-accent-success-dark",
    "neutral": "text-text-secondary",
    "danger": "text-danger",
}


def base(world: World) -> str:
    return world.settings.base_url.rstrip("/")


def lst(world: World) -> OrdersListPage:
    return OrdersListPage(world.page)


def det(world: World) -> OrderDetailPage:
    return OrderDetailPage(world.page)


def open_orders(world: World, query: str = "") -> None:
    world.page.goto(f"{base(world)}/account/orders{query}", wait_until="domcontentloaded")


def open_detail(world: World, order_id: str | None = None, query: str = "") -> None:
    oid = order_id or world.state.extra["d2_order_id"]
    world.page.goto(f"{base(world)}/account/orders/{oid}{query}", wait_until="domcontentloaded")


# ── Seeding ──────────────────────────────────────────────────────────────
@given("a d2 buyer has no orders")
def buyer_no_orders(world: World) -> None:
    d2.seed_buyer(world)
    world.state.extra["d2_shops"] = []


@given(parsers.parse("a d2 buyer has {count:d} orders from one shop"))
def buyer_n_orders(world: World, count: int) -> None:
    d2.seed_buyer(world)
    shop = d2.seed_shop(price=100_000, stock=100)
    world.state.extra["d2_shops"] = [shop]
    ids = [d2.place_order(world, shop) for _ in range(count)]
    world.state.extra["d2_order_ids"] = ids


@given(parsers.parse('a d2 buyer has an order from a shop named "{name}"'))
def buyer_order_named_shop(world: World, name: str) -> None:
    d2.seed_buyer(world)
    shop = d2.seed_shop(None if name == "-" else name, price=100_000, stock=100)
    world.state.extra["d2_shops"] = [shop]
    d2.place_order(world, shop)


@given("a d2 buyer has a pending, a shipped, a completed and a cancelled order")
def buyer_four_orders(world: World) -> None:
    d2.seed_buyer(world)
    shop = d2.seed_shop(price=100_000, stock=100)
    world.state.extra["d2_shops"] = [shop]
    ids = {}
    ids["pending"] = d2.place_order(world, shop)
    ids["shipped"] = d2.place_order(world, shop)
    d2.ship_order(shop, ids["shipped"])
    ids["completed"] = d2.place_order(world, shop)
    d2.ship_order(shop, ids["completed"], complete=True)
    ids["cancelled"] = d2.place_order(world, shop)
    d2.buyer_orders(world).cancel_order(ids["cancelled"], "e2e")
    world.state.extra["d2_order_map"] = ids


@given(parsers.parse("a d2 buyer has a single {kind} order"))
def buyer_one_order(world: World, kind: str) -> None:
    d2.seed_buyer(world)
    shop = d2.seed_shop(price=100_000, stock=100, image=True)
    world.state.extra["d2_shops"] = [shop]
    oid = d2.place_order(world, shop)
    if kind == "shipped":
        d2.ship_order(shop, oid)
    elif kind == "completed":
        d2.ship_order(shop, oid, complete=True)
    elif kind == "cancelled":
        d2.buyer_orders(world).cancel_order(oid, "e2e")
    elif kind == "payment-failed":
        d2.force_fail_payment(oid)
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            if d2.order_statuses(world).get(oid) == "ORDER_STATUS_CANCELLED":
                break
            time.sleep(0.5)
    else:
        assert kind == "pending", kind
    world.state.extra["d2_order_id"] = oid


# ── List: URL state, pagination ──────────────────────────────────────────
@when("the d2 buyer opens the orders list")
def when_open_orders(world: World) -> None:
    open_orders(world)


@when(parsers.parse('the d2 buyer opens the orders list with the query "{query}"'))
def when_open_orders_query(world: World, query: str) -> None:
    open_orders(world, f"?{query}" if query else "")


@when("the d2 buyer opens the orders list as a guest")
def when_guest_orders(world: World) -> None:
    world.context.clear_cookies()
    open_orders(world)


@then(parsers.parse("the browser lands on the login page"))
def lands_login(world: World) -> None:
    world.page.wait_for_url(re.compile(r".*/login(\?.*)?$"), timeout=timeouts.NAVIGATION)


@then(parsers.parse('the "{label}" tab is current and {count:d} orders are listed'))
def tab_current_and_count(world: World, label: str, count: int) -> None:
    expect(lst(world).tab(label)).to_have_attribute(
        "aria-current", "page", timeout=timeouts.DEFAULT
    )
    expect(lst(world).order_cards).to_have_count(count, timeout=timeouts.DEFAULT)


@when("the d2 buyer navigates away and comes back with the browser back button")
def away_and_back(world: World) -> None:
    world.page.goto(f"{base(world)}/cart", wait_until="domcontentloaded")
    world.page.go_back(wait_until="domcontentloaded")


@then(parsers.parse('the orders URL has the query "{query}"'))
def url_query(world: World, query: str) -> None:
    expect(world.page).to_have_url(
        re.compile(rf".*/account/orders\?{re.escape(query)}$"), timeout=timeouts.DEFAULT
    )


@then(parsers.parse("the pagination has {pages:d} pages"))
def pagination_pages(world: World, pages: int) -> None:
    nav = lst(world).pagination
    expect(nav).to_be_visible(timeout=timeouts.DEFAULT)
    expect(nav.get_by_role("link", name=f"Trang {pages}")).to_be_visible()
    expect(nav.get_by_role("link", name=f"Trang {pages + 1}")).to_have_count(0)


@when(parsers.parse('the d2 buyer follows the pagination link "{number:d}"'))
def follow_page(world: World, number: int) -> None:
    lst(world).page_link(number).click()
    world.page.wait_for_url(re.compile(rf".*page={number}\b.*"), timeout=timeouts.DEFAULT)


@then("the all tab is active and the last page is shown without an error")
def invalid_query_fallback(world: World) -> None:
    page = world.page
    expect(lst(world).tab("Tất cả")).to_have_attribute(
        "aria-current", "page", timeout=timeouts.DEFAULT
    )
    expect(lst(world).order_cards).to_have_count(1, timeout=timeouts.DEFAULT)
    expect(page.get_by_role("alert").filter(has_text="lỗi")).to_have_count(0)
    expect(page.get_by_text("Application error")).to_have_count(0)


# ── List: real data, badges, empty ───────────────────────────────────────
@then(parsers.parse('the order row shows "{name}" and not "Shop #"'))
def row_real_name(world: World, name: str) -> None:
    card = lst(world).order_cards.first
    expect(card).to_contain_text(name, timeout=timeouts.DEFAULT)
    expect(card).not_to_contain_text("Shop #")


@then('the order row shows "Shop #" followed by the first 6 characters of the seller id')
def row_fallback(world: World) -> None:
    shop = world.state.extra["d2_shops"][0]
    expect(lst(world).order_cards.first).to_contain_text(
        f"Shop #{shop['seller_id'][:6]}", timeout=timeouts.DEFAULT
    )


@then(
    "the pending, shipped, completed and cancelled orders render warning, info, success and neutral badges"
)
def badge_tones(world: World) -> None:
    ids = world.state.extra["d2_order_map"]
    expected = {
        "pending": "warning",
        "shipped": "info",
        "completed": "success",
        "cancelled": "neutral",
    }
    cards = lst(world).order_cards
    expect(cards).to_have_count(4, timeout=timeouts.DEFAULT)
    seen = {}
    for i in range(4):
        card = cards.nth(i)
        href = card.get_by_role("link", name="Xem chi tiết").get_attribute("href")
        oid = href.rsplit("/", 1)[-1]
        kind = next(k for k, v in ids.items() if v == oid)
        badge = card.get_by_test_id("order-status")
        text = badge.inner_text().strip()
        classes = badge.get_attribute("class") or ""
        assert text, kind
        assert TONE_CLASS[expected[kind]] in classes, (kind, text, classes)
        assert "action-primary" not in classes and "primary-" not in classes, classes
        seen[kind] = text
    assert len(set(seen.values())) == 4, seen


@then("the empty state offers a shopping link and every tab count reads 0")
def empty_all(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Bạn chưa có đơn hàng nào")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.get_by_role("link", name="Mua sắm ngay")).to_have_attribute("href", "/")
    tabs = lst(world).tabs
    counts = [t.inner_text() for t in tabs.get_by_role("link").all()]
    assert counts, "no tabs"
    for text in counts:
        assert re.search(r"\b0\b", text), counts


@when("the d2 buyer opens the orders list from the cart while the list data is slow")
def orders_slow_soft(world: World) -> None:
    from tests.e2e.step_definitions.d2_quality_steps import install_cls

    page = world.page
    install_cls(world)
    page.goto(f"{base(world)}/cart", wait_until="networkidle")

    def slow(route) -> None:  # noqa: ANN001
        time.sleep(1.5)
        route.continue_()

    page.route(re.compile(r".*/account/orders\?_rsc=.*"), slow)
    page.evaluate("window.__cls = 0")
    page.locator('a[href="/account/orders"]').first.click()


@then(
    "a tab bar and three order card skeletons show first and the content then replaces them without a layout shift"
)
def orders_skeleton(world: World) -> None:
    from tests.e2e.step_definitions.d2_quality_steps import cls

    page = world.page
    skeleton = page.locator('section[aria-busy="true"]')
    expect(skeleton).to_be_visible(timeout=timeouts.DEFAULT)
    assert skeleton.locator(".animate-pulse").count() >= 6, "tab bar and card skeletons missing"
    expect(lst(world).order_cards.first).to_be_visible(timeout=timeouts.NAVIGATION)
    page.wait_for_timeout(1000)
    soft = cls(page)
    page.goto(f"{base(world)}/account/orders", wait_until="load")
    expect(lst(world).order_cards.first).to_be_visible(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(1500)
    hard = cls(page)
    assert soft == 0 and hard == 0, f"cumulative layout shift: soft {soft}, hard {hard}"


@then("no recipient, item or amount of that order appears in the page")
def forbidden_leaks_nothing(world: World) -> None:
    body = world.page.locator("main").inner_text()
    for needle in ("Nguyen Van A", "Phuong Lieu Giai", "Order #", "Tổng thanh toán", "x1"):
        assert needle not in body, f"{needle!r} leaked into the 403 page"


# ── Row actions: reorder and cancel ──────────────────────────────────────
def first_card_button(world: World, label: str):  # noqa: ANN202
    card = lst(world).order_cards.first
    expect(card).to_be_visible(timeout=timeouts.DEFAULT)
    return card.get_by_role("button", name=label, exact=True)


@when(parsers.parse('the d2 buyer presses "{label}" on the first order while the server is slow'))
def press_first_slow(world: World, label: str) -> None:
    page = world.page
    button = first_card_button(world, label)
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)  # island hydration
    world.state.extra["d2_width"] = button.bounding_box()["width"]
    slow_actions(world, 1.5, "**/account/orders**")
    track_actions(world)
    button.click()


@then("the reorder button is busy, disabled and keeps its width")
def reorder_pending(world: World) -> None:
    button = first_card_button(world, "Mua lại")
    expect(button).to_have_attribute("aria-busy", "true", timeout=timeouts.DEFAULT)
    expect(button).to_be_disabled()
    assert abs(button.bounding_box()["width"] - world.state.extra["d2_width"]) < 1


@then("a success toast shows and the buyer lands on the cart holding the order's item")
def reorder_success(world: World) -> None:
    page = world.page
    expect(toast(page, "Đã thêm lại sản phẩm")).to_be_visible(timeout=timeouts.NAVIGATION)
    page.wait_for_url(re.compile(r".*/cart$"), timeout=timeouts.NAVIGATION)
    shop = world.state.extra["d2_shops"][0]
    items = d2.buyer_cart(world).get_cart()["cart"]["items"]
    assert any(i["listingId"] == shop["listing_id"] for i in items), items


@given("the order's listing was deleted by its seller")
def listing_deleted(world: World) -> None:
    from src.api.services import ListingService

    shop = world.state.extra["d2_shops"][0]
    ListingService(token=shop["token"]).delete_listing(shop["listing_id"])


@then(
    "an error toast shows the reason, the reorder button is enabled and the buyer stays on the list"
)
def reorder_failed(world: World) -> None:
    page = world.page
    expect(
        page.get_by_role("alert")
        .filter(has_text=re.compile(r"\S{4,}"))
        .filter(has_not_text="Đóng")
        .first
    ).to_be_visible(timeout=timeouts.NAVIGATION)
    assert "/account/orders" in page.url, page.url
    expect(first_card_button(world, "Mua lại")).to_be_enabled()


@when("the d2 buyer opens the cancel Modal of the first order")
def open_cancel_modal(world: World) -> None:
    page = world.page
    button = first_card_button(world, "Hủy đơn")
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)
    button.click()
    expect(page.get_by_role("dialog")).to_be_visible(timeout=timeouts.DEFAULT)


@when("the d2 buyer confirms the cancellation while the server is slow")
def confirm_cancel_slow(world: World) -> None:
    slow_actions(world, 1.5, "**/account/orders**")
    track_actions(world)
    world.page.get_by_test_id("cancel-confirm").click()


@then("the confirm button is busy and the Modal cannot be dismissed")
def cancel_pending(world: World) -> None:
    page = world.page
    confirm = page.get_by_test_id("cancel-confirm")
    expect(confirm).to_have_attribute("aria-busy", "true", timeout=timeouts.DEFAULT)
    expect(page.get_by_role("button", name="Không hủy")).to_be_disabled()
    page.keyboard.press("Escape")
    expect(page.get_by_role("dialog")).to_be_visible()


@then('a success toast shows, the Modal closes and the order badge reads "Đã hủy"')
def cancel_success(world: World) -> None:
    page = world.page
    expect(toast(page, "Đã hủy đơn hàng thành công")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_role("dialog")).to_have_count(0, timeout=timeouts.DEFAULT)
    expect(lst(world).order_statuses.first).to_have_text("Đã hủy", timeout=timeouts.DEFAULT)
    assert d2.order_statuses(world)[world.state.extra["d2_order_id"]] == "ORDER_STATUS_CANCELLED"


@given("the order was completed by its seller after the list was rendered")
@when("the order is completed by its seller behind the buyer's back")
def completed_behind_back(world: World) -> None:
    shop = world.state.extra["d2_shops"][0]
    d2.ship_order(shop, world.state.extra["d2_order_id"], complete=True)


@when("the d2 buyer confirms the cancellation")
def confirm_cancel(world: World) -> None:
    world.page.get_by_test_id("cancel-confirm").click()


@then("an error toast shows, the Modal stays open and the confirm button is enabled")
def cancel_failed(world: World) -> None:
    page = world.page
    expect(
        page.get_by_role("alert")
        .filter(has_text=re.compile(r"\S{4,}"))
        .filter(has_not_text="Đóng")
        .first
    ).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_role("dialog")).to_be_visible()
    expect(page.get_by_test_id("cancel-confirm")).to_be_enabled()
    assert d2.order_statuses(world)[world.state.extra["d2_order_id"]] == "ORDER_STATUS_COMPLETED"


# ── Detail ───────────────────────────────────────────────────────────────
@when("the d2 buyer opens the order detail")
def when_open_detail(world: World) -> None:
    open_detail(world)


@when(parsers.parse('the d2 buyer opens the order detail with the query "{query}"'))
def when_open_detail_query(world: World, query: str) -> None:
    open_detail(world, query=f"?{query}")


@when("the d2 buyer opens the detail of an order that does not exist")
def when_open_missing(world: World) -> None:
    open_detail(world, "does-not-exist")


@then(
    "the detail shows the breadcrumb, id, shipped badge and total, the progress step, the descriptions, the items table and the active timeline tab"
)
def detail_anatomy(world: World) -> None:
    page = world.page
    oid = world.state.extra["d2_order_id"]
    shop = world.state.extra["d2_shops"][0]
    expect(page.get_by_role("heading", level=1)).to_have_text(
        f"Order #{oid[:8]}", timeout=timeouts.DEFAULT
    )
    crumbs = page.get_by_role("navigation", name="Breadcrumb")
    expect(crumbs.get_by_role("link", name="Đơn hàng của tôi")).to_have_attribute(
        "href", "/account/orders"
    )
    expect(crumbs).to_contain_text(f"Chi tiết #{oid[:8]}")
    badge = page.get_by_test_id("order-status")
    expect(badge).to_have_text("Đang giao hàng")
    assert TONE_CLASS["info"] in (badge.get_attribute("class") or "")
    expect(page.locator("main header").first).to_contain_text(re.compile(r"\d{3}\.\d{3}"))
    current = page.get_by_role("navigation", name="Progress").first.locator('[aria-current="step"]')
    expect(current).to_contain_text("Đang vận chuyển")
    for text in ("Người nhận", "Nguyen Van A", "Hình thức thanh toán", "Tổng thanh toán"):
        expect(page.get_by_text(text).first).to_be_visible()
    expect(page.get_by_role("table").get_by_text(shop["title"])).to_be_visible()
    expect(page.get_by_role("tab", name="Hành trình")).to_have_attribute("aria-selected", "true")
    expect(page.get_by_test_id("order-timeline")).to_be_visible()


@when('the d2 buyer selects the "Trả hàng / Hoàn tiền" tab')
def select_returns_tab(world: World) -> None:
    page = world.page
    page.wait_for_timeout(500)
    page.get_by_role("tab", name="Trả hàng / Hoàn tiền").click()


@then("the URL gains tab=returns and reloading shows the same tab")
def tab_in_url(world: World) -> None:
    page = world.page
    page.wait_for_url(re.compile(r".*[?&]tab=returns\b.*"), timeout=timeouts.DEFAULT)
    page.reload(wait_until="domcontentloaded")
    expect(page.get_by_role("tab", name="Trả hàng / Hoàn tiền")).to_have_attribute(
        "aria-selected", "true", timeout=timeouts.DEFAULT
    )
    expect(page.get_by_test_id("return-section")).to_be_visible()


@then("an alert says the order was cancelled, no stepper is rendered and cancel is not offered")
def cancelled_detail(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Đơn hàng đã hủy", exact=True)).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.get_by_role("navigation", name="Progress")).to_have_count(0)
    expect(page.get_by_role("button", name="Hủy đơn", exact=True)).to_have_count(0)


@when("the d2 buyer confirms the cancellation of the detail Modal")
def confirm_detail_cancel(world: World) -> None:
    page = world.page
    track_actions(world)
    button = page.get_by_role("button", name="Hủy đơn", exact=True)
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)
    button.click()
    expect(page.get_by_role("dialog")).to_be_visible(timeout=timeouts.DEFAULT)
    page.get_by_test_id("cancel-confirm").click()


@then('the cancel action was sent once, a success toast shows and the header badge reads "Đã hủy"')
def detail_cancelled(world: World) -> None:
    page = world.page
    expect(toast(page, "Đã hủy đơn hàng thành công")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_test_id("order-status")).to_have_text("Đã hủy", timeout=timeouts.DEFAULT)
    assert len(world.state.extra["d2_posts"]) == 1, world.state.extra["d2_posts"]
    assert d2.order_statuses(world)[world.state.extra["d2_order_id"]] == "ORDER_STATUS_CANCELLED"


@then("the return request button is not offered")
def no_return_button(world: World) -> None:
    page = world.page
    expect(page.get_by_role("heading", level=1)).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.get_by_role("button", name="Yêu cầu trả hàng", exact=True)).to_have_count(0)


# ── Timeline ─────────────────────────────────────────────────────────────
@given("the order has a shipment")
def order_has_shipment(world: World) -> None:
    from src.api.services import OrderService

    shop = world.state.extra["d2_shops"][0]
    oid = world.state.extra["d2_order_id"]
    code = f"D2{oid[:8].upper()}"
    OrderService(token=shop["token"]).create_shipment(oid, "SPX Express", code)
    world.state.extra["d2_tracking"] = code


@then("the shipment checkpoint is marked current with the carrier and tracking code in the header")
def checkpoint_current(world: World) -> None:
    page = world.page
    timeline = page.get_by_test_id("order-timeline")
    expect(timeline).to_contain_text("SPX Express", timeout=timeouts.DEFAULT)
    expect(timeline).to_contain_text(world.state.extra["d2_tracking"])
    items = page.get_by_test_id("timeline-checkpoint")
    assert items.count() >= 1
    expect(items.first).to_have_attribute("aria-current", "step")


@then("the failed and compensated steps render as error items with an error alert offering Mua lại")
def failed_saga(world: World) -> None:
    page = world.page
    failures = page.get_by_test_id("timeline-failure")
    expect(failures.first).to_be_visible(timeout=timeouts.DEFAULT)
    assert failures.count() >= 2, failures.count()
    alert = page.get_by_role("alert").filter(has_text="Đơn hàng không hoàn tất")
    expect(alert.get_by_role("button", name="Mua lại")).to_be_visible()


@then("the payment step renders as pending, no failure item exists and no error alert is shown")
def pending_saga(world: World) -> None:
    page = world.page
    steps = page.get_by_test_id("timeline-saga-step")
    expect(steps.first).to_be_visible(timeout=timeouts.DEFAULT)
    payment = steps.filter(has_text="Thanh Toán")
    expect(payment).to_have_count(1)
    expect(payment.locator("span.bg-promo")).to_have_count(1)
    expect(page.get_by_test_id("timeline-failure")).to_have_count(0)
    expect(page.get_by_role("alert").filter(has_text="Đơn hàng không hoàn tất")).to_have_count(0)


# ── Exception pages ──────────────────────────────────────────────────────
@then("a 404 result links back to the order list")
def order_404(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Không tìm thấy đơn hàng")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.get_by_role("link", name="Về đơn hàng của tôi")).to_have_attribute(
        "href", "/account/orders"
    )


# ── Returns ──────────────────────────────────────────────────────────────
def completed_order_with_return(world: World, shop: dict, status: str | None) -> tuple[str, str]:
    """A completed (paid) order, optionally with a return moved to `status` by its seller."""
    from src.api.services import OrderService, PaymentService

    oid = d2.place_order(world, shop)
    PaymentService(token=world.state.extra["d2_buyer"].token).mock_pay(oid, 120_000, True)
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        if d2.order_statuses(world).get(oid) == "ORDER_STATUS_PAID":
            break
        time.sleep(0.5)
    d2.ship_order(shop, oid, complete=True)
    if status is None:
        return oid, ""
    ret = d2.buyer_orders(world).create_return_request(oid, "defective", 120_000)["returnRequest"]
    seller = OrderService(token=shop["token"])
    if status == "REFUNDED":
        seller.update_return_status(ret["id"], "RETURN_STATUS_APPROVED")
    seller.update_return_status(ret["id"], f"RETURN_STATUS_{status}")
    return oid, ret["id"]


@given("a d2 buyer has a refunded return and a rejected return on two completed orders")
def two_returns(world: World) -> None:
    d2.seed_buyer(world)
    shop = d2.seed_shop(price=100_000, stock=100)
    world.state.extra["d2_shops"] = [shop]
    world.state.extra["d2_refunded"], _ = completed_order_with_return(world, shop, "REFUNDED")
    world.state.extra["d2_rejected"], _ = completed_order_with_return(world, shop, "REJECTED")


@then(
    "the refunded return shows a success badge and the rejected return a danger badge, both as return-status"
)
def return_badges(world: World) -> None:
    page = world.page
    for key, tone in (
        ("d2_refunded", "success"),
        ("d2_rejected", "danger"),
    ):
        open_detail(world, world.state.extra[key], "?tab=returns")
        badge = page.get_by_test_id("return-status")
        expect(badge).to_have_count(1, timeout=timeouts.DEFAULT)
        assert TONE_CLASS[tone] in (badge.get_attribute("class") or ""), (
            key,
            badge.get_attribute("class"),
        )
        text = badge.inner_text().strip()
        assert text, key
        world.state.extra.setdefault("d2_return_labels", []).append(text)
    assert len(set(world.state.extra["d2_return_labels"])) == 2, world.state.extra[
        "d2_return_labels"
    ]


@given("a d2 buyer has a completed paid order")
def completed_paid_order(world: World) -> None:
    d2.seed_buyer(world)
    shop = d2.seed_shop(price=100_000, stock=100)
    world.state.extra["d2_shops"] = [shop]
    world.state.extra["d2_order_id"], _ = completed_order_with_return(world, shop, None)
    world.state.extra["d2_total"] = int(
        d2.buyer_orders(world).get_order(world.state.extra["d2_order_id"])["order"]["totalAmount"]
    )


@when("the d2 buyer opens the return Modal")
def open_return_modal_d2(world: World) -> None:
    page = world.page
    button = page.get_by_role("button", name="Yêu cầu trả hàng", exact=True).first
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)
    button.click()
    expect(page.get_by_role("dialog")).to_be_visible(timeout=timeouts.DEFAULT)


@when(
    parsers.parse('the d2 buyer chooses the reason "{reason}" and submits while the server is slow')
)
def submit_return_slow(world: World, reason: str) -> None:
    page = world.page
    track_actions(world)
    slow_actions(world, 1.5, "**/account/orders/**")
    page.get_by_test_id("return-reason").select_option(reason)
    world.state.extra["d2_default_amount"] = page.get_by_test_id("return-amount").input_value()
    page.get_by_test_id("return-submit").click()


@then("the submit button is busy and the fields are disabled")
def return_pending(world: World) -> None:
    page = world.page
    expect(page.get_by_test_id("return-submit")).to_have_attribute(
        "aria-busy", "true", timeout=timeouts.DEFAULT
    )
    expect(page.get_by_test_id("return-reason")).to_be_disabled()
    expect(page.get_by_test_id("return-amount")).to_be_disabled()
    assert world.state.extra["d2_default_amount"] == str(world.state.extra["d2_total"])


@then("the Modal closes, a success toast shows and the returns tab shows a pending badge")
def return_submitted(world: World) -> None:
    page = world.page
    expect(toast(page, "Đã gửi yêu cầu trả hàng")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_role("dialog")).to_have_count(0, timeout=timeouts.DEFAULT)
    page.get_by_role("tab", name="Trả hàng / Hoàn tiền").click()
    badge = page.get_by_test_id("return-status")
    expect(badge).to_have_text("Chờ duyệt", timeout=timeouts.DEFAULT)
    assert TONE_CLASS["warning"] in (badge.get_attribute("class") or "")
    returns = d2.buyer_orders(world).post(
        "/platform.order.v1.OrderService/ListOrderReturns",
        {"orderId": world.state.extra["d2_order_id"]},
    )
    assert len(returns.get("returns", [])) == 1, returns


@when("the d2 buyer submits the return form without a reason")
def submit_no_reason(world: World) -> None:
    track_actions(world)
    world.page.get_by_test_id("return-submit").click()


@when("the d2 buyer submits the return form with an amount above the order total")
def submit_amount_too_high(world: World) -> None:
    page = world.page
    page.get_by_test_id("return-reason").select_option("defective")
    page.get_by_test_id("return-amount").fill(str(world.state.extra["d2_total"] + 1))
    page.get_by_test_id("return-submit").click()


@then("the reason form item shows an inline error, no action was sent and the Modal stays open")
def reason_error(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Vui lòng chọn lý do trả hàng.")).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    assert world.state.extra["d2_posts"] == []
    expect(page.get_by_role("dialog")).to_be_visible()


@then("the amount form item shows an inline error, no action was sent and the Modal stays open")
def amount_error(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Số tiền hoàn không được vượt quá tổng đơn hàng.")).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    assert world.state.extra["d2_posts"] == []
    expect(page.get_by_role("dialog")).to_be_visible()


@when("a return for the full amount is filed from another session")
def return_filed_elsewhere(world: World) -> None:
    d2.buyer_orders(world).create_return_request(
        world.state.extra["d2_order_id"], "other tab", world.state.extra["d2_total"]
    )


@when(parsers.parse('the d2 buyer chooses the reason "{reason}" and submits'))
def submit_return(world: World, reason: str) -> None:
    page = world.page
    page.get_by_test_id("return-reason").select_option(reason)
    page.get_by_test_id("return-submit").click()


@then("an error toast shows the error, the Modal stays open and the submit button is enabled")
def return_failed(world: World) -> None:
    page = world.page
    expect(
        page.get_by_role("alert")
        .filter(has_text=re.compile(r"\S{4,}"))
        .filter(has_not_text="Đóng")
        .first
    ).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_role("dialog")).to_be_visible()
    expect(page.get_by_test_id("return-submit")).to_be_enabled()


@given(parsers.parse("a d2 buyer has {count:d} orders from one shop with thumbnails"))
def buyer_n_orders_images(world: World, count: int) -> None:
    d2.seed_buyer(world)
    shop = d2.seed_shop(price=100_000, stock=100, image=True)
    world.state.extra["d2_shops"] = [shop]
    for _ in range(count):
        d2.place_order(world, shop)


# ── Layout, images and source ────────────────────────────────────────────
def server_html(world: World, path: str) -> str:
    resp = world.context.request.get(f"{base(world)}{path}")
    assert resp.ok, resp.status
    return resp.text()


@then("the first order card's thumbnail is eager and the thumbnails of the later cards are lazy")
def orders_lazy(world: World) -> None:
    shop = world.state.extra["d2_shops"][0]
    html = server_html(world, "/account/orders")
    tags = re.findall(rf'<img[^>]*alt="{re.escape(shop["title"])}"[^>]*>', html)
    assert len(tags) == 4, f"expected 4 thumbnails in the server HTML, got {len(tags)}"
    loading = [(re.search(r'loading="(\w+)"', t) or [None, ""])[1] for t in tags]
    assert loading == ["eager", "lazy", "lazy", "lazy"], loading


@then("every thumbnail keeps a fixed 1:1 box and shows the placeholder when the picture fails")
def thumbs_box(world: World) -> None:
    page = world.page
    page.wait_for_load_state("networkidle")
    thumb = page.get_by_test_id("order-card").first.locator("div.w-16").first
    expect(thumb).to_be_visible(timeout=timeouts.DEFAULT)
    box = thumb.bounding_box()
    assert abs(box["width"] - 64) < 1 and abs(box["height"] - 64) < 1, box
    # the seeded media key does not exist in storage: the picture fails and the fallback shows
    expect(thumb.get_by_role("img", name="Không có ảnh")).to_be_visible(timeout=timeouts.DEFAULT)
    after = thumb.bounding_box()
    assert abs(after["height"] - box["height"]) < 1, (box, after)


@then("the cumulative layout shift of the detail is 0")
def detail_cls(world: World) -> None:
    from tests.e2e.step_definitions.d2_quality_steps import cls, install_cls

    page = world.page
    install_cls(world)
    open_detail(world)
    page.wait_for_load_state("load")
    expect(page.get_by_test_id("order-timeline")).to_be_visible(timeout=timeouts.NAVIGATION)
    page.wait_for_timeout(1500)
    assert cls(page) == 0, f"layout shift {cls(page)} on the order detail"


@when("the d2 buyer opens the order list and the detail at 375 by 812")
def open_both_mobile(world: World) -> None:
    world.page.set_viewport_size({"width": 375, "height": 812})
    open_orders(world)
    world.page.wait_for_load_state("networkidle")


@then(
    "there is no horizontal page scroll, the tab bar scrolls inside its own container and the action buttons are full width"
)
def mobile_list(world: World) -> None:
    page = world.page
    overflow = page.evaluate(
        "document.documentElement.scrollWidth - document.documentElement.clientWidth"
    )
    assert overflow <= 0, overflow
    scroller = lst(world).tabs.locator(
        "xpath=descendant-or-self::*[contains(@class,'overflow-x')][1]"
    )
    expect(scroller).to_have_count(1)
    assert scroller.evaluate(
        "el => el.scrollWidth > el.clientWidth"
    ), "the tab bar does not scroll by itself"
    card = lst(world).order_cards.first
    card_w = card.bounding_box()["width"]
    for label in ("Xem chi tiết", "Mua lại"):
        target = (
            card.get_by_role("button", name=label).or_(card.get_by_role("link", name=label)).first
        )
        assert target.bounding_box()["width"] >= card_w - 40, (label, target.bounding_box(), card_w)


@then("the detail at 375px has no horizontal scroll, full width actions and a vertical stepper")
def mobile_detail(world: World) -> None:
    page = world.page
    open_detail(world)
    expect(page.get_by_role("heading", level=1)).to_be_visible(timeout=timeouts.DEFAULT)
    overflow = page.evaluate(
        "document.documentElement.scrollWidth - document.documentElement.clientWidth"
    )
    assert overflow <= 0, overflow
    header = page.locator("main header").first
    hw = header.bounding_box()["width"]
    reorder = page.get_by_role("button", name="Mua lại", exact=True).first
    assert reorder.bounding_box()["width"] >= hw - 60, (reorder.bounding_box(), hw)
    visible = page.locator('section[aria-label="Tiến trình đơn hàng"] ol:visible')
    assert visible.count() == 1, visible.count()
    tops = visible.locator("> li").evaluate_all(
        "els => els.map(e => Math.round(e.getBoundingClientRect().top))"
    )
    assert len(set(tops)) == len(tops), f"stepper steps share a row: {tops}"
    page.wait_for_timeout(500)
    page.get_by_role("button", name="Hủy đơn", exact=True).click()
    dialog = page.get_by_role("dialog")
    expect(dialog).to_be_visible(timeout=timeouts.DEFAULT)
    assert dialog.bounding_box()["width"] >= 375 - 40, dialog.bounding_box()


@when("the d2 buyer opens the order detail at 1280 by 800")
def open_detail_desktop(world: World) -> None:
    world.page.set_viewport_size({"width": 1280, "height": 800})
    open_detail(world)


@then(
    "the stepper is horizontal, the items show as table columns and the content is at most 960px wide"
)
def desktop_detail(world: World) -> None:
    page = world.page
    expect(page.get_by_role("heading", level=1)).to_be_visible(timeout=timeouts.DEFAULT)
    visible = page.locator('section[aria-label="Tiến trình đơn hàng"] ol:visible')
    assert visible.count() == 1, visible.count()
    tops = visible.locator("> li").evaluate_all(
        "els => els.map(e => Math.round(e.getBoundingClientRect().top))"
    )
    assert len(set(tops)) == 1, f"stepper is not horizontal: {tops}"
    table = page.get_by_role("table")
    for column in ("Sản phẩm", "Đơn giá", "Số lượng", "Thành tiền"):
        expect(table.get_by_role("columnheader", name=column)).to_be_visible()
    width = page.locator("main header").first.bounding_box()["width"]
    assert width <= 960, width


@then(
    "neither order page source contains a use client directive and the order data is in the server HTML"
)
def orders_pages_server(world: World) -> None:
    from tests.e2e.step_definitions.d2_quality_steps import FRONTEND, has_use_client

    for rel in ("(shop)/account/orders/(list)/page.tsx", "(shop)/account/orders/[id]/page.tsx"):
        path = FRONTEND / "src/app" / rel
        assert path.exists(), path
        assert not has_use_client(path), f"{rel} is a client component"
    oid = world.state.extra["d2_order_id"]
    assert oid[:8] in server_html(world, "/account/orders")
    detail = server_html(world, f"/account/orders/{oid}").replace("<!-- -->", "")
    assert f"Order #{oid[:8]}" in detail and "Nguyen Van A" in detail


def d2_open_return_form(page: Page) -> None:
    page.wait_for_timeout(500)
    page.get_by_role("button", name="Yêu cầu trả hàng", exact=True).first.click()
    expect(page.get_by_role("dialog")).to_be_visible(timeout=timeouts.DEFAULT)


@then("the existing timeline and return test ids are present in the rendered states")
def check_ids_present(world: World) -> None:
    from src.api.services import OrderService

    page = world.page
    ids = world.state.extra["d2_order_map"]
    shop = world.state.extra["d2_shops"][0]
    open_detail(world, ids["pending"])
    expect(page.get_by_test_id("order-timeline")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.get_by_test_id("timeline-saga")).to_be_visible()
    expect(page.get_by_test_id("timeline-saga-step").first).to_be_visible()
    OrderService(token=shop["token"]).create_shipment(
        ids["pending"], "SPX Express", f"D2{ids['pending'][:8]}"
    )
    open_detail(world, ids["pending"])
    expect(page.get_by_test_id("timeline-checkpoint").first).to_be_visible(timeout=timeouts.DEFAULT)
    open_detail(world, ids["completed"], "?tab=returns")
    expect(page.get_by_test_id("return-section")).to_be_visible(timeout=timeouts.DEFAULT)
    d2_open_return_form(page)
    for tid in ("return-reason", "return-amount", "return-submit"):
        expect(page.get_by_test_id(tid)).to_be_visible()
    page.get_by_test_id("return-reason").select_option("changed_mind")
    page.get_by_test_id("return-submit").click()
    expect(page.get_by_test_id("return-status")).to_be_visible(timeout=timeouts.NAVIGATION)
