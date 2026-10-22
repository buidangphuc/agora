"""Steps for frontend/checkout_coherence_ui.feature (change frontend-checkout-coherence).

Black box: carts, vouchers and addresses are seeded through the gateway as the real buyer and
sellers; the figures a page must show are the real PreviewCheckout answer, the placed orders and
payments are read back from the gateway (and the payment table, read-only).
"""

from __future__ import annotations

import re
import time
from typing import Any
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import PageName, timeouts
from src.pages import CheckoutPage
from tests.e2e.step_definitions.d2_cart_steps import digits
from tests.e2e.support import address_support as addr
from tests.e2e.support import advertised_price_ui_support as adv
from tests.e2e.support import checkout_coherence_support as cc
from tests.e2e.support import order_lifecycle_ui_support as olc
from tests.e2e.support.world import World

PAY_URL = re.compile(r".*/checkout/pay/([^/?#]+)")
ORDERS_URL = re.compile(r".*/account/orders")


def checkout_page(world: World) -> CheckoutPage:
    return world.get_page(PageName.CHECKOUT)  # type: ignore[return-value]


def goto_with_query(world: World, extra: list[tuple[str, str]]) -> None:
    parts = urlsplit(world.page.url)
    query = [(k, v) for k, v in parse_qsl(parts.query) if k not in {"sv", "pv", "voucher"}]
    url = urlunsplit(parts._replace(query=urlencode(query + extra, safe=":,")))
    world.page.goto(url, wait_until="domcontentloaded")
    expect(checkout_page(world).stepper).to_be_visible(timeout=timeouts.NAVIGATION)


def summary(world: World) -> dict[str, int]:
    from tests.e2e.step_definitions.d2_checkout_steps import summary_rows

    expect(world.page.get_by_test_id("order-summary")).to_be_visible(timeout=timeouts.NAVIGATION)
    return summary_rows(world.page)


def shown_discount(world: World) -> int:
    el = world.page.get_by_test_id("voucher-discount")
    if not el.count():
        return 0
    return max(digits(el.first.inner_text()), 0)


def open_confirm(world: World) -> None:
    world.navigate_to(PageName.CHECKOUT)
    expect(checkout_page(world).stepper).to_be_visible(timeout=timeouts.NAVIGATION)


# ── Given ────────────────────────────────────────────────────────────────
@given(parsers.parse('a cc buyer in "{city}" has one item of each price {price:d} from two shops'))
def buyer_two_shops(world: World, city: str, price: int) -> None:
    cc.seed_cart(world, city, [price, price])


@given(parsers.parse('a cc buyer in "{city}" has one item of each price {price:d} from one shop'))
def buyer_one_shop(world: World, city: str, price: int) -> None:
    cc.seed_cart(world, city, [price])


@given("shop A has a cc shop voucher and shop B has a cc shop voucher")
def shop_vouchers(world: World) -> None:
    c = cc.ctx(world)
    a, b = sorted(c["shops"], key=lambda s: s["seller_id"])
    c["shop_a"], c["shop_b"] = a, b
    c["code_a"] = cc.seed_shop_voucher(a, "CCA")
    c["code_b"] = cc.seed_shop_voucher(b, "CCB")


@given("a cc platform voucher exists")
def platform_voucher(world: World) -> None:
    cc.ctx(world)["code_p"] = cc.seed_platform_voucher()


@given('a cc buyer in "Hà Nội" has an online order with no payment transaction')
def buyer_unpaid_order(world: World) -> None:
    cc.seed_cart(world, "Hà Nội", [])
    from tests.e2e.support import d2_support as d2

    shop = d2.seed_shop(price=100_000, stock=50)
    c = cc.ctx(world)
    c["shops"] = [shop]
    c["order_id"] = olc.place(c["buyer"].token, shop["listing_id"], olc.MOMO)
    assert cc.payment_count(c["order_id"]) == 0


# ── When: checkout navigation ────────────────────────────────────────────
@when(parsers.parse('the cc buyer opens checkout at the "{step}" step'))
def open_at_step(world: World, step: str) -> None:
    open_confirm(world)
    checkout_page(world).continue_to(step)


@when(parsers.parse('the cc buyer goes on to the "{step}" step'))
def go_on(world: World, step: str) -> None:
    checkout_page(world).continue_to(step)


@when(parsers.parse('the cc buyer opens the "payment" step with shop B\'s own code on group B'))
def payment_own_code(world: World) -> None:
    open_confirm(world)
    checkout_page(world).continue_to("payment")
    c = cc.ctx(world)
    goto_with_query(world, [("sv", f"{c['shop_b']['seller_id']}:{c['code_b']}")])


