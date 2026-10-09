"""Steps for frontend/ui_checkout_flow.feature and frontend/ui_checkout_order.feature
(OpenSpec change ui-phase-cart-checkout, checkout wizard).

Builds on d2_cart_steps (seeding) and cart_checkout_steps (open the wizard, go to a step).
Slow-server states (pending buttons) are observed by delaying the Next server-action POST
in the browser; the order itself is created by the real stack.
"""

from __future__ import annotations

import re
import time

from playwright.sync_api import Page, expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from tests.e2e.flows import set_checkout_flag
from tests.e2e.step_definitions.d2_cart_steps import digits, toast
from tests.e2e.support import d2_support as d2
from tests.e2e.support.world import World

_MOBILE = {"width": 375, "height": 812}


def summary_text(page: Page) -> str:
    return page.get_by_test_id("order-summary").inner_text()


def summary_rows(page: Page, locator=None) -> dict[str, int]:
    """Subtotal, discount, shipping and total (VND) read from the summary rows."""
    text = (locator or page.get_by_test_id("order-summary")).inner_text()
    flat = text.replace("\n", " | ")
    out: dict[str, int] = {}
    for key, label in (
        ("subtotal", r"Tạm tính[^|]*\|"),
        ("discount", r"Giảm giá \|(?: [A-Z0-9-]+ \|)?"),
        ("shipping", r"Phí vận chuyển \|"),
        ("total", r"Tổng cộng \|(?: ₫ \|)?"),
    ):
        m = re.search(label + r"\s*([^|]*)", flat)
        out[key] = digits(m.group(1)) if m else -1
    if re.search(r"Freeship", flat):
        out["shipping"] = 0
    return out


def top(locator) -> float:  # noqa: ANN001
    """Top of an element in document coordinates (clicking scrolls the viewport)."""
    return locator.evaluate("el => el.getBoundingClientRect().top + window.scrollY")


def track_actions(world: World) -> list:
    """Collect every Next server-action POST the page sends from now on."""
    posts: list = world.state.extra.setdefault("d2_posts", [])
    if not world.state.extra.get("d2_posts_hooked"):
        world.state.extra["d2_posts_hooked"] = True

        def on_request(req) -> None:  # noqa: ANN001
            if req.method == "POST" and "next-action" in req.headers:
                posts.append(req.url)

        world.page.on("request", on_request)
    return posts


def slow_actions(world: World, seconds: float) -> None:
    def handler(route) -> None:  # noqa: ANN001
        req = route.request
        if req.method == "POST" and "next-action" in req.headers:
            time.sleep(seconds)
        route.continue_()

    world.page.route("**/checkout**", handler)


