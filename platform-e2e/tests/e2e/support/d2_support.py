"""Seeding helpers for the ui-phase-cart-checkout / -orders / -seller e2e (area d2).

Everything is seeded through the gateway API by the principal the browser is then logged in
as, so the server-rendered pages show the seeded state. No real payment data is involved:
orders use COD or the mock payment.
"""

from __future__ import annotations

import base64
import json
import uuid
from typing import Any

from config.settings import get_settings
from src.api.services import AddressService, AuthService, CartService, ListingService, OrderService
from src.models import Listing, User
from src.utils import data as fake
from tests.e2e.flows.auth_flow import SESSION_COOKIE
from tests.e2e.support.world import World

SETTINGS = get_settings()


def principal_id(token: str) -> str:
    """The `sub` claim (principal id) of a gateway JWT."""
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload)).get("sub", "")


def register(role: str, prefix: str | None = None) -> tuple[str, str]:
    """Register a fresh account; returns (username, token)."""
    username = fake.unique_username(prefix or f"d2_{role}")
    token = AuthService().register(username, SETTINGS.seed_password, role)
    return username, token


def seed_shop(
    name: str | None = None,
    *,
    price: int = 100_000,
    stock: int = 50,
    title: str | None = None,
) -> dict[str, Any]:
    """A seller (optionally with a shop display name) with one published listing."""
    username, token = register("seller")
    svc = ListingService(token=token)
    if name:
        svc.upsert_storefront(f"d2-shop-{uuid.uuid4().hex[:10]}", name)
    listing = Listing(
        title=title or f"[E2E][d2] {uuid.uuid4().hex[:8]}",
        category_id="cat-electronics",
        price=price,
        stock=stock,
        status="published",
        description="Seed for the d2 UI e2e.",
    )
    svc.create_listing(listing)
    return {
        "username": username,
        "token": token,
        "seller_id": principal_id(token),
        "listing_id": listing.listing_id,
        "title": listing.title,
        "price": price,
        "name": name,
    }


def seed_buyer(world: World, *, address_city: str | None = "Ha Noi") -> User:
    """Register a buyer (with an optional default address) and log the browser in as them."""
    username, token = register("buyer")
    if address_city:
        AddressService(token=token).create_address(
            recipient_name="Nguyen Van A",
            phone="0912345678",
            street="29 Lieu Giai",
            city=address_city,
            ward="Phuong Lieu Giai",
            district="Quan Ba Dinh",
            is_default=True,
        )
    user = User(username=username, password=SETTINGS.seed_password, role="buyer", token=token)
    world.context.add_cookies(
        [{"name": SESSION_COOKIE, "value": token, "url": world.settings.base_url}]
    )
    world.service_factory.set_token(token)
    world.state.current_user = user
    world.state.extra["d2_buyer"] = user
    return user


def buyer_cart(world: World) -> CartService:
    return CartService(token=world.state.extra["d2_buyer"].token)


def buyer_orders(world: World) -> OrderService:
    return OrderService(token=world.state.extra["d2_buyer"].token)


def add_to_cart(world: World, shop: dict[str, Any], quantity: int = 1) -> None:
    buyer_cart(world).add_to_cart(shop["listing_id"], quantity)
