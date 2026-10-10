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
from src.constants import gateway_endpoints as ep
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
    image: bool = False,
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
    if image:
        # a stored key resolves to a media URL, so the page renders a real <img>
        res = svc.post(
            ep.LISTING_CREATE,
            {
                "listing": {
                    "title": listing.title,
                    "categoryId": listing.category_id,
                    "price": price,
                    "stock": stock,
                    "status": "LISTING_STATUS_PUBLISHED",
                    "currency": listing.currency,
                    "description": listing.description,
                    "imageKeys": ["d2/thumb.png"],
                }
            },
        )
        listing.listing_id = res["listing"]["id"]
    else:
        svc.create_listing(listing)
    return {
        "username": username,
        "token": token,
        "seller_id": principal_id(token),
        "listing_id": listing.listing_id,
        "title": listing.title,
        "price": price,
        "stock": stock,
        "name": name,
    }


def seed_buyer(world: World, *, address_city: str | None = "Hà Nội") -> User:
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


def place_order(world: World, shop: dict[str, Any], quantity: int = 1) -> str:
    """Create a COD order for the logged-in d2 buyer from `shop`; returns the order id."""
    add_to_cart(world, shop, quantity)
    res = buyer_orders(world).create_order({"paymentMethod": "PAYMENT_METHOD_COD"})
    orders = res.get("orders", [])
    order_id = orders[0]["id"] if orders else res["order"]["id"]
    world.state.extra["d2_order_id"] = order_id
    return order_id


def force_fail_payment(order_id: str) -> None:
    """ForceFailSaga is admin-only: act as the seeded admin."""
    from src.api.services import OrderService
    from src.utils import get_test_data_manager

    admin = get_test_data_manager().get_user_by_role("admin")
    token = AuthService().login(admin.username, admin.password)
    OrderService(token=token).force_fail_saga(order_id)


def order_statuses(world: World) -> dict[str, str]:
    return {
        o["id"]: o.get("status", "")
        for o in buyer_orders(world).list_buyer_orders().get("orders", [])
    }


def ship_order(shop: dict[str, Any], order_id: str, *, complete: bool = False) -> None:
    """Ship (and optionally complete) an order as its seller."""
    svc = OrderService(token=shop["token"])
    svc.update_order_status(order_id, "ORDER_STATUS_SHIPPED")
    if complete:
        svc.update_order_status(order_id, "ORDER_STATUS_COMPLETED")


def seed_seller_session(
    world: World, *, listings: int = 0, price: int = 100_000, stock: int = 50, image: bool = False
) -> dict[str, Any]:
    """Register a seller (with `listings` published listings), log the browser in as them."""
    username, token = register("seller")
    svc = ListingService(token=token)
    items = []
    for i in range(listings):
        listing = Listing(
            title=f"[E2E][d2] seller item {i:02d} {uuid.uuid4().hex[:6]}",
            category_id="cat-electronics",
            price=price,
            stock=stock,
            status="published",
            description="Seed for the d2 seller UI e2e.",
        )
        if image:
            res = svc.post(
                ep.LISTING_CREATE,
                {
                    "listing": {
                        "title": listing.title,
                        "categoryId": listing.category_id,
                        "price": price,
                        "stock": stock,
                        "status": "LISTING_STATUS_PUBLISHED",
                        "currency": listing.currency,
                        "description": listing.description,
                        "imageKeys": ["d2/thumb.png"],
                    }
                },
            )
            listing.listing_id = res["listing"]["id"]
        else:
            svc.create_listing(listing)
        items.append({"id": listing.listing_id, "title": listing.title, "stock": stock})
    world.context.add_cookies(
        [{"name": SESSION_COOKIE, "value": token, "url": world.settings.base_url}]
    )
    world.service_factory.set_token(token)
    seller = {
        "username": username,
        "token": token,
        "seller_id": principal_id(token),
        "listings": items,
    }
    world.state.extra["d2_seller"] = seller
    return seller


def seller_order(world: World, seller: dict[str, Any], *, listing_index: int = 0) -> str:
    """A buyer's COD order for one of the seller's listings; returns the order id."""
    buyer = seed_buyer_token()
    item = seller["listings"][listing_index]
    CartService(token=buyer["token"]).add_to_cart(item["id"], 1)
    res = OrderService(token=buyer["token"]).create_order({"paymentMethod": "PAYMENT_METHOD_COD"})
    orders = res.get("orders", [])
    order_id = orders[0]["id"] if orders else res["order"]["id"]
    world.state.extra["d2_order_id"] = order_id
    world.state.extra["d2_buyer_token"] = buyer["token"]
    return order_id


def seed_buyer_token() -> dict[str, str]:
    username, token = register("buyer")
    AddressService(token=token).create_address(
        recipient_name="Nguyen Van A",
        phone="0912345678",
        street="29 Lieu Giai",
        city="Hà Nội",
        ward="Phuong Lieu Giai",
        district="Quan Ba Dinh",
        is_default=True,
    )
    return {"username": username, "token": token}
