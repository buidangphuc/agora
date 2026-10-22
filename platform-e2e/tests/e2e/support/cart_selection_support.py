"""Helpers of frontend/cart_selection_ui.feature (change frontend-cart-selection).

Black box: shops, listings, buyers and carts are seeded through the gateway; cart line ids and
placed orders are read back from the gateway, never from the page under test.
"""

from __future__ import annotations

import time
import uuid
from typing import Any

from src.api.services import ListingService
from src.models import Listing
from tests.e2e.support import checkout_coherence_support as cc
from tests.e2e.support import d2_support as d2
from tests.e2e.support.world import World


def ctx(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("cs", {})


def extra_listing(shop: dict[str, Any], price: int = 200_000) -> dict[str, Any]:
    """A second published listing of the same seller."""
    listing = Listing(
        title=f"[E2E][cs] {uuid.uuid4().hex[:8]}",
        category_id="cat-electronics",
        price=price,
        stock=50,
        status="published",
        description="Seed for the cart selection UI e2e.",
    )
    svc = ListingService(token=shop["token"])
    try:
        svc.create_listing(listing)
    finally:
        svc.close()
    return {**shop, "listing_id": listing.listing_id, "title": listing.title, "price": price}


def cart_items(token: str) -> list[dict[str, Any]]:
    svc = d2.CartService(token=token)
    try:
        return (svc.get_cart().get("cart") or {}).get("items", [])
    finally:
        svc.close()


def line_id(token: str, listing_id: str, timeout: float = 15.0) -> str:
    deadline = time.monotonic() + timeout
    found: list[dict[str, Any]] = []
    while time.monotonic() < deadline:
        found = [i for i in cart_items(token) if i.get("listingId") == listing_id]
        if found:
            break
        time.sleep(0.5)
    assert len(found) == 1, (listing_id, found)
    return found[0]["id"]


def add(token: str, listing_id: str, quantity: int = 1) -> None:
    svc = d2.CartService(token=token)
    try:
        svc.add_to_cart(listing_id, quantity)
    finally:
        svc.close()


def seed_three_lines(world: World) -> dict[str, Any]:
    """Buyer (browser logged in, default address) with lines A1, A2 (shop A) and B1 (shop B)."""
    user = d2.seed_buyer(world, address_city="Đà Nẵng")
    shop_a = d2.seed_shop(price=300_000, stock=50)
    a2 = extra_listing(shop_a, 200_000)
    shop_b = d2.seed_shop(price=100_000, stock=50)
    for shop in (shop_a, a2, shop_b):
        add(user.token, shop["listing_id"])
    c = ctx(world)
    c.update(
        buyer=user,
        shop_a=shop_a,
        a2=a2,
        shop_b=shop_b,
        ids={
            "A1": line_id(user.token, shop_a["listing_id"]),
            "A2": line_id(user.token, a2["listing_id"]),
            "B1": line_id(user.token, shop_b["listing_id"]),
        },
    )
    c["titles"] = {"A1": shop_a["title"], "A2": a2["title"], "B1": shop_b["title"]}
    return c


def wait_orders(token: str, count: int, timeout: float = 25.0) -> list[dict[str, Any]]:
    deadline = time.monotonic() + timeout
    orders: list[dict[str, Any]] = []
    while time.monotonic() < deadline:
        orders = cc.buyer_orders(token)
        if len(orders) >= count:
            break
        time.sleep(0.5)
    assert len(orders) == count, orders
    return orders