# ── Shell, URL and redirects ─────────────────────────────────────────────
@then("the checkout shell shows the logo link, the stepper and the secure footer only")
def shell_chrome(world: World) -> None:
    page = world.page
    shell = page.get_by_test_id("checkout-shell")
    expect(shell.get_by_role("link", name="Marketplace")).to_have_attribute("href", "/")
    expect(shell.get_by_role("navigation", name="Progress")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(shell.get_by_text("Thanh toán an toàn · Môi trường demo")).to_be_visible()
    expect(page.get_by_placeholder("Tìm kiếm sản phẩm")).to_have_count(0)
    # the stepper is the only navigation landmark: no consumer or bottom navigation
    expect(page.get_by_role("navigation")).to_have_count(1)


@when("the buyer uses the browser back button")
def browser_back(world: World) -> None:
    world.state.extra["d2_addr_url"] = re.search(r"addr=([^&]+)", world.page.url).group(1)
    world.page.go_back(wait_until="domcontentloaded")


@then("the shipping step is shown with the selections preserved")
def back_to_shipping(world: World) -> None:
    page = world.page
    page.wait_for_url(re.compile(r".*[?&]step=shipping\b.*"), timeout=timeouts.NAVIGATION)
    assert f"addr={world.state.extra['d2_addr_url']}" in page.url, page.url
    expect(
        page.get_by_role("navigation", name="Progress").locator('li[aria-current="step"]')
    ).to_contain_text("Vận chuyển", timeout=timeouts.DEFAULT)


@when(parsers.parse('the buyer opens the checkout URL "{path}" directly'))
def open_checkout_url(world: World, path: str) -> None:
    world.page.goto(f"{world.settings.base_url}{path}", wait_until="domcontentloaded")


@then(parsers.parse('the browser is redirected to step "{step}"'))
def redirected_to_step(world: World, step: str) -> None:
    world.page.wait_for_url(re.compile(rf".*[?&]step={step}\b.*"), timeout=timeouts.NAVIGATION)


@when("the visitor signs out and opens the checkout page")
def signed_out_open_checkout(world: World) -> None:
    world.context.clear_cookies()
    world.page.goto(f"{world.settings.base_url}/checkout", wait_until="domcontentloaded")


@when("the buyer opens the checkout page directly")
def open_checkout_direct(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/checkout", wait_until="domcontentloaded")


@then(parsers.parse('the browser lands on "{path}"'))
def lands_on(world: World, path: str) -> None:
    world.page.wait_for_url(
        re.compile(rf".*{re.escape(path)}(\?.*)?$"), timeout=timeouts.NAVIGATION
    )


# ── Address step ─────────────────────────────────────────────────────────
@then("the address step shows the empty state and a disabled continue action")
def no_address_blocks(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Bạn chưa có địa chỉ nhận hàng")).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    expect(page.get_by_role("button", name="Thêm địa chỉ", exact=True)).to_be_visible()
    cont = page.get_by_role("button", name="Tiếp tục").first
    expect(cont).to_be_disabled()
    expect(cont).to_have_attribute("aria-disabled", "true")
    expect(page.get_by_role("link", name="Tiếp tục")).to_have_count(0)


@when(parsers.parse('the buyer adds the address for "{recipient}" from the empty state'))
def add_address(world: World, recipient: str) -> None:
    page = world.page
    page.wait_for_timeout(500)  # island hydration
    page.get_by_role("button", name="Thêm địa chỉ", exact=True).click()
    dialog = page.get_by_role("dialog")
    expect(dialog).to_be_visible(timeout=timeouts.DEFAULT)
    for name, value in (
        ("recipientName", recipient),
        ("phone", "0912345678"),
        ("street", "29 Lieu Giai"),
        ("ward", "Phường Liễu Giai"),
        ("district", "Quận Ba Đình"),
        ("city", "Hà Nội"),
    ):
        dialog.locator(f'[name="{name}"]').fill(value)
    dialog.get_by_role("button", name="Thêm mới").click()
    world.state.extra["d2_recipient"] = recipient


@then("a success toast shows and the new address is listed and selected")
def address_added(world: World) -> None:
    page = world.page
    recipient = world.state.extra["d2_recipient"]
    expect(toast(page, "Đã thêm địa chỉ mới")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_text(recipient).first).to_be_visible()
    addresses = world.service_factory.address.list_addresses()["addresses"]
    created = next(a["id"] for a in addresses if a["recipientName"] == recipient)
    page.wait_for_url(re.compile(rf".*[?&]addr={re.escape(created)}\b"), timeout=timeouts.DEFAULT)
    expect(page.get_by_role("link", name="Tiếp tục").first).to_be_visible()


# ── Shipping and payment steps ───────────────────────────────────────────
@then("the shipping step shows the Freeship tag and the summary shipping row is free")
def freeship(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Giao hàng tiêu chuẩn").first).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.get_by_text("Freeship")).to_have_count(2)  # the step card and the summary row
    assert summary_rows(page)["shipping"] == 0, summary_text(page)


@then("the payment cards are one per row, fit the viewport and are at least 44px tall")
def payment_cards_mobile(world: World) -> None:
    page = world.page
    cards = page.locator("[data-selected]")
    expect(cards).to_have_count(4, timeout=timeouts.DEFAULT)
    tops = set()
    for i in range(4):
        box = cards.nth(i).bounding_box()
        assert box["x"] >= 0 and box["x"] + box["width"] <= 375.5, box
        assert box["height"] >= 44, box
        tops.add(round(box["y"]))
    assert len(tops) == 4, f"cards share a row: {sorted(tops)}"
    overflow = page.evaluate(
        "document.documentElement.scrollWidth - document.documentElement.clientWidth"
    )
    assert overflow <= 0, overflow


@when("the buyer sets the viewport to 375 by 812")
def mobile_viewport(world: World) -> None:
    world.page.set_viewport_size(_MOBILE)


@when("the buyer sets the viewport to 1280 by 720")
def desktop_viewport(world: World) -> None:
    world.page.set_viewport_size({"width": 1280, "height": 720})


# ── Order summary ────────────────────────────────────────────────────────
@when(parsers.parse('the buyer applies the code "{code}" on the payment step'))
def apply_on_payment(world: World, code: str) -> None:
    page = world.page
    world.state.extra["d2_cta_top"] = top(page.get_by_role("link", name="Tiếp tục").first)
    world.state.extra["d2_total_top"] = top(page.get_by_test_id("order-total"))
    opener = page.get_by_role("button", name="Chọn hoặc nhập mã")
    expect(opener).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)
    opener.click()
    page.locator('input[name="voucher_code"]').fill(code)
    page.get_by_role("dialog").get_by_role("button", name="Áp dụng").click()
    expect(page.get_by_role("dialog")).to_have_count(0, timeout=timeouts.DEFAULT)
    expect(page.get_by_test_id("voucher-discount")).to_contain_text(
        re.compile(r"\d"), timeout=timeouts.DEFAULT
    )


@then(
    parsers.parse(
        "the summary shows subtotal {sub:d}, discount {disc:d}, shipping {ship:d} and total {total:d}"
    )
)
def summary_numbers(world: World, sub: int, disc: int, ship: int, total: int) -> None:
    rows = summary_rows(world.page)
    assert rows == {"subtotal": sub, "discount": disc, "shipping": ship, "total": total}, (
        rows,
        summary_text(world.page),
    )


@then("the continue action and the total row did not move")
def nothing_moved(world: World) -> None:
    page = world.page
    assert (
        abs(top(page.get_by_role("link", name="Tiếp tục").first) - world.state.extra["d2_cta_top"])
        < 1
    )
    moved = top(page.get_by_test_id("order-total")) - world.state.extra["d2_total_top"]
    assert abs(moved) < 1, f"the total row moved by {moved}px"


@then("a sticky bottom bar shows the total and the primary action")
def mobile_bar(world: World) -> None:
    bar = world.page.get_by_test_id("mobile-summary-bar")
    expect(bar).to_be_visible(timeout=timeouts.DEFAULT)
    expect(bar.get_by_role("button", name="Chi tiết")).to_be_visible()
    assert digits(bar.inner_text()) > 0, bar.inner_text()
    expect(
        bar.get_by_role("link", name="Tiếp tục").or_(bar.get_by_role("button", name="Đặt hàng"))
    ).to_be_visible()
    box = bar.bounding_box()
    assert abs(box["y"] + box["height"] - 812) < 2, box


@when('the buyer taps "Chi tiết" in the bottom bar')
def tap_details(world: World) -> None:
    page = world.page
    page.wait_for_timeout(500)
    page.get_by_test_id("mobile-summary-bar").get_by_role("button", name="Chi tiết").click()


@then("a drawer shows the full breakdown without horizontal scroll")
def drawer_breakdown(world: World) -> None:
    page = world.page
    drawer = page.get_by_role("dialog")
    expect(drawer).to_be_visible(timeout=timeouts.DEFAULT)
    rows = summary_rows(page, drawer)
    assert rows["subtotal"] > 0 and rows["total"] > 0, drawer.inner_text()
    overflow = page.evaluate(
        "document.documentElement.scrollWidth - document.documentElement.clientWidth"
    )
    assert overflow <= 0, overflow


@when("the buyer scrolls the page down")
def scroll_down(world: World) -> None:
    world.page.evaluate("window.scrollTo(0, document.body.scrollHeight)")


@then("the order summary is sticky and still visible in the right column")
def summary_sticky(world: World) -> None:
    page = world.page
    summary = page.get_by_test_id("order-summary")
    expect(summary).to_be_visible(timeout=timeouts.DEFAULT)
    assert summary.evaluate("el => getComputedStyle(el).position") == "sticky"
    box = summary.bounding_box()
    assert 0 <= box["y"] < 720 and box["x"] > 640, box
    main = page.get_by_test_id("checkout-shell").bounding_box()
    assert box["x"] > main["x"] + main["width"] / 2, (box, main)


# ── Analytics ────────────────────────────────────────────────────────────
@when("the buyer moves through the four checkout steps")
def through_steps(world: World) -> None:
    from src.constants import PageName
    from src.pages import CheckoutPage

    world.navigate_to(PageName.CHECKOUT)
    checkout: CheckoutPage = world.get_page(PageName.CHECKOUT)  # type: ignore[assignment]
    expect(checkout.stepper).to_be_visible(timeout=timeouts.NAVIGATION)
    checkout.continue_to("confirm")


def datalayer(world: World, event: str) -> list[dict]:
    return world.page.evaluate(
        "(name) => (window.dataLayer || []).filter((e) => e.event === name)", event
    )


@then(parsers.parse('exactly one "{event}" event was pushed with the cart items and value'))
def one_event_with_items(world: World, event: str) -> None:
    events = datalayer(world, event)
    assert len(events) == 1, events
    blob = str(events[0])
    shop = world.state.extra["d2_shops"][0]
    assert shop["title"] in blob or shop["listing_id"] in blob, blob
    assert str(shop["price"]) in blob, blob


@when("the buyer places the order with a double click")
def place_with_double_click(world: World) -> None:
    page = world.page
    track_actions(world)
    button = page.get_by_role("button", name="Đặt hàng").first
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)
    button.dblclick()
    page.wait_for_url(
        re.compile(r".*/(account/orders|checkout/pay).*"), timeout=timeouts.NAVIGATION
    )


@then("exactly one purchase event carries the id of the created order")
def one_purchase(world: World) -> None:
    deadline = time.monotonic() + 10
    orders: list = []
    while time.monotonic() < deadline and not orders:
        orders = d2.buyer_orders(world).list_buyer_orders().get("orders", [])
        time.sleep(0.5)
    assert len(orders) == 1, orders
    page = world.page
    page.wait_for_load_state("domcontentloaded")
    # the order list page is a new document: the event lives in the checkout page's
    # dataLayer, which survives soft navigation (router.push) within the same document.
    events = datalayer(world, "purchase")
    assert len(events) == 1, events
    assert orders[0]["id"] in str(events[0]), (events[0], orders[0]["id"])


@when(
    parsers.parse('the buyer previews the codes "{first}" and then "{second}" on the payment step')
)
def preview_two_codes(world: World, first: str, second: str) -> None:
    page = world.page
    page.wait_for_timeout(500)
    for i, code in enumerate((first, second)):
        page.get_by_role("button", name=re.compile("Chọn hoặc nhập mã|Đổi mã")).click()
        page.locator('input[name="voucher_code"]').fill(code)
        page.get_by_role("dialog").get_by_role("button", name="Áp dụng").click()
        if i == 0:
            expect(page.get_by_role("dialog")).to_have_count(0, timeout=timeouts.DEFAULT)
        else:
            expect(page.get_by_role("dialog").locator('p[aria-live="polite"]')).to_be_visible(
                timeout=timeouts.DEFAULT
            )


@then('two "apply_promotion" events were pushed with valid "true" and then "false"')
def two_promotions(world: World) -> None:
    events = datalayer(world, "apply_promotion")
    assert len(events) == 2, events
    assert "'valid': 'true'" in str(events[0]) or '"valid": "true"' in str(events[0]), events[0]
    assert "'valid': 'false'" in str(events[1]) or '"valid": "false"' in str(events[1]), events[1]


# ── Placing an order ─────────────────────────────────────────────────────
@when("the buyer places the order while the server is slow")
def place_slow(world: World) -> None:
    page = world.page
    track_actions(world)
    slow_actions(world, 2.0)
    button = page.get_by_role("button", name="Đặt hàng").first
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)
    world.state.extra["d2_width"] = button.bounding_box()["width"]
    button.click()


@then(
    "the place order button is disabled, busy and keeps its width, and back and stepper navigation are inert"
)
def pending_button(world: World) -> None:
    page = world.page
    button = page.get_by_role("button", name="Đặt hàng").first
    expect(button).to_have_attribute("aria-busy", "true", timeout=timeouts.DEFAULT)
    expect(button).to_have_attribute("aria-disabled", "true")
    expect(button).to_be_disabled()
    assert abs(button.bounding_box()["width"] - world.state.extra["d2_width"]) < 1
    expect(page.get_by_role("link", name="Quay lại", exact=True)).to_have_count(0)
    expect(page.locator('span[aria-disabled="true"]', has_text="Quay lại")).to_have_count(1)
    nav = page.get_by_role("navigation", name="Progress")
    expect(nav.get_by_role("link")).to_have_count(0)
    expect(nav.locator('[aria-disabled="true"]')).not_to_have_count(0)


@when("the buyer presses Enter in the confirm form while the order is pending")
def press_enter_pending(world: World) -> None:
    page = world.page
    button = page.get_by_role("button", name="Đặt hàng").first
    expect(button).to_have_attribute("aria-busy", "true", timeout=timeouts.DEFAULT)
    page.keyboard.press("Enter")
    page.evaluate("document.getElementById('place-order-form').requestSubmit()")


@then("only one order action was sent and exactly one order exists")
def one_action_one_order(world: World) -> None:
    page = world.page
    page.wait_for_url(
        re.compile(r".*/(account/orders|checkout/pay).*"), timeout=timeouts.NAVIGATION
    )
    posts = world.state.extra["d2_posts"]
    assert len(posts) == 1, posts
    deadline = time.monotonic() + 10
    orders: list = []
    while time.monotonic() < deadline and not orders:
        orders = d2.buyer_orders(world).list_buyer_orders().get("orders", [])
        time.sleep(0.5)
    assert len(orders) == 1, orders


@when("the buyer places the order while the next page is slow")
def place_with_slow_navigation(world: World) -> None:
    page = world.page
    track_actions(world)

    def handler(route) -> None:  # noqa: ANN001
        time.sleep(2.0)
        route.continue_()

    page.route(re.compile(r".*/account/orders.*"), handler)
    page.route(re.compile(r".*/checkout/pay/.*"), handler)
    button = page.get_by_role("button", name="Đặt hàng").first
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)
    button.click()


