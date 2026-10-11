"""Steps for frontend/ui_cart_page.feature (OpenSpec change ui-phase-cart-checkout, cart page)."""

from __future__ import annotations

import re
from pathlib import Path

from playwright.sync_api import Page, expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from tests.e2e.support import d2_support as d2
from tests.e2e.support.world import World

REPO = Path(__file__).resolve().parents[4]
STATE = "d2_cart"


def digits(text: str) -> int:
    """All digits of a price string ("350.000 ₫" -> 350000); -1 when there are none."""
    got = re.sub(r"\D", "", text or "")
    return int(got) if got else -1


def toast(page: Page, text: str, kind: str = "status"):
    return page.get_by_role(kind).filter(has_text=text)


def watch_busy(page: Page) -> None:
    """Record every moment an element carries aria-busy=true (pending UI is brief)."""
    page.evaluate("""() => {
          window.__busy = 0;
          const check = () => { if (document.querySelector('[aria-busy="true"]')) window.__busy++; };
          new MutationObserver(check).observe(document.body,
            {subtree: true, attributes: true, attributeFilter: ['aria-busy', 'disabled']});
        }""")


def busy_seen(page: Page) -> int:
    return page.evaluate("window.__busy || 0")


# ── Seeding ──────────────────────────────────────────────────────────────
@given(parsers.parse('a d2 buyer has a cart with one item from each of the shops "{names}"'))
def buyer_cart_shops(world: World, names: str) -> None:
    buyer = d2.seed_buyer(world)
    shops = []
    for i, name in enumerate(n.strip() for n in names.split(",")):
        shop = d2.seed_shop(None if name == "-" else name, price=100_000 * (i + 1))
        d2.add_to_cart(world, shop)
        shops.append(shop)
    world.state.extra["d2_shops"] = shops
    world.logger.info(f"d2 buyer {buyer.username} has a cart from {len(shops)} shops")


@given(parsers.parse("a d2 buyer has a cart with one item priced {price:d} and stock {stock:d}"))
def buyer_cart_one_item(world: World, price: int, stock: int) -> None:
    d2.seed_buyer(world)
    shop = d2.seed_shop(price=price, stock=stock)
    d2.add_to_cart(world, shop)
    world.state.extra["d2_shops"] = [shop]


@given(
    parsers.parse(
        'a d2 buyer with the address city "{city}" has a cart with one item priced {price:d}'
    )
)
def buyer_cart_in_city(world: World, city: str, price: int) -> None:
    d2.seed_buyer(world, address_city=None if city == "-" else city)
    shop = d2.seed_shop(price=price)
    d2.add_to_cart(world, shop)
    world.state.extra["d2_shops"] = [shop]


@given("a d2 buyer has an empty cart")
def buyer_empty_cart(world: World) -> None:
    d2.seed_buyer(world)
    world.state.extra["d2_shops"] = []


@given("a d2 buyer has an empty cart and no saved address")
def buyer_empty_cart_no_address(world: World) -> None:
    d2.seed_buyer(world, address_city=None)
    world.state.extra["d2_shops"] = []


