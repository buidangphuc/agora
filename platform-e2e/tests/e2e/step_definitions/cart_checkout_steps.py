"""Steps for buyer/cart_checkout.feature (ui-phase-cart-checkout).

Reuse-first: login, "opens the cart", "proceeds to checkout", the promo seeding step
("a promotion buyer has a qualifying cart and a saved address") and the shop-name
seeding steps come from their own modules; only the new cart/checkout/payment UI
behaviour is defined here. Preconditions are seeded through the gateway API by the
same principal the browser is logged in as.
"""

from __future__ import annotations

import re
import time

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import PageName, timeouts
from src.pages import CartPage, CheckoutPage
from src.pages.payment_result_page import PaymentResultPage
from src.utils import data as fake
from tests.e2e.flows import create_order_via_api, login_via_api, scenario_buyer
from tests.e2e.flows.auth_flow import SESSION_COOKIE
from tests.e2e.support.world import World

SETTINGS = get_settings()
_MOBILE = {"width": 375, "height": 812}


def _cart(world: World) -> CartPage:
    return world.get_page(PageName.CART)  # type: ignore[return-value]


def _checkout(world: World) -> CheckoutPage:
    return world.get_page(PageName.CHECKOUT)  # type: ignore[return-value]


def _payment(world: World) -> PaymentResultPage:
    return world.get_page(PageName.PAYMENT_RESULT)  # type: ignore[return-value]


# ── Cart: shop groups ────────────────────────────────────────────────────
@when("the buyer opens the cart with the seeded session")
def buyer_opens_cart_with_session(world: World) -> None:
    """The shop-name seeding step logs the buyer in through the API only; set the browser cookie."""
    token = world.state.extra["buyer_token"]
    world.context.add_cookies(
        [{"name": SESSION_COOKIE, "value": token, "url": world.settings.base_url}]
    )
    world.navigate_to(PageName.CART)


@then(parsers.parse('the cart shows two shop groups headed "{first}" and "{second}"'))
def cart_two_shop_groups(world: World, first: str, second: str) -> None:
    cart = _cart(world)
    expect(cart.shop_groups).to_have_count(2, timeout=timeouts.DEFAULT)
    expect(cart.shop_header(first)).to_be_visible()
    expect(cart.shop_header(second)).to_be_visible()
    # a real name is shown, never the id fallback
    expect(cart.shop_groups.get_by_text("Shop #", exact=False)).to_have_count(0)


@then("the empty cart offers a link to continue shopping")
def empty_cart_offers_continue(world: World) -> None:
    cart = _cart(world)
    expect(cart.continue_shopping_link).to_have_attribute("href", "/", timeout=timeouts.DEFAULT)
    expect(cart.order_summary).to_have_count(0)


# ── Checkout wizard ──────────────────────────────────────────────────────
@given(parsers.parse('the buyer has a second saved address for "{recipient}"'))
def buyer_second_address(world: World, recipient: str) -> None:
    # The promo seeding step left the buyer's token on the service factory.
    world.service_factory.address.create_address(
        recipient_name=recipient,
        phone="0987654321",
        street="12 Nguyen Hue",
        city="Ho Chi Minh",
        ward="Phuong Ben Nghe",
        district="Quan 1",
        is_default=False,
    )
    world.state.extra["second_recipient"] = recipient


@when("the buyer opens the checkout page")
def buyer_opens_checkout(world: World) -> None:
    world.navigate_to(PageName.CHECKOUT)
    expect(_checkout(world).stepper).to_be_visible(timeout=timeouts.NAVIGATION)


@when(parsers.parse('the buyer goes to the "{step}" checkout step'))
def buyer_goes_to_step(world: World, step: str) -> None:
    _checkout(world).continue_to(step)


@when("the buyer reloads the checkout page")
def buyer_reloads_checkout(world: World) -> None:
    world.page.reload(wait_until="domcontentloaded")


@then("the checkout page shows the stepper without the global search")
def checkout_shell(world: World) -> None:
    checkout = _checkout(world)
    expect(checkout.stepper).to_be_visible(timeout=timeouts.DEFAULT)
    expect(checkout.global_search).to_have_count(0)


@then(parsers.parse('the checkout URL shows step "{step}" and the selected address'))
def checkout_url_step(world: World, step: str) -> None:
    world.page.wait_for_url(
        re.compile(rf".*[?&]step={step}\b.*addr=.+"), timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('the "{step}" step is the current step'))
