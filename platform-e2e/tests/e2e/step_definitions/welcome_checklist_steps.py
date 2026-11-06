from __future__ import annotations

import re
import time
from collections.abc import Callable
from datetime import datetime, timezone
from typing import Any
from urllib.parse import parse_qs, urlsplit

from playwright.sync_api import Locator, expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AddressService, CartService, OrderService
from src.api.services.base_service import BaseService
from src.constants import PageName, timeouts
from src.core.base_page import goto_streamed
from src.models import User
from src.pages.checkout_page import CheckoutPage
from src.pages.shop_follow_page import ShopFollowPage
from tests.e2e.flows.auth_flow import session_cookies
from tests.e2e.support import d2_support as d2
from tests.e2e.support import flash_hold_support as fh
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.voucher_seed import ensure_known_voucher
from tests.e2e.support.world import World

SETTINGS = get_settings()
BASE = SETTINGS.base_url.rstrip("/")
VOUCHER_RPC = "/platform.promotion.v1.VoucherService/ListMyVouchers"
ISSUE_WAIT_S = 45
PENDING_TEXT = "Voucher đang được chuẩn bị"
MY_VOUCHERS_HEADING = "Voucher của bạn"
SETTLE_MS = 3_000
DISMISS_MAX_AGE_S = 1_209_600
STEP_NAMES = {
    "Theo dõi một shop",
    "Thêm địa chỉ giao hàng",
    "Đặt đơn đầu tiên",
}
DONG = re.compile("Đóng", re.IGNORECASE)


def _x(world: World) -> dict[str, Any]:
    return world.state.extra


def _until(fn: Callable[[], Any], *, timeout_s: float, what: str, interval_s: float = 1.0) -> Any:
    deadline = time.monotonic() + timeout_s
    last: Any = None
    while time.monotonic() < deadline:
        last = fn()
        if last:
            return last
        time.sleep(interval_s)
    raise AssertionError(f"timed out after {timeout_s:.0f}s waiting for {what} (last={last!r})")


def _my_vouchers(token: str) -> list[dict[str, Any]]:
    api = BaseService(token=token)
    try:
        return api.post(VOUCHER_RPC, {}).get("vouchers", [])
    finally:
        api.close()


def _new_user_voucher(token: str) -> dict[str, Any] | None:
    for voucher in _my_vouchers(token):
        if "NEW_USER" in str(voucher.get("kind", "")):
            return voucher
    return None


def _wait_voucher(token: str) -> dict[str, Any]:
    return _until(
        lambda: _new_user_voucher(token),
        timeout_s=ISSUE_WAIT_S,
        what="the NEW_USER voucher to be issued",
    )


def _register_buyer() -> dict[str, Any]:
    username, token = d2.register("buyer", "wc_buyer")
    return {
        "username": username,
        "token": token,
        "id": d2.principal_id(token),
    }


def _sign_in(world: World, buyer: dict[str, Any]) -> None:
    world.context.clear_cookies()
    world.context.add_cookies(
        [
            *session_cookies(world.settings, buyer["token"]),
            {
                "name": "agora_consent",
                "value": f"v=1&a=1&m=0&p=0&ts={int(time.time())}",
                "url": world.settings.base_url,
            },
        ]
    )
    world.service_factory.set_token(buyer["token"])
    world.state.current_user = User(
        username=buyer["username"],
        password=SETTINGS.seed_password,
        role="buyer",
        token=buyer["token"],
    )
    _x(world)["d2_buyer"] = world.state.current_user


def _buyer(world: World) -> dict[str, Any]:
    return _x(world)["wc_buyer"]


def _set_age(buyer: dict[str, Any], interval: str) -> None:
    ue._psql(
        f"UPDATE users SET created_at = now() - interval '{interval}' WHERE id = '{buyer['id']}';"
    )


def _address(token: str) -> None:
    AddressService(token=token).create_address(
        recipient_name="Nguyen Van A",
        phone="0912345678",
        street="29 Lieu Giai",
        city="Hà Nội",
        ward="Phuong Lieu Giai",
        district="Quan Ba Dinh",
        is_default=True,
    )


def _goto(world: World, route: str) -> None:
    goto_streamed(world.page, f"{BASE}{route}")
    world.page.wait_for_load_state("networkidle")


def _settle(world: World) -> None:
    world.page.wait_for_timeout(SETTLE_MS)