@when(parsers.parse('the cc buyer opens the "payment" step with shop A\'s code on group B'))
def payment_wrong_scope(world: World) -> None:
    c = cc.ctx(world)
    if "payment_reached" not in c:
        open_confirm(world)
        checkout_page(world).continue_to("payment")
    c["payment_reached"] = True
    goto_with_query(world, [("sv", f"{c['shop_b']['seller_id']}:{c['code_a']}")])


@when(
    parsers.parse(
        'the cc buyer opens the "payment" step with both shop vouchers and the platform voucher'
    )
)
def payment_all_vouchers(world: World) -> None:
    open_confirm(world)
    checkout_page(world).continue_to("payment")
    c = cc.ctx(world)
    goto_with_query(
        world,
        [
            ("sv", f"{c['shop_a']['seller_id']}:{c['code_a']}"),
            ("sv", f"{c['shop_b']['seller_id']}:{c['code_b']}"),
            ("pv", c["code_p"]),
        ],
    )


@when("the cc buyer places the order")
def place_order(world: World) -> None:
    world.state.extra["cc_shown"] = summary(world)
    cc.ctx(world)["shown_total"] = world.state.extra["cc_shown"]["total"]
    cc.ctx(world)["shown_discount"] = shown_discount(world)
    button = checkout_page(world).place_order_button
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    world.page.wait_for_timeout(500)
    button.click()
    world.page.wait_for_url(
        re.compile(r".*/(account/orders|checkout/pay).*"),
        timeout=timeouts.NAVIGATION,
        wait_until="commit",
    )


def choose_method(world: World, value: str) -> None:
    open_confirm(world)
    page = checkout_page(world)
    page.continue_to("payment")
    radio = world.page.locator(f'input[name="paymentMethod"][value="{value}"]')
    expect(radio).to_have_count(1, timeout=timeouts.DEFAULT)
    radio.check(force=True)
    expect(radio).to_be_checked()
    world.page.wait_for_url(re.compile(rf".*[?&]pay={value}\b"), timeout=timeouts.DEFAULT)
    page.continue_to("confirm")


@when("the cc buyer places the order paying with MoMo")
def place_momo(world: World) -> None:
    choose_method(world, "2")
    place_order(world)


@when("the cc buyer places the order paying cash on delivery")
def place_cod(world: World) -> None:
    choose_method(world, "1")
    place_order(world)


@when(
    parsers.parse('the cc buyer\'s address street is edited to "{street}" through the address API')
)
def edit_address(world: World, street: str) -> None:
    c = cc.ctx(world)
    address = cc.default_address(world)
    edited = addr.edit_address(world, c["buyer"], address, street=street)
    assert edited.get("street") == street, edited
    c["edited_street"] = street


@when("the cc buyer presses the place order button")
def press_place(world: World) -> None:
    button = checkout_page(world).place_order_button
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    world.page.wait_for_timeout(500)
    button.click()
    world.page.wait_for_timeout(1500)


# ── When: delivery date ─────────────────────────────────────────────────
@when("the cc buyer opens the product page of the item")
def open_pdp(world: World) -> None:
    listing_id = cc.ctx(world)["shops"][0]["listing_id"]
    world.page.goto(f"{cc.base(world)}/listing/{listing_id}", wait_until="domcontentloaded")


@when("the cc visitor is anonymous and opens the product page of the item")
def anonymous_pdp(world: World) -> None:
    adv.sign_in_browser(world, None)
    open_pdp(world)