def step_is_current(world: World, step: str) -> None:
    labels = {
        "address": "Địa chỉ",
        "shipping": "Vận chuyển",
        "payment": "Thanh toán",
        "confirm": "Xác nhận",
    }
    expect(_checkout(world).current_step_item).to_contain_text(
        labels[step], timeout=timeouts.DEFAULT
    )


@when(parsers.parse('the buyer changes the delivery address to "{recipient}"'))
def buyer_changes_address(world: World, recipient: str) -> None:
    checkout = _checkout(world)
    # The opener is server-rendered; a click before hydration is dropped.
    checkout.wait_until_interactive(checkout.change_address_button)
    checkout.change_address_button.click()
    expect(checkout.dialog).to_be_visible(timeout=timeouts.DEFAULT)
    checkout.address_option(recipient).check()
    checkout.confirm_address_button.click()
    expect(checkout.dialog).to_have_count(0, timeout=timeouts.DEFAULT)


@then(parsers.parse('the address card shows "{recipient}" and the URL carries that address'))
def address_card_shows(world: World, recipient: str) -> None:
    expect(world.page.get_by_text(recipient, exact=False).first).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    addresses = world.service_factory.address.list_addresses().get("addresses", [])
    chosen = next(a["id"] for a in addresses if a.get("recipientName") == recipient)
    world.page.wait_for_url(
        re.compile(rf".*[?&]addr={re.escape(chosen)}\b"), timeout=timeouts.DEFAULT
    )


@when("the buyer presses the Down arrow on the selected payment method")
def buyer_presses_down(world: World) -> None:
    checkout = _checkout(world)
    world.state.extra["pay_before"] = checkout.checked_payment_radio.get_attribute("value")
    checkout.checked_payment_radio.focus()
    world.page.keyboard.press("ArrowDown")


@then("exactly one payment method is selected and the URL pay parameter follows it")
def one_payment_selected(world: World) -> None:
    checkout = _checkout(world)
    expect(checkout.checked_payment_radio).to_have_count(1, timeout=timeouts.DEFAULT)
    chosen = checkout.checked_payment_radio.get_attribute("value")
    assert chosen != world.state.extra["pay_before"], "Down arrow did not move the selection"
    world.page.wait_for_url(re.compile(rf".*[?&]pay={chosen}\b"), timeout=timeouts.DEFAULT)


# ── Placing an order: double submit and saga failure ─────────────────────
@when("the buyer double-clicks the place order button")
def buyer_double_clicks_place_order(world: World) -> None:
    checkout = _checkout(world)
    expect(checkout.place_order_button).to_be_enabled(timeout=timeouts.DEFAULT)
    checkout.place_order_button.dblclick()


@then("exactly one order exists for the buyer")
def exactly_one_order(world: World) -> None:
    world.page.wait_for_url(
        re.compile(r".*/(account/orders|checkout/pay).*"), timeout=timeouts.NAVIGATION
    )
    # the order saga is asynchronous: poll the list briefly before asserting
    deadline = time.monotonic() + 10
    orders: list = []
    while time.monotonic() < deadline:
        orders = world.service_factory.order.list_buyer_orders().get("orders", [])
        if orders:
            break
        time.sleep(0.5)
    assert len(orders) == 1, f"expected exactly one order, got {len(orders)}"


@given("another buyer exhausts the listing stock")
def another_buyer_exhausts_stock(world: World) -> None:
    sf = world.service_factory
    listing = world.state.listing
    stock = listing.stock if listing and listing.stock else 100
    buyer_token = sf._token if hasattr(sf, "_token") else None  # noqa: SLF001 - restore below
    other = fake.unique_username("promo_other_buyer")
    token = sf.auth.register(other, SETTINGS.seed_password, "buyer")
    sf.set_token(token)
    sf.address.create_address(
        recipient_name="Le Van C",
        phone="0911111111",
        street="1 Tran Phu",
        city="Ha Noi",
        is_default=True,
    )
    sf.cart.add_to_cart(listing_id=listing.listing_id, quantity=stock)
    sf.order.create_order({"paymentMethod": "PAYMENT_METHOD_COD"})
    if buyer_token:
        sf.set_token(buyer_token)


def _document_top(locator) -> float:  # noqa: ANN001
    """Top of the element in document coordinates: clicking scrolls the page, which changes
    its viewport-relative `bounding_box` without the layout having shifted."""
    return locator.evaluate("el => el.getBoundingClientRect().top + window.scrollY")