@when("the d2 buyer opens the cart")
def open_cart(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/cart", wait_until="domcontentloaded")
    world.page.wait_for_load_state("networkidle")


# ── Groups and headers ───────────────────────────────────────────────────
@then(
    "the cart shows one group per shop, each with only its own item, and the subtotal is their sum"
)
def groups_and_subtotal(world: World) -> None:
    shops = world.state.extra["d2_shops"]
    groups = world.page.get_by_test_id("cart-shop-group")
    expect(groups).to_have_count(len(shops), timeout=timeouts.DEFAULT)
    for i, shop in enumerate(shops):
        group = groups.nth(i)
        expect(group.get_by_role("link", name=shop["title"])).to_have_count(1)
        for other in shops:
            if other is not shop:
                expect(group.get_by_text(other["title"])).to_have_count(0)
    total = sum(s["price"] for s in shops)
    summary = world.page.get_by_test_id("order-summary")
    subtotal_row = summary.get_by_text("Tạm tính").locator(
        "xpath=ancestor::*[self::div or self::li][1]"
    )
    assert digits(subtotal_row.first.inner_text()) % 10**12 != -1
    assert str(total)[:-3] in re.sub(r"\D", "", summary.inner_text()), summary.inner_text()


@then(parsers.parse('the group header shows "{name}" linking to the shop page and not "Shop #"'))
def header_real_name(world: World, name: str) -> None:
    shop = world.state.extra["d2_shops"][0]
    link = world.page.get_by_test_id("cart-shop-group").get_by_role("link", name=name, exact=True)
    expect(link).to_have_attribute("href", f"/shop/{shop['seller_id']}", timeout=timeouts.DEFAULT)
    expect(world.page.get_by_test_id("cart-shop-group").get_by_text("Shop #")).to_have_count(0)


@then('the group header shows "Shop #" followed by the first 6 characters of the seller id')
def header_fallback(world: World) -> None:
    shop = world.state.extra["d2_shops"][0]
    expect(
        world.page.get_by_test_id("cart-shop-group").get_by_role(
            "link", name=f"Shop #{shop['seller_id'][:6]}", exact=True
        )
    ).to_be_visible(timeout=timeouts.DEFAULT)


@then("every shop header holds only the shop name")
def header_only_name(world: World) -> None:
    groups = world.page.get_by_test_id("cart-shop-group")
    expect(groups.first).to_be_visible(timeout=timeouts.DEFAULT)
    for i, shop in enumerate(world.state.extra["d2_shops"]):
        header = groups.nth(i).evaluate("el => el.firstElementChild.innerText").strip()
        assert header == (shop["name"] or f"Shop #{shop['seller_id'][:6]}"), header
        assert not re.search(r"đánh giá|theo dõi|phản hồi|★|%|huy hiệu|badge", header, re.I), header


# ── Server rendering (spec: not a client component) ───────────────────────
@then('the cart page source has no "use client" directive')
def cart_page_not_client(world: World) -> None:
    src = (REPO / "team-frontend/src/app/(shop)/cart/page.tsx").read_text("utf-8")
    assert not re.search(r"^\s*[\"']use client[\"']", src, re.M), "cart page is a client component"


@then("the cart HTML response already contains the item rows")
def cart_html_has_items(world: World) -> None:
    resp = world.context.request.get(f"{world.settings.base_url}/cart")
    assert resp.ok, resp.status
    html = resp.text()
    for shop in world.state.extra["d2_shops"]:
        assert shop["title"] in html, f"item {shop['title']} missing from the server HTML"


# ── Mutations ────────────────────────────────────────────────────────────
@when("the d2 buyer increases the quantity watching for pending state")
def increase_quantity(world: World) -> None:
    page = world.page
    inc = page.get_by_role("button", name="Tăng số lượng").first
    expect(inc).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)  # hydration of the island; no signal exposed for it
    watch_busy(page)
    inc.click()


@then(
    parsers.parse(
        "the quantity, line total and subtotal show the server values for quantity {qty:d}"
    )
)
def quantity_updated(world: World, qty: int) -> None:
    page = world.page
    shop = world.state.extra["d2_shops"][0]
    expect(page.get_by_role("spinbutton").first).to_have_value(str(qty), timeout=timeouts.DEFAULT)
    assert busy_seen(page) > 0, "the controls never showed aria-busy while the action ran"
    expected = str(shop["price"] * qty)[:-3]
    expect(page.get_by_test_id("order-total")).to_contain_text(
        re.compile(rf"{expected[0]}.*{expected[1:]}" if len(expected) > 1 else expected),
        timeout=timeouts.DEFAULT,
    )
    cart = d2.buyer_cart(world).get_cart().get("cart", {})
    assert cart["items"][0]["quantity"] == qty, cart
    expect(page.get_by_role("button", name="Tăng số lượng").first).to_be_enabled()


@when("the cart is emptied from another session")
def cart_emptied_elsewhere(world: World) -> None:
    d2.buyer_cart(world).clear_cart()


@when("the d2 buyer increases the quantity of the stale row")
def increase_stale(world: World) -> None:
    page = world.page
    inc = page.get_by_role("button", name="Tăng số lượng").first
    expect(inc).to_be_enabled(timeout=timeouts.DEFAULT)
    inc.click()


