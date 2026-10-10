"""Steps for frontend/oic_inv_checkout_idempotency.feature (port-order-inventory-correctness).

The checkout is a Next.js server action: the browser POSTs the action to the storefront,
which calls the gateway's CreateOrder (with the Idempotency-Key header once the storefront
sends one). The replay records that exact browser request and sends it again unchanged.
The buyer's cart is refilled first: with an empty cart a replay would fail on its own, so
only the key (not the empty cart) can keep the second submission from creating an order.
"""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import PageName, timeouts
from src.pages import CheckoutPage
from tests.e2e.flows import login_via_api
from tests.e2e.flows.oic_inv_flow import (
    buyer_orders,
    create_listing,
    fill_cart,
    fixture,
    make_buyer,
    poll,
    register,
    stock_of,
)
from tests.e2e.support.world import World

_DROP_HEADERS = {"host", "content-length", "connection", "cookie", "accept-encoding"}
_AFTER_ORDER = re.compile(r".*/(account/orders|checkout/pay).*")


def _checkout(world: World) -> CheckoutPage:
    return world.get_page(PageName.CHECKOUT)  # type: ignore[return-value]


def _click_place_order(world: World) -> None:
    checkout = _checkout(world)
    expect(checkout.place_order_button).to_be_enabled(timeout=timeouts.DEFAULT)
    # The button is server-rendered and clickable before hydration, but the click is lost.
    checkout.wait_until_interactive(checkout.place_order_button)
    checkout.place_order_button.click()
    world.page.wait_for_url(_AFTER_ORDER, timeout=timeouts.NAVIGATION)


@given(
    parsers.parse(
        "a logged-in buyer with a saved address and one unit of a listing with stock {stock:d} in the cart"
    )
)
def buyer_with_cart(world: World, stock: int) -> None:
    seller = register(world, "seller")
    create_listing(world, seller, stock, "oic-checkout")
    buyer = make_buyer(world)
    fx = fixture(world)
    fill_cart(world, buyer, [(fx.listings[0], 1)])
    login_via_api(world, buyer.user)  # session cookie for the browser


@when("the buyer opens the checkout page")
def buyer_opens_checkout(world: World) -> None:
    world.navigate_to(PageName.CHECKOUT)
    expect(_checkout(world).stepper).to_be_visible(timeout=timeouts.NAVIGATION)


@when(parsers.parse('the buyer goes to the "{step}" checkout step'))
def buyer_goes_to_step(world: World, step: str) -> None:
    _checkout(world).continue_to(step)


@when("the buyer places the order on the checkout page recording the checkout submission")
def place_recording(world: World) -> None:
    recorded: list = []

    def on_request(request) -> None:  # noqa: ANN001
        if request.method == "POST" and "next-action" in request.headers:
            recorded.append(request)

    world.page.on("request", on_request)
    _click_place_order(world)
    world.page.remove_listener("request", on_request)
    assert recorded, "the checkout page sent no server-action request"
    address_ids = [
        a["id"] for a in world.service_factory.address.list_addresses().get("addresses", [])
    ]
    chosen = next(
        (
            r
            for r in recorded
            if any(a.encode() in (r.post_data_buffer or b"") for a in address_ids)
        ),
        recorded[0],
    )
    world.state.extra["oic_inv_submission"] = {
        "url": chosen.url,
        "headers": {
            k: v for k, v in chosen.all_headers().items() if k.lower() not in _DROP_HEADERS
        },
        "body": chosen.post_data_buffer or b"",
    }


@when("the buyer's cart holds the listing again")
def cart_refilled(world: World) -> None:
    fx = fixture(world)
    fill_cart(world, fx.buyer, [(fx.listings[0], 1)])


@when("the recorded checkout submission is sent a second time")
def replay_submission(world: World) -> None:
    sub = world.state.extra["oic_inv_submission"]
    resp = world.context.request.post(sub["url"], headers=sub["headers"], data=sub["body"])
    world.logger.info(f"replayed checkout submission -> HTTP {resp.status}")
    assert resp.status < 500, f"the replayed submission crashed the storefront: HTTP {resp.status}"


@when(
    "the buyer adds the listing to the cart again and places a second order from the checkout page"
)
def second_checkout(world: World) -> None:
    fx = fixture(world)
    fill_cart(world, fx.buyer, [(fx.listings[0], 1)])
    world.navigate_to(PageName.CHECKOUT)
    checkout = _checkout(world)
    expect(checkout.stepper).to_be_visible(timeout=timeouts.NAVIGATION)
    checkout.continue_to("confirm")
    _click_place_order(world)


@then("the buyer has exactly one new order and the listing's stock is reduced once")
def one_order_one_decrement(world: World) -> None:
    fx = fixture(world)
    orders = poll(lambda: buyer_orders(world, fx.buyer), timeout_s=10)
    assert (
        len(orders) == 1
    ), f"expected exactly one order, got {len(orders)}: {[o['id'] for o in orders]}"
    listing_id = fx.listings[0]
    stock = stock_of(world, listing_id)
    assert (
        stock == fx.stock_before[listing_id] - 1
    ), f"listing {listing_id}: stock {stock}, expected {fx.stock_before[listing_id] - 1}"


@then("the buyer has two distinct new orders")
def two_orders(world: World) -> None:
    fx = fixture(world)
    orders = poll(lambda: len(buyer_orders(world, fx.buyer)) >= 2 and buyer_orders(world, fx.buyer))
    ids = {o["id"] for o in orders or []}
    assert len(ids) == 2, f"expected two distinct orders, got {sorted(ids)}"