# ── When: payments ──────────────────────────────────────────────────────
@when("the cc buyer pays successfully on the pay page")
def pay_success(world: World) -> None:
    page = world.page
    button = page.get_by_role("button", name="Thanh toán thành công")
    expect(button).to_be_enabled(timeout=timeouts.NAVIGATION)
    page.wait_for_timeout(500)
    button.click()
    expect(page.get_by_role("heading", name="Thanh toán thành công")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@when(parsers.parse('the cc buyer follows "{label}"'))
def follow_link(world: World, label: str) -> None:
    world.page.get_by_role("link", name=label).first.click()


@when("the cc buyer opens the order list")
def open_orders(world: World) -> None:
    world.page.goto(f"{cc.base(world)}/account/orders", wait_until="domcontentloaded")


@when(parsers.parse('the cc buyer presses "{label}" on the order'))
def press_pay_now(world: World, label: str) -> None:
    c = cc.ctx(world)
    link = world.page.get_by_role("link", name=re.compile(r"Thanh toán (ngay|lại)")).first
    expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
    with world.page.expect_response(
        lambda r: f"/checkout/pay/{c['order_id']}" in r.url,
        timeout=timeouts.NAVIGATION,
    ) as info:
        link.click()
    c["pay_response"] = info.value


# ── Then: totals ────────────────────────────────────────────────────────
@then("the cc summary total shown is the server preview total")
def total_is_preview(world: World) -> None:
    quote = cc.preview(world)
    rows = summary(world)
    assert rows["total"] == cc.n(quote.get("totalAmount")), (rows, quote)
    assert len(quote.get("groups", [])) == 2, quote
    cc.ctx(world)["quote"] = quote


@then(parsers.parse("the cc confirm page shows the per-group shipping fee {fee:d} twice"))
def group_fee_twice(world: World, fee: int) -> None:
    text = world.page.locator("body").inner_text()
    assert text.count(cc.vnd(fee)) >= 2, text


@then(
    parsers.parse(
        "the cc buyer has {count:d} orders whose total amounts sum to the summary total shown before placing"
    )
)
def orders_sum_shown(world: World, count: int) -> None:
    c = cc.ctx(world)
    orders = wait_orders(c["buyer"].token, count)
    total = sum(cc.n(o.get("totalAmount")) for o in orders)
    assert total == c["shown_total"], (total, c["shown_total"], orders)


def wait_orders(token: str, count: int, timeout: float = 20.0) -> list[dict[str, Any]]:
    deadline = time.monotonic() + timeout
    orders: list[dict[str, Any]] = []
    while time.monotonic() < deadline:
        orders = cc.buyer_orders(token)
        if len(orders) >= count:
            break
        time.sleep(0.5)
    assert len(orders) == count, orders
    return orders


# ── Then: vouchers ──────────────────────────────────────────────────────
@then("the cc summary shows a voucher discount and a total below the undiscounted total")
def discount_applied(world: World) -> None:
    c = cc.ctx(world)
    undiscounted = cc.n(cc.preview(world).get("totalAmount"))
    c["undiscounted"] = undiscounted
    expect(world.page.get_by_test_id("voucher-discount")).to_contain_text(
        re.compile(r"[1-9]"), timeout=timeouts.NAVIGATION
    )
    rows = summary(world)
    assert 0 < rows["total"] < undiscounted, (rows, undiscounted)


@then("the cc page shows the server's refusal reason for group B's voucher")
def refusal_reason(world: World) -> None:
    c = cc.ctx(world)
    quote = cc.preview(world, [(c["shop_b"]["seller_id"], c["code_a"])])
    refused = [v for v in quote.get("vouchers", []) if v.get("code") == c["code_a"]]
    assert refused and not refused[0].get("valid"), quote
    reason = refused[0].get("reason", "")
    assert reason, quote
    expect(world.page.get_by_text(reason).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the cc summary shows no voucher discount and the undiscounted total")
def no_discount(world: World) -> None:
    c = cc.ctx(world)
    assert shown_discount(world) == 0, summary(world)
    assert summary(world)["total"] == c["undiscounted"], (summary(world), c["undiscounted"])


@then("the cc summary discount is the server preview discount")
def discount_is_preview(world: World) -> None:
    c = cc.ctx(world)
    quote = cc.preview(
        world,
        [
            (c["shop_a"]["seller_id"], c["code_a"]),
            (c["shop_b"]["seller_id"], c["code_b"]),
        ],
        c["code_p"],
    )
    assert all(v.get("valid") for v in quote.get("vouchers", [])), quote
    c["quote"] = quote
    want = cc.n(quote.get("discountAmount"))
    assert want > 0, quote
    expect(world.page.get_by_test_id("voucher-discount")).to_contain_text(
        re.compile(r"[1-9]"), timeout=timeouts.NAVIGATION
    )
    assert shown_discount(world) == want, (shown_discount(world), quote)


@then("each placed cc order discount equals its group's shop discount plus platform share")
def per_order_discount(world: World) -> None:
    c = cc.ctx(world)
    orders = wait_orders(c["buyer"].token, 2)
    groups = {g["sellerId"]: g for g in c["quote"].get("groups", [])}
    for order in orders:
        group = groups[order["sellerId"]]
        want = cc.n(group.get("shopDiscountAmount")) + cc.n(group.get("platformDiscountAmount"))
        assert want > 0, group
        assert cc.n(order.get("discountAmount")) == want, (order, group)
    c["placed"] = orders


@then("the cc placed orders discounts sum to the summary discount shown")
def discounts_sum(world: World) -> None:
    c = cc.ctx(world)
    total = sum(cc.n(o.get("discountAmount")) for o in c["placed"])
    assert total == c["shown_discount"] > 0, (total, c["shown_discount"])


# ── Then: delivery date ─────────────────────────────────────────────────
@then(parsers.parse("the cc page shows the delivery date of today plus {days:d} days"))
def shows_date(world: World, days: int) -> None:
    expect(world.page.get_by_text(cc.eta_pattern(days)).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then("the cc page shows no delivery date")
def shows_no_date(world: World) -> None:
    world.page.wait_for_load_state("domcontentloaded")
    expect(world.page.get_by_text(re.compile(r"Nhận hàng dự kiến"))).to_have_count(0)


# ── Then: address ───────────────────────────────────────────────────────
@then(parsers.parse('the cc confirm page shows the street "{street}"'))
def shows_street(world: World, street: str) -> None:
    expect(world.page.get_by_text(street).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the cc buyer has no order")
def no_order(world: World) -> None:
    assert cc.buyer_orders(cc.ctx(world)["buyer"].token) == []


@then(
    parsers.parse('the cc page shows the street "{street}" and a message that the address changed')
)
def shows_new_street_and_message(world: World, street: str) -> None:
    page = world.page
    expect(page.get_by_text(street).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(
        page.get_by_role("alert").filter(has_text=re.compile(r"địa chỉ.*thay đổi", re.I)).first
    ).to_be_visible(timeout=timeouts.DEFAULT)


@then(parsers.parse('the cc buyer has {count:d} order placed at the street "{street}"'))
def order_at_street(world: World, count: int, street: str) -> None:
    orders = wait_orders(cc.ctx(world)["buyer"].token, count)
    assert (orders[0].get("shippingAddress") or {}).get("street") == street, orders


# ── Then: payments ──────────────────────────────────────────────────────
@then("the cc browser lands on the pay page of one of the two orders")
def lands_first_pay(world: World) -> None:
    c = cc.ctx(world)
    world.page.wait_for_url(PAY_URL, timeout=timeouts.NAVIGATION, wait_until="commit")
    c["first_id"] = PAY_URL.match(world.page.url).group(1)
    orders = wait_orders(c["buyer"].token, 2)
    c["order_ids"] = {o["id"] for o in orders}
    assert c["first_id"] in c["order_ids"], (world.page.url, c["order_ids"])


@then('the cc success state links to "Thanh toán đơn tiếp theo"')
def next_link(world: World) -> None:
    expect(world.page.get_by_role("link", name="Thanh toán đơn tiếp theo").first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then("the cc browser lands on the pay page of the other order")
def lands_second_pay(world: World) -> None:
    c = cc.ctx(world)
    other = next(iter(c["order_ids"] - {c["first_id"]}))
    world.page.wait_for_url(
        re.compile(rf".*/checkout/pay/{other}"), timeout=timeouts.NAVIGATION, wait_until="commit"
    )
    c["second_id"] = other


@then("both cc orders reach status PAID with exactly one payment transaction each")
def both_paid(world: World) -> None:
    c = cc.ctx(world)
    deadline = time.monotonic() + 25
    statuses: dict[str, str] = {}
    while time.monotonic() < deadline:
        statuses = {
            oid: cc.get_order(c["buyer"].token, oid).get("status", "")
            for oid in (c["first_id"], c["second_id"])
        }
        if set(statuses.values()) == {"ORDER_STATUS_PAID"}:
            break
        time.sleep(1)
    assert set(statuses.values()) == {"ORDER_STATUS_PAID"}, statuses
    for oid in statuses:
        assert cc.payment_count(oid) == 1, oid


@then(parsers.parse("the cc buyer has {count:d} orders and none has a payment transaction"))
def cod_no_tx(world: World, count: int) -> None:
    c = cc.ctx(world)
    orders = wait_orders(c["buyer"].token, count)
    for order in orders:
        assert order.get("paymentMethod") == cc.COD, order
        assert cc.payment_count(order["id"]) == 0, order


@then("the cc pay page answers 200 with the order amount and the payment simulator")
def pay_page_ok(world: World) -> None:
    c = cc.ctx(world)
    response = c["pay_response"]
    assert response.status == 200, response.status
    order = cc.get_order(c["buyer"].token, c["order_id"])
    page = world.page
    expect(page.get_by_role("button", name="Thanh toán thành công")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(page.get_by_text(cc.vnd(cc.n(order.get("totalAmount")))).first).to_be_visible()