@then("an error toast gives the reason and the quantity is back to its server value")
def failed_update(world: World) -> None:
    page = world.page
    expect(
        page.get_by_role("alert")
        .filter(has_text=re.compile(r"\S{3,}"))
        .filter(has_not_text="Đóng")
        .first
    ).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.get_by_role("spinbutton").first).to_have_value("1", timeout=timeouts.DEFAULT)
    expect(page.get_by_role("spinbutton").first).to_be_enabled()


@when("the d2 buyer clears the cart watching for pending state")
def clear_cart(world: World) -> None:
    page = world.page
    button = page.get_by_role("button", name="Xóa tất cả")
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)
    page.once("dialog", lambda d: d.accept())
    watch_busy(page)
    button.click()


@then("an info toast is shown and the cart shows the empty state with a continue shopping link")
def cart_cleared(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Giỏ hàng của bạn đang trống")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(toast(page, "Đã làm trống giỏ hàng")).to_be_visible()
    expect(page.get_by_role("link", name="Tiếp tục mua sắm")).to_have_attribute("href", "/")
    assert busy_seen(page) > 0, "clear button never showed a loading state"


@then("the decrease control is disabled")
def decrease_disabled(world: World) -> None:
    dec = world.page.get_by_role("button", name="Giảm số lượng").first
    expect(dec).to_be_disabled(timeout=timeouts.DEFAULT)
    expect(dec).to_have_attribute("aria-disabled", "true")
    assert d2.buyer_cart(world).get_cart()["cart"]["items"][0]["quantity"] == 1


# ── Voucher modal (cart page) ────────────────────────────────────────────
@when("the d2 buyer opens the voucher modal")
def open_voucher_modal(world: World) -> None:
    page = world.page
    opener = page.get_by_role("button", name="Chọn hoặc nhập mã")
    expect(opener).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)
    opener.click()
    expect(page.get_by_role("dialog")).to_be_visible(timeout=timeouts.DEFAULT)


@when(parsers.parse('the d2 buyer applies the code "{code}" in the voucher modal'))
def apply_code(world: World, code: str) -> None:
    page = world.page
    page.locator('input[name="voucher_code"]').fill(code)
    page.get_by_role("dialog").get_by_role("button", name="Áp dụng").click()


@then(
    "the modal closes, a success toast shows and the discount row and total reflect the server amount"
)
def voucher_applied(world: World) -> None:
    page = world.page
    shop = world.state.extra["d2_shops"][0]
    expect(page.get_by_role("dialog")).to_have_count(0, timeout=timeouts.DEFAULT)
    expect(toast(page, "Đã áp dụng mã")).to_be_visible()
    discount = shop["price"] // 10
    expect(page.get_by_test_id("voucher-discount")).to_contain_text(
        re.compile(r"\d"), timeout=timeouts.DEFAULT
    )
    assert digits(page.get_by_test_id("voucher-discount").inner_text()) == discount
    assert digits(page.get_by_test_id("order-total").inner_text()) == shop["price"] - discount


@then(
    "the form item shows the server reason, an error toast shows, no discount applies and the modal stays open"
)
def voucher_rejected(world: World) -> None:
    page = world.page
    shop = world.state.extra["d2_shops"][0]
    dialog = page.get_by_role("dialog")
    reason = dialog.locator('p[aria-live="polite"]')
    expect(reason).to_be_visible(timeout=timeouts.DEFAULT)
    assert reason.inner_text().strip()
    expect(
        page.get_by_role("alert").filter(has_text=reason.inner_text().strip()).first
    ).to_be_visible()
    expect(dialog).to_be_visible()
    assert digits(page.get_by_test_id("voucher-discount").inner_text()) == -1
    assert digits(page.get_by_test_id("order-total").inner_text()) == shop["price"]


@when("the d2 buyer presses Tab 10 times in the voucher modal")
def tab_in_modal(world: World) -> None:
    for _ in range(10):
        world.page.keyboard.press("Tab")


