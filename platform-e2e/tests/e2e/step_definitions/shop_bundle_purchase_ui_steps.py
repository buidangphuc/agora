"""Steps of frontend/shop_bundle_purchase_ui.feature (change shop-bundle-purchase-ui).

Black box: sellers, listings, combos and carts are seeded through the real gateway API; the buyer
signs in through the login form; the browser drives /shop/{id}, /cart and /checkout.
"""

from __future__ import annotations

import re
import time
import uuid
from typing import Any

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from tests.e2e.flows.auth_flow import login_via_ui
from tests.e2e.step_definitions.d2_checkout_steps import summary_rows
from tests.e2e.support.oic_order_support import (
    CART,
    LISTING,
    OicWorld,
    ensure_address,
    ok,
    post,
    register,
)
from tests.e2e.support.world import World

_SYNC_WAIT_S = 60
_ADD = "Thêm combo vào giỏ"
_UNAVAILABLE = "Combo tạm không khả dụng"
_KEY = "bsp"


def _api(world: World) -> OicWorld:
    return world.state.extra.setdefault(f"{_KEY}_api", OicWorld())


def _combo(world: World) -> dict[str, Any]:
    return world.state.extra[f"{_KEY}_combo"]


def _new_listing(w: OicWorld, seller, price: int) -> dict[str, str]:
    title = f"[BSP] {uuid.uuid4().hex[:8]}"
    resp = post(
        w, seller, LISTING, "CreateListing",
        {"listing": {
            "title": title, "categoryId": "cat-electronics", "price": price, "stock": 50,
            "status": "LISTING_STATUS_PUBLISHED", "currency": "VND",
            "description": "shop bundle purchase e2e",
        }},
    )  # fmt: skip
    listing_id = (ok(resp).get("listing") or {}).get("id", "")
    assert listing_id, resp.text
    return {"id": listing_id, "title": title}


def _cart_items(world: World) -> list[dict[str, Any]]:
    buyer = _api(world).actors["buyer"]
    return (ok(post(_api(world), buyer, CART, "GetCart", {})).get("cart") or {}).get("items") or []


