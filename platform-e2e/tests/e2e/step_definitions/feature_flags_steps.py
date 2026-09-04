"""Checkout kill-switch (OpenFeature + Flipt) step definitions (wire-openfeature).

Toggles `checkout-enabled` through Flipt's API and asserts the effect end-to-end:
OFF hides the UI checkout entry and makes team-order reject CreateOrder with a
FailedPrecondition; ON restores it — no redeploy. Flags are evaluated server-side
only, so the browser must never receive a flag SDK or the Flipt address.
"""

from __future__ import annotations

from urllib.parse import urlparse

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services import GatewayError
from src.constants import PageName, timeouts
from src.models import User
from tests.e2e.flows import login_via_api, set_checkout_flag
from tests.e2e.support.world import World


def _buyer(world: World) -> User:
    buyer = world.state.current_user or world.state.extra.get("seeded_buyer")
    assert buyer is not None, "no buyer in scenario state"
    return buyer


def _as_buyer(world: World) -> User:
    buyer = _buyer(world)
    world.service_factory.set_token(buyer.token)
    return buyer


def _try_place_order(world: World) -> None:
    _as_buyer(world)
    try:
        world.service_factory.order.create_order()
        world.state.extra["order_error"] = None
    except GatewayError as exc:
        world.state.extra["order_error"] = exc


# ── Flag toggles (Given/When/Then all flip the same real switch) ──────────
@given(parsers.parse('"{flag}" is turned ON in Flipt'))
@when(parsers.parse('"{flag}" is turned ON in Flipt'))
def flag_on(world: World, flag: str) -> None:
    set_checkout_flag(True)
    world.logger.info(f"Flipt: {flag} -> ON")


@when(parsers.parse('"{flag}" is turned OFF in Flipt'))
def flag_off(world: World, flag: str) -> None:
    set_checkout_flag(False)
    world.logger.info(f"Flipt: {flag} -> OFF")


@then(parsers.parse('checkout is restored by turning "{flag}" back ON in Flipt'))
def flag_restore_on(world: World, flag: str) -> None:
    set_checkout_flag(True)


# ── Cart / order preconditions ───────────────────────────────────────────
@given("a buyer has an item in the cart")
def buyer_has_cart_item(world: World) -> None:
    buyer = world.state.extra.get("seeded_buyer") or _buyer(world)
    login_via_api(world, buyer)  # sets the buyer token + session cookie
    listing = world.state.listing
    assert listing and listing.listing_id, "no seeded listing to add to the cart"
    world.service_factory.cart.add_to_cart(listing_id=listing.listing_id, quantity=1)


@when("the buyer opens the cart page")
def open_cart_page(world: World) -> None:
    world.navigate_to(PageName.CART)


# ── Order placement ──────────────────────────────────────────────────────
@when("the buyer places the order through the gateway")
def buyer_places_order_gateway(world: World) -> None:
    _try_place_order(world)


@then("the order is accepted by team-order")
def order_accepted(world: World) -> None:
    err = world.state.extra.get("order_error")
    assert err is None, f"CreateOrder should succeed when checkout-enabled is ON, got {err}"


@then("the buyer can place the order through the gateway to team-order")
def buyer_can_place_order(world: World) -> None:
    _try_place_order(world)
    err = world.state.extra.get("order_error")
    assert err is None, f"CreateOrder should succeed when checkout-enabled is ON, got {err}"


@then("a CreateOrder request through the gateway is rejected as checkout-unavailable")
def create_order_rejected(world: World) -> None:
    _try_place_order(world)
    err = world.state.extra.get("order_error")
    assert isinstance(err, GatewayError), "CreateOrder should be rejected when checkout is OFF"
    body = (err.body or "").lower()
    assert (
        "unavailable" in body
        or "checkout" in body
        or "failedprecondition" in body.replace("_", "")
        or err.status in (400, 409, 412, 422, 500, 503)
    ), f"rejection should signal checkout-unavailable, got HTTP {err.status}: {body[:200]}"


# ── UI gating ────────────────────────────────────────────────────────────
@then("the checkout entry point is hidden in the UI and a checkout-unavailable notice is shown")
def checkout_hidden_with_notice(world: World) -> None:
    cart = world.get_page(PageName.CART)
    expect(cart.checkout_unavailable_notice).to_be_visible(timeout=timeouts.DEFAULT)  # type: ignore[attr-defined]
    expect(cart.checkout_link).to_have_count(0)  # type: ignore[attr-defined]


@then("the checkout entry point is shown in the UI")
def checkout_shown(world: World) -> None:
    cart = world.get_page(PageName.CART)
    expect(cart.checkout_link).to_be_visible(timeout=timeouts.DEFAULT)  # type: ignore[attr-defined]


@then("the browser receives resolved markup with no Flipt address or flag SDK")
def browser_has_no_flag_sdk(world: World) -> None:
    content = world.page.content().lower()
    # Full host:port of the Flipt endpoint (e.g. localhost:8080 / flipt:8080) — specific
    # enough not to collide with the frontend origin (:3000).
    flipt_netloc = (urlparse(world.settings.flipt_url).netloc or "flipt").lower()
    assert "flipt" not in content, "Flipt SDK/marker leaked into the browser markup"
    assert flipt_netloc not in content, "Flipt address leaked into the browser markup"
    assert "openfeature" not in content, "a flag SDK leaked into the browser markup"