@then("once the order is created the button stays disabled and no second order action can be sent")
def locked_after_success(world: World) -> None:
    page = world.page
    deadline = time.monotonic() + 10
    orders: list = []
    while time.monotonic() < deadline and not orders:
        orders = d2.buyer_orders(world).list_buyer_orders().get("orders", [])
        time.sleep(0.3)
    assert len(orders) == 1, orders
    assert "/checkout" in page.url, f"navigated too early: {page.url}"
    button = page.get_by_role("button", name="Đặt hàng").first
    expect(button).to_be_disabled()
    page.evaluate("document.getElementById('place-order-form').requestSubmit()")
    page.wait_for_url(
        re.compile(r".*/(account/orders|checkout/pay).*"), timeout=timeouts.NAVIGATION
    )
    assert len(world.state.extra["d2_posts"]) == 1, world.state.extra["d2_posts"]
    assert len(d2.buyer_orders(world).list_buyer_orders()["orders"]) == 1


@then("the place order button is enabled again with an error alert and an error toast")
def reenabled_after_failure(world: World) -> None:
    page = world.page
    expect(page.get_by_test_id("saga-alert-slot").get_by_role("alert")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(
        page.get_by_role("alert")
        .filter(has_text=re.compile(r"\S{4,}"))
        .filter(has_not_text="Không thể đặt hàng")
        .first
    ).to_be_visible()
    expect(page.get_by_role("button", name="Đặt hàng").first).to_be_enabled()


# ── Kill-switch (destructive: flips the stack-wide flag) ─────────────────
@when('the "checkout-enabled" flag is turned OFF for the d2 scenario')
def flag_off(world: World) -> None:
    world.add_cleanup(lambda: set_checkout_flag(True))
    set_checkout_flag(False)


@then("the checkout page shows the unavailable Result with a back to cart button and no wizard")
def killswitch_result(world: World) -> None:
    page = world.page
    expect(page.get_by_role("heading", name="Thanh toán tạm thời không khả dụng")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(page.get_by_role("link", name="Quay lại giỏ hàng")).to_have_attribute("href", "/cart")
    expect(page.get_by_role("navigation", name="Progress")).to_have_count(0)


@then("the buy button is disabled with aria-disabled and an alert explains checkout is unavailable")
def killswitch_cart(world: World) -> None:
    page = world.page
    button = page.get_by_role("button", name="Mua hàng").first
    expect(button).to_be_disabled(timeout=timeouts.NAVIGATION)
    expect(button).to_have_attribute("aria-disabled", "true")
    expect(page.get_by_text("Thanh toán tạm thời không khả dụng").first).to_be_visible()


@given("another d2 buyer buys all the remaining stock of the listing")
def other_buyer_exhausts_stock(world: World) -> None:
    from src.api.services import AddressService, CartService, OrderService

    shop = world.state.extra["d2_shops"][0]
    _, token = d2.register("buyer", "d2_other")
    AddressService(token=token).create_address(
        recipient_name="Le Van C",
        phone="0911111111",
        street="1 Tran Phu",
        city="Hà Nội",
        is_default=True,
    )
    CartService(token=token).add_to_cart(shop["listing_id"], shop["stock"])
    OrderService(token=token).create_order({"paymentMethod": "PAYMENT_METHOD_COD"})