def _goto_shop(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/shop/{_combo(world)['seller_id']}")
    world.page.wait_for_load_state("networkidle")


def _open_shop_until(world: World, ready) -> None:  # noqa: ANN001
    deadline = time.monotonic() + _SYNC_WAIT_S
    while True:
        _goto_shop(world)
        if ready():
            return
        assert time.monotonic() < deadline, "the shop page never showed the expected combo state"
        time.sleep(3)


@given(
    parsers.parse(
        "a bsp seller with a published combo of listings priced {a:d} and {b:d} at combo price {p:d}"
    )
)
def seller_combo(world: World, a: int, b: int, p: int) -> None:
    w = _api(world)
    seller = register(w, "seller", "seller")
    listings = [_new_listing(w, seller, a), _new_listing(w, seller, b)]
    resp = post(
        w, seller, LISTING, "CreateBundle",
        {"title": f"combo {uuid.uuid4().hex[:6]}", "listingIds": [x["id"] for x in listings],
         "bundlePrice": p},
    )  # fmt: skip
    bundle_id = (ok(resp).get("bundle") or {}).get("id", "")
    assert bundle_id, resp.text
    world.state.extra[f"{_KEY}_combo"] = {
        "id": bundle_id,
        "listings": listings,
        "seller": seller,
        "seller_id": seller.user_id,
    }


@given("a bsp buyer with a saved address signs in through the UI")
def buyer_signs_in(world: World) -> None:
    w = _api(world)
    buyer = register(w, "buyer", "buyer")
    ensure_address(w, buyer)
    login_via_ui(world, buyer.username, world.settings.seed_password)
    world.page.wait_for_url(lambda url: "/login" not in url, timeout=timeouts.NAVIGATION)


@given("the bsp buyer has the combo in the cart")
def buyer_has_combo(world: World) -> None:
    resp = post(
        _api(world), _api(world).actors["buyer"], CART, "AddBundleToCart",
        {"bundleId": _combo(world)["id"], "quantity": 1},
    )  # fmt: skip
    ok(resp)


@given("the second listing of the bsp combo is unpublished")
def unpublish_second(world: World) -> None:
    w, combo = _api(world), _combo(world)
    listing_id = combo["listings"][1]["id"]
    current = ok(post(w, None, LISTING, "GetListing", {"id": listing_id}))["listing"]
    current["status"] = "LISTING_STATUS_DRAFT"
    ok(post(w, combo["seller"], LISTING, "UpdateListing", {"listing": current}))


@when("the bsp buyer opens the seller's shop page once the combo can be added")
def open_shop_addable(world: World) -> None:
    button = world.page.get_by_role("button", name=_ADD)
    _open_shop_until(world, lambda: button.count() > 0)


@when("a bsp visitor opens the seller's shop page once the combo shows unavailable")
def open_shop_unavailable(world: World) -> None:
    text = world.page.get_by_text(_UNAVAILABLE)
    _open_shop_until(world, lambda: text.count() > 0)


@when(parsers.parse('the bsp buyer clicks "{label}"'))
def click_button(world: World, label: str) -> None:
    world.page.get_by_role("button", name=label).first.click()


@when("the bsp buyer opens the cart")
def open_cart(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/cart", wait_until="domcontentloaded")
    world.page.wait_for_load_state("networkidle")


@when("the bsp buyer opens the checkout page")
def open_checkout(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/checkout", wait_until="domcontentloaded")
    world.page.wait_for_load_state("networkidle")


@when("the bsp buyer removes the first combo line")
def remove_first_line(world: World) -> None:
    remove = world.page.get_by_role("button", name=re.compile(r"^(Xóa|Xoá)(?! tất cả)")).first
    expect(remove).to_be_visible(timeout=timeouts.NAVIGATION)
    remove.click()


@then(parsers.parse('the bsp page shows "{text}" with a link to "{href}"'))
def shows_added(world: World, text: str, href: str) -> None:
    expect(world.page.get_by_text(text).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.locator(f'a[href="{href}"]').filter(visible=True).first).to_be_visible()


@then("the bsp buyer's cart holds one line per combo listing, each tagged with the combo")
def cart_tagged(world: World) -> None:
    combo = _combo(world)
    items = _cart_items(world)
    assert sorted(i["listingId"] for i in items) == sorted(x["id"] for x in combo["listings"]), (
        items
    )
    assert all(i.get("bundleId") == combo["id"] for i in items), items


@then(parsers.parse('the bsp combo card shows "{unavailable}" and has no "{add}" button'))
def card_unavailable(world: World, unavailable: str, add: str) -> None:
    expect(world.page.get_by_text(unavailable).first).to_be_visible(timeout=timeouts.NAVIGATION)
    assert world.page.get_by_role("button", name=add).count() == 0


@then('the bsp cart shows both combo lines under one "Combo" label')
def cart_one_label(world: World) -> None:
    page = world.page
    label = page.get_by_text("Combo", exact=True)
    expect(label).to_have_count(1, timeout=timeouts.NAVIGATION)
    for listing in _combo(world)["listings"]:
        expect(page.get_by_text(listing["title"]).first).to_be_visible()


@then(parsers.parse('the bsp cart summary shows "Giảm giá combo" with {amount:d}'))
def cart_discount_row(world: World, amount: int) -> None:
    summary = world.page.get_by_test_id("order-summary")
    expect(summary).to_contain_text("Giảm giá combo", timeout=timeouts.NAVIGATION)
    flat = summary.inner_text().replace("\n", " ")
    digits = re.sub(r"\D", "", flat.split("Giảm giá combo", 1)[1])[: len(str(amount))]
    assert digits == str(amount), flat


@then(parsers.parse('the bsp cart shows "{text}"'))
def cart_shows(world: World, text: str) -> None:
    expect(world.page.get_by_text(text).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then('the bsp cart has no "Combo" label left')
def cart_no_label(world: World) -> None:
    expect(world.page.get_by_text("Combo", exact=True)).to_have_count(0)


@then(
    parsers.parse(
        'the bsp checkout summary shows "Giảm giá combo" with {amount:d} and a merchandise total of {total:d}'
    )
)
def checkout_summary(world: World, amount: int, total: int) -> None:
    summary = world.page.get_by_test_id("order-summary")
    expect(summary).to_contain_text("Giảm giá combo", timeout=timeouts.NAVIGATION)
    flat = summary.inner_text().replace("\n", " ")
    got = re.sub(r"\D", "", flat.split("Giảm giá combo", 1)[1])[: len(str(amount))]
    assert got == str(amount), flat
    assert summary_rows(world.page)["subtotal"] == total, flat