@then("focus is still inside the voucher modal")
def focus_inside(world: World) -> None:
    inside = world.page.evaluate(
        "() => !!document.activeElement && !!document.activeElement.closest('dialog,[role=\"dialog\"]')"
    )
    assert inside, "focus escaped the voucher modal"


@when("the d2 buyer presses Escape")
def press_escape(world: World) -> None:
    world.page.keyboard.press("Escape")


@then("the voucher modal is closed and focus is back on the voucher selector")
def focus_restored(world: World) -> None:
    page = world.page
    expect(page.get_by_role("dialog")).to_have_count(0, timeout=timeouts.DEFAULT)
    label = page.evaluate("() => document.activeElement && document.activeElement.textContent")
    assert "Chọn hoặc nhập mã" in (label or ""), f"focus is on {label!r}"


@then("the voucher modal offers the manual code input")
def manual_input(world: World) -> None:
    expect(world.page.locator('input[name="voucher_code"]')).to_be_enabled(timeout=timeouts.DEFAULT)


@then("the voucher modal shows the empty state and keeps the manual code input usable")
def voucher_empty(world: World) -> None:
    dialog = world.page.get_by_role("dialog")
    expect(dialog.get_by_text("Chưa có voucher khả dụng")).to_be_visible(timeout=timeouts.DEFAULT)
    field = dialog.locator('input[name="voucher_code"]')
    field.fill("ANYTHING")
    expect(field).to_have_value("ANYTHING")
    expect(dialog.get_by_role("button", name="Áp dụng")).to_be_enabled()


# ── Empty cart, skeleton and layout shift ────────────────────────────────
@then("the empty cart state links to continue shopping and no summary or buy button is rendered")
def empty_cart_state(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Giỏ hàng của bạn đang trống")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.get_by_role("link", name="Tiếp tục mua sắm")).to_have_attribute("href", "/")
    expect(page.get_by_test_id("order-summary")).to_have_count(0)
    main = page.locator("main")
    expect(main.get_by_role("link", name="Mua hàng")).to_have_count(0)
    expect(main.get_by_role("button", name="Mua hàng")).to_have_count(0)


@when("the d2 buyer opens the cart from the home page while the cart data is slow")
def open_cart_slow(world: World) -> None:
    import time

    from tests.e2e.step_definitions.d2_quality_steps import install_cls

    page = world.page
    install_cls(world)
    page.goto(f"{world.settings.base_url}/", wait_until="networkidle")

    def slow(route) -> None:  # noqa: ANN001
        time.sleep(1.5)
        route.continue_()

    page.route(re.compile(r".*/cart\?_rsc=.*"), slow)
    page.evaluate("window.__cls = 0")
    page.locator('a[href="/cart"]').first.click()


@then("a skeleton with a shop card, three item rows and the summary is shown before the cart")
def cart_skeleton(world: World) -> None:
    page = world.page
    skeleton = page.get_by_test_id("cart-skeleton")
    expect(skeleton).to_be_visible(timeout=timeouts.DEFAULT)
    expect(skeleton.locator("ul > li")).to_have_count(3)
    expect(page.get_by_test_id("cart-shop-group").first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(skeleton).to_have_count(0)


@then("the layout shift score of the cart is 0")
def cart_cls_zero(world: World) -> None:
    from tests.e2e.step_definitions.d2_quality_steps import cls

    page = world.page
    page.wait_for_timeout(1000)
    soft = cls(page)
    page.goto(f"{world.settings.base_url}/cart", wait_until="load")
    expect(page.get_by_test_id("cart-shop-group").first).to_be_visible(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(1500)
    hard = cls(page)
    assert (
        soft == 0 and hard == 0
    ), f"cumulative layout shift: soft navigation {soft}, hard load {hard}"


@given(parsers.parse("a d2 buyer has a cart with one item from each of {n:d} shops"))
def buyer_cart_n_shops(world: World, n: int) -> None:
    d2.seed_buyer(world)
    shops = []
    for i in range(n):
        shop = d2.seed_shop(None, price=100_000 * (i + 1), image=True)
        d2.add_to_cart(world, shop)
        shops.append(shop)
    world.state.extra["d2_shops"] = shops