def _banner(world: World) -> Locator:
    return world.page.get_by_test_id("welcome-banner")


def _checklist(world: World) -> Locator:
    return world.page.get_by_test_id("welcome-checklist")


def _step(world: World, name: str) -> Locator:
    assert name in STEP_NAMES, name
    return world.page.get_by_test_id("welcome-step").filter(has_text=name)


def _amount_pattern(voucher: dict[str, Any]) -> re.Pattern[str]:
    value = int(voucher.get("discountValue") or 0)
    dotted = f"{value:,}".replace(",", ".")
    if "PERCENT" in str(voucher.get("discountType", "")):
        return re.compile(rf"{value}\s*%")
    return re.compile(
        rf"(?:[đ₫]\s*{re.escape(dotted)}(?![\d.])|(?<![\d.]){re.escape(dotted)}\s*[đ₫])"
    )


def _checkout(world: World) -> CheckoutPage:
    return world.get_page(PageName.CHECKOUT)  # type: ignore[return-value]


def _cookie(world: World, name: str) -> dict[str, Any] | None:
    found = [c for c in world.context.cookies() if c["name"] == name]
    return found[0] if found else None


def _seed_cart(world: World, buyer: dict[str, Any], price: int, quantity: int = 1) -> None:
    shop = d2.seed_shop(price=price)
    CartService(token=buyer["token"]).add_to_cart(shop["listing_id"], quantity)


# ── givens ────────────────────────────────────────────────────────────────────
@given("a welcome buyer holding a NEW_USER voucher is signed in")
def welcome_buyer_with_voucher(world: World) -> None:
    buyer = _register_buyer()
    buyer["voucher"] = _wait_voucher(buyer["token"])
    _x(world)["wc_buyer"] = buyer
    _sign_in(world, buyer)


@given("a second welcome buyer holding a NEW_USER voucher")
def second_welcome_buyer(world: World) -> None:
    buyer = _register_buyer()
    buyer["voucher"] = _wait_voucher(buyer["token"])
    _x(world)["wc_buyer2"] = buyer


@given("a freshly registered welcome buyer is signed in")
def fresh_welcome_buyer(world: World) -> None:
    buyer = _register_buyer()
    _x(world)["wc_buyer"] = buyer
    _sign_in(world, buyer)


@given("a welcome buyer without address or order is signed in")
def welcome_buyer_plain(world: World) -> None:
    buyer = _register_buyer()
    _x(world)["wc_buyer"] = buyer
    _sign_in(world, buyer)


@given("a welcome buyer with an address is signed in")
def welcome_buyer_with_address(world: World) -> None:
    buyer = _register_buyer()
    _address(buyer["token"])
    _x(world)["wc_buyer"] = buyer
    _sign_in(world, buyer)


@given(
    "a welcome buyer holding a NEW_USER voucher is signed in with a qualifying cart and an address"
)
def welcome_buyer_qualifying_cart(world: World) -> None:
    welcome_buyer_with_voucher(world)
    buyer = _buyer(world)
    _address(buyer["token"])
    min_spend = int(buyer["voucher"].get("minSpend") or 0)
    _seed_cart(world, buyer, max(min_spend, 100_000))


@given(
    "a welcome buyer holding a NEW_USER voucher is signed in with a cart below the voucher "
    "minimum spend and an address"
)
def welcome_buyer_small_cart(world: World) -> None:
    welcome_buyer_with_voucher(world)
    buyer = _buyer(world)
    _address(buyer["token"])
    min_spend = int(buyer["voucher"].get("minSpend") or 0)
    assert min_spend > 1_000, f"the NEW_USER voucher has no usable minimum spend: {min_spend}"
    _seed_cart(world, buyer, min_spend - 1_000)


@given(parsers.parse('the platform voucher "{code}" exists'))
def platform_voucher_exists(world: World, code: str) -> None:
    ensure_known_voucher(code)
    fh.psql(f"UPDATE vouchers SET created_at = now() WHERE code = {fh.sql_lit(code)};")


@given(parsers.parse("the welcome buyer's account is {days:d} days old"))
def account_days_old(world: World, days: int) -> None:
    _set_age(_buyer(world), f"{days} days")


@given("the welcome buyer's account is 13 days and 23 hours old")
def account_13d23h(world: World) -> None:
    _set_age(_buyer(world), "13 days 23 hours")