@when("the buyer places the order in the browser")
def buyer_places_order_in_browser(world: World) -> None:
    checkout = _checkout(world)
    expect(checkout.place_order_button).to_be_enabled(timeout=timeouts.DEFAULT)
    world.state.extra["cta_top"] = _document_top(checkout.place_order_button)
    checkout.place_order_button.click()


@then("an error alert with retry and back-to-cart actions is shown")
def saga_alert_shown(world: World) -> None:
    checkout = _checkout(world)
    expect(checkout.saga_alert).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(checkout.alert_retry_button).to_be_visible()
    expect(checkout.alert_back_to_cart_link).to_have_attribute("href", "/cart")


@then("the place order button is enabled again and did not move")
def cta_unmoved(world: World) -> None:
    checkout = _checkout(world)
    expect(checkout.place_order_button).to_be_enabled(timeout=timeouts.DEFAULT)
    before = world.state.extra["cta_top"]
    after = _document_top(checkout.place_order_button)
    assert abs(before - after) < 1, f"place order button moved from {before} to {after}"


@then("the cart still holds the same item")
def cart_unchanged(world: World) -> None:
    items = (world.service_factory.cart.get_cart().get("cart") or {}).get("items", [])
    assert len(items) == 1, items


# ── Payment page outcomes ────────────────────────────────────────────────
@given("a buyer has an order awaiting mock payment")
def buyer_order_awaiting_payment(world: World) -> None:
    buyer = scenario_buyer(world)
    login_via_api(world, buyer)
    listing_id = world.state.listing.listing_id if world.state.listing else "listing_001"
    create_order_via_api(world, buyer, listing_id)
    res = world.service_factory.payment.create_payment(world.state.order_id)
    assert (res.get("transaction") or {}).get("id"), res


@when("the buyer opens the payment page for the order")
def buyer_opens_payment_page(world: World) -> None:
    world.navigate_to(PageName.PAYMENT_RESULT, order_id=world.state.order_id)
    expect(_payment(world).simulate_success_button).to_be_visible(timeout=timeouts.NAVIGATION)


@when(parsers.parse('the buyer presses "{label}" on the payment page'))
def buyer_presses_payment_button(world: World, label: str) -> None:
    payment = _payment(world)
    button = (
        payment.simulate_success_button
        if label == "Thanh toán thành công"
        else payment.simulate_failure_button
    )
    # The buttons are server-rendered and visible before React hydrates them; a
    # click before hydration is dropped, so wait for the handler first.
    payment.wait_until_interactive(button)
    button.click()


@then("a success result links to the order list and to continue shopping")
def payment_success_result(world: World) -> None:
    payment = _payment(world)
    expect(payment.success_result).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(payment.view_orders_link).to_have_attribute("href", "/account/orders")
    expect(payment.continue_shopping_link).to_have_attribute("href", "/")


@then("an error result offers retry and change of payment method")
def payment_error_result(world: World) -> None:
    payment = _payment(world)
    expect(payment.error_result).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(payment.retry_button).to_be_visible()
    expect(payment.change_method_link).to_be_visible()


# ── Mobile layout ────────────────────────────────────────────────────────
@when("the buyer opens the cart at 375px width")
def buyer_opens_cart_mobile(world: World) -> None:
    world.page.set_viewport_size(_MOBILE)
    world.navigate_to(PageName.CART)
    expect(_cart(world).shop_groups.first).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the cart has no horizontal overflow and the quantity control and buy button are visible")
def cart_mobile_ok(world: World) -> None:
    cart = _cart(world)
    assert cart.horizontal_overflow() <= 0, "cart overflows horizontally at 375px"
    expect(cart.increase_quantity_button).to_be_visible()
    expect(cart.buy_button).to_be_visible()


# ── Analytics hooks (GA4 dataLayer, unchanged payloads) ───────────────────
def _datalayer_count(world: World, event: str) -> int:
    return world.page.evaluate(
        "(name) => (window.dataLayer || []).filter((e) => e.event === name).length", event
    )


@then(parsers.parse('the page analytics dataLayer holds exactly {count:d} "{event}" event'))
def datalayer_exactly(world: World, count: int, event: str) -> None:
    got = _datalayer_count(world, event)
    assert got == count, f"expected {count} {event} event(s), got {got}"


@then(parsers.parse('the page analytics dataLayer holds at least {count:d} "{event}" event'))
def datalayer_at_least(world: World, count: int, event: str) -> None:
    got = _datalayer_count(world, event)
    assert got >= count, f"expected >= {count} {event} event(s), got {got}"