@given("the welcome buyer follows a shop")
def buyer_follows_shop(world: World) -> None:
    shop = d2.seed_shop()
    follow = ShopFollowPage(world.page)
    follow.open(shop["seller_id"])
    expect(follow.follow_toggle).to_be_visible(timeout=timeouts.NAVIGATION)
    follow.toggle_follow()
    expect(world.page.get_by_text("Đã theo dõi shop", exact=False)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@given("the welcome buyer has only a cancelled order")
def buyer_cancelled_order(world: World) -> None:
    buyer = _buyer(world)
    shop = d2.seed_shop()
    CartService(token=buyer["token"]).add_to_cart(shop["listing_id"], 1)
    orders = OrderService(token=buyer["token"])
    res = orders.create_order({"paymentMethod": "PAYMENT_METHOD_COD"})
    found = res.get("orders", [])
    order_id = found[0]["id"] if found else res["order"]["id"]
    orders.cancel_order(order_id, "changed my mind")
    _until(
        lambda: (
            all(
                o.get("status") == "ORDER_STATUS_CANCELLED"
                for o in orders.list_buyer_orders().get("orders", [])
            )
            and orders.list_buyer_orders().get("orders")
        ),
        timeout_s=20,
        what="the order to be cancelled",
        interval_s=0.5,
    )


# ── whens ─────────────────────────────────────────────────────────────────────
@when("the welcome buyer opens the vouchers page")
def open_vouchers(world: World) -> None:
    _goto(world, "/vouchers")
    _settle(world)


@when("the second buyer opens the vouchers page")
def second_opens_vouchers(world: World) -> None:
    _sign_in(world, _x(world)["wc_buyer2"])
    _goto(world, "/vouchers")
    _settle(world)


@when("an anonymous visitor opens the vouchers page")
def anonymous_vouchers(world: World) -> None:
    world.context.clear_cookies()
    _goto(world, "/vouchers")
    _settle(world)


@when("the welcome buyer opens the home page")
def open_home(world: World) -> None:
    _goto(world, "/")
    _settle(world)


@when("the welcome buyer opens the addresses page")
def open_addresses(world: World) -> None:
    _goto(world, "/account/addresses")
    _settle(world)


@when("an anonymous visitor opens the home page")
def anonymous_home(world: World) -> None:
    world.context.clear_cookies()
    _goto(world, "/")
    _settle(world)


@when("an anonymous visitor opens the following page")
def anonymous_following(world: World) -> None:
    world.context.clear_cookies()
    world.page.goto(f"{BASE}/account/following", wait_until="domcontentloaded")
    world.page.wait_for_url(lambda url: urlsplit(url).path == "/login", timeout=timeouts.NAVIGATION)


@when("an anonymous visitor opens the checkout page")
def anonymous_checkout(world: World) -> None:
    world.context.clear_cookies()
    world.page.goto(f"{BASE}/checkout", wait_until="domcontentloaded")
    world.page.wait_for_url(lambda url: urlsplit(url).path == "/login", timeout=timeouts.NAVIGATION)


@when("the welcome buyer's account becomes 14 days old")
def account_becomes_14d(world: World) -> None:
    _set_age(_buyer(world), "14 days")


@when("the welcome buyer opens the payment step")
def open_payment(world: World) -> None:
    world.navigate_to(PageName.CHECKOUT)
    page = _checkout(world)
    expect(page.stepper).to_be_visible(timeout=timeouts.NAVIGATION)
    page.continue_to("payment")


@when("the welcome buyer picks the voucher from the picker")
def pick_voucher(world: World) -> None:
    code = _buyer(world)["voucher"]["code"]
    checkout = _checkout(world)
    offer = checkout.platform_voucher.get_by_role("button", name=code, exact=True)
    expect(offer).to_be_enabled(timeout=timeouts.NAVIGATION)
    checkout.wait_until_interactive(offer)
    offer.click()


@when("the welcome buyer closes the welcome banner")
def close_banner(world: World) -> None:
    banner = _banner(world)
    expect(banner).to_be_visible(timeout=timeouts.DEFAULT)
    banner.get_by_role("button", name=DONG).click()
    expect(banner).to_have_count(0, timeout=timeouts.DEFAULT)


@when("the welcome buyer hides the checklist")
def hide_checklist(world: World) -> None:
    checklist = _checklist(world)
    expect(checklist).to_be_visible(timeout=timeouts.DEFAULT)
    checklist.get_by_role("button", name="Ẩn danh sách").click()
    expect(checklist).to_have_count(0, timeout=timeouts.DEFAULT)


@when("the welcome buyer places an order")
def place_order(world: World) -> None:
    d2.place_order(world, d2.seed_shop())
    orders = OrderService(token=_buyer(world)["token"])
    _until(
        lambda: orders.list_buyer_orders().get("orders"),
        timeout_s=20,
        what="the order to be listed",
        interval_s=0.5,
    )


# ── thens ─────────────────────────────────────────────────────────────────────
@then(
    'the page has a "Voucher của bạn" section containing the voucher code, its amount and its expiry'
)
def own_section(world: World) -> None:
    voucher = _buyer(world)["voucher"]
    heading = world.page.get_by_text(MY_VOUCHERS_HEADING, exact=True).first
    expect(heading).to_be_visible(timeout=timeouts.NAVIGATION)
    section = (
        world.page.locator("section, div").filter(has=heading).filter(has_text=voucher["code"]).last
    )
    expect(section).to_contain_text(voucher["code"])
    expect(section).to_contain_text(_amount_pattern(voucher))
    expect(section).to_contain_text(re.compile(r"\d{1,2}[/.-]\d{1,2}[/.-]\d{4}"))


@then(parsers.parse('the section comes before the platform voucher "{code}"'))
def section_before_platform(world: World, code: str) -> None:
    own = world.page.get_by_text(_buyer(world)["voucher"]["code"]).first.bounding_box()
    platform = world.page.locator(
        f'[data-testid="voucher-card"][data-code="{code}"]'
    ).first.bounding_box()
    assert own and platform, "own voucher or platform card not rendered"
    assert own["y"] < platform["y"], (own, platform)


@then(parsers.parse('the platform voucher "{code}" is listed'))
def platform_listed(world: World, code: str) -> None:
    card = world.page.locator(f'[data-testid="voucher-card"][data-code="{code}"]')
    expect(card.first).to_be_visible(timeout=timeouts.NAVIGATION)


@then('the page has no "Voucher của bạn" section')
def no_own_section(world: World) -> None:
    expect(world.page.get_by_text(MY_VOUCHERS_HEADING)).to_have_count(0)


@then("the page shows the welcome buyer's voucher code")
def shows_first_code(world: World) -> None:
    expect(world.page.get_by_text(_buyer(world)["voucher"]["code"]).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then("the page shows the second buyer's voucher code")
def shows_second_code(world: World) -> None:
    code = _x(world)["wc_buyer2"]["voucher"]["code"]
    expect(world.page.get_by_text(code).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the page does not show the second buyer's voucher code")
def hides_second_code(world: World) -> None:
    code = _x(world)["wc_buyer2"]["voucher"]["code"]
    expect(world.page.get_by_text(code)).to_have_count(0)


@then("the page does not show the welcome buyer's voucher code")
def hides_first_code(world: World) -> None:
    expect(world.page.get_by_text(_buyer(world)["voucher"]["code"])).to_have_count(0)


@then("the voucher code is marked applied in the platform slot")
def code_applied(world: World) -> None:
    code = _buyer(world)["voucher"]["code"]
    slot = _checkout(world).platform_voucher
    expect(slot.get_by_role("button", name=f"Bỏ mã {code}")).to_be_visible(timeout=timeouts.DEFAULT)


@then("the voucher code is not marked applied in the platform slot")
def code_not_applied(world: World) -> None:
    code = _buyer(world)["voucher"]["code"]
    slot = _checkout(world).platform_voucher
    expect(slot.get_by_role("button", name=f"Bỏ mã {code}")).to_have_count(0)


@then("the checkout quote carries the voucher code as the platform voucher")
def quote_carries_code(world: World) -> None:
    code = _buyer(world)["voucher"]["code"]
    world.page.wait_for_url(
        lambda url: parse_qs(urlsplit(url).query).get("pv") == [code], timeout=timeouts.DEFAULT
    )


@then("the discount row shows a discount")
def discount_row(world: World) -> None:
    expect(_checkout(world).voucher_discount).to_contain_text(
        re.compile(r"\d"), timeout=timeouts.DEFAULT
    )


@then("the checkout shows an invalid-voucher state")
def invalid_state(world: World) -> None:
    expect(world.page.get_by_role("alert").first).to_be_visible(timeout=timeouts.DEFAULT)
    assert "pv" not in parse_qs(urlsplit(world.page.url).query), world.page.url


@then("the visitor is on the login page")
def on_login(world: World) -> None:
    assert urlsplit(world.page.url).path == "/login", world.page.url


@then("the welcome banner shows the voucher code, the amount in đ and the expiry date")
def banner_full(world: World) -> None:
    voucher = _buyer(world)["voucher"]
    banner = _banner(world)
    expect(banner).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(banner).to_contain_text(voucher["code"])
    expect(banner).to_contain_text(_amount_pattern(voucher))
    expect(banner).to_contain_text(re.compile(r"\d{1,2}[/.-]\d{1,2}[/.-]\d{4}"))


@then("the welcome banner shows the pending text without a code until the voucher exists")
def banner_pending(world: World) -> None:
    buyer = _buyer(world)
    banner = _banner(world)
    expect(banner).to_be_visible(timeout=timeouts.NAVIGATION)
    text = banner.text_content() or ""
    if PENDING_TEXT in text:
        voucher = _new_user_voucher(buyer["token"])
        assert voucher is None or voucher["code"] not in text, text
    else:
        voucher = _new_user_voucher(buyer["token"])
        assert voucher is not None, f"banner shows neither the pending text nor a voucher: {text!r}"
        assert voucher["code"] in text, text


@then("within 10 seconds the welcome banner shows the voucher code")
def banner_gets_code(world: World) -> None:
    buyer = _buyer(world)
    voucher = _wait_voucher(buyer["token"])
    buyer["voucher"] = voucher
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        _goto(world, "/account/addresses")
        if voucher["code"] in (_banner(world).text_content() or ""):
            break
        time.sleep(1)
    expect(_banner(world)).to_contain_text(voucher["code"])
    expect(_banner(world)).not_to_contain_text(PENDING_TEXT)


@then("the welcome banner shows the voucher code")
def banner_code(world: World) -> None:
    expect(_banner(world)).to_contain_text(
        _buyer(world)["voucher"]["code"], timeout=timeouts.DEFAULT
    )


@then("the welcome panel is rendered")
def panel_rendered(world: World) -> None:
    expect(_banner(world).or_(_checklist(world)).first).to_be_visible(timeout=timeouts.DEFAULT)


@then("the welcome panel is not rendered")
def panel_absent(world: World) -> None:
    expect(_banner(world)).to_have_count(0)
    expect(_checklist(world)).to_have_count(0)


@then("the welcome banner is absent")
def banner_absent(world: World) -> None:
    expect(_banner(world)).to_have_count(0)


@then("the welcome checklist is absent")
def checklist_absent(world: World) -> None:
    expect(_checklist(world)).to_have_count(0)


@then(parsers.parse('the text "{text}" is not shown'))
def text_absent(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_have_count(0)


@then(parsers.parse('the cookie "{name}" is set with value 1 and the session attributes'))
def dismiss_cookie(world: World, name: str) -> None:
    cookie = _cookie(world, name)
    assert cookie is not None, f"{name} not set"
    session = _cookie(world, "session")
    assert session is not None, "no session cookie"
    assert cookie["value"] == "1", cookie
    assert cookie["httpOnly"] is True, cookie
    assert cookie["sameSite"] == "Lax", cookie
    assert cookie["path"] == "/", cookie
    assert cookie["secure"] == session["secure"], (cookie, session)
    remaining = cookie["expires"] - datetime.now(timezone.utc).timestamp()
    assert abs(remaining - DISMISS_MAX_AGE_S) < 300, remaining


def _step_state(world: World, name: str, done: bool) -> None:
    step = _step(world, name)
    expect(step).to_have_count(1, timeout=timeouts.DEFAULT)
    expect(step).to_have_attribute("data-done", "true" if done else "false")


@then(parsers.parse('the checklist step "{name}" is done'))
def step_done(world: World, name: str) -> None:
    _step_state(world, name, True)


@then(parsers.parse('the checklist step "{name}" is not done'))
def step_not_done(world: World, name: str) -> None:
    _step_state(world, name, False)
