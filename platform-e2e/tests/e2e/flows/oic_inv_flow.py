"""Seeding and observation helpers for the order/inventory correctness scenarios.

Everything goes through the gateway (Connect JSON): a seller's listing, a buyer with an
address and a cart, checkout, cancel, and the listing's stock read back with GetListing.
Reservation RPCs are unreachable at the edge, so stock is the observable.
"""

from __future__ import annotations

import time
import uuid
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

from src.api.services import (
    AddressService,
    AuthService,
    CartService,
    ListingService,
    OrderService,
)
from src.models import Listing, User

PASSWORD = "pass123"


@dataclass
class Actor:
    token: str
    user: User

    @property
    def id(self) -> str:
        return self.user.user_id


@dataclass
class Fixture:
    """Per-scenario bag kept in `world.state.extra["oic_inv"]`."""

    sellers: list[Actor] = field(default_factory=list)
    listings: list[str] = field(default_factory=list)
    stock_before: dict[str, int] = field(default_factory=dict)
    buyer: Actor | None = None
    orders: list[dict[str, Any]] = field(default_factory=list)
    checkout_error: Exception | None = None
    started_ms: int = field(default_factory=lambda: int(time.time() * 1000) - 5_000)


def fixture(world) -> Fixture:
    return world.state.extra.setdefault("oic_inv", Fixture())


def _svc(world, cls, token: str | None):
    svc = cls(token=token)
    world.add_cleanup(svc.close)
    return svc


def register(world, role: str) -> Actor:
    name = f"oic_{role}_{uuid.uuid4().hex[:10]}"
    token = _svc(world, AuthService, None).register(name, PASSWORD, role)
    return Actor(token=token, user=User(username=name, password=PASSWORD, role=role, token=token))


def create_listing(world, seller: Actor, stock: int, title: str = "oic-inv") -> str:
    listing = Listing(title=f"{title}-{uuid.uuid4().hex[:6]}", stock=stock)
    listing_id = _svc(world, ListingService, seller.token).create_listing(listing)
    assert listing_id, "listing was not created"
    fx = fixture(world)
    fx.listings.append(listing_id)
    fx.stock_before[listing_id] = stock
    return listing_id


def stock_of(world, listing_id: str) -> int:
    """Stock as the gateway reports it (proto3 JSON omits a zero stock)."""
    listing = _svc(world, ListingService, None).get_listing(listing_id)
    return int(listing.get("stock", 0))


def make_buyer(world) -> Actor:
    buyer = register(world, "buyer")
    _svc(world, AddressService, buyer.token).create_address(
        recipient_name="Nguyen Van A",
        phone="0912345678",
        street="29 Lieu Giai",
        city="Ha Noi",
        ward="Phuong Lieu Giai",
        district="Quan Ba Dinh",
        is_default=True,
    )
    fixture(world).buyer = buyer
    return buyer


def fill_cart(world, buyer: Actor, lines: list[tuple[str, int]]) -> None:
    cart = _svc(world, CartService, buyer.token)
    cart.clear_cart()
    for listing_id, quantity in lines:
        cart.add_to_cart(listing_id, quantity)


def checkout(world, buyer: Actor) -> list[dict[str, Any]]:
    """CreateOrder (COD) for the buyer's whole cart; returns the created orders."""
    res = _svc(world, OrderService, buyer.token).create_order(
        {"paymentMethod": "PAYMENT_METHOD_COD"}
    )
    orders = res.get("orders", [])
    assert orders, f"CreateOrder returned no orders: {res}"
    fixture(world).orders = orders
    return orders


def order_status(world, buyer: Actor, order_id: str) -> str:
    order = _svc(world, OrderService, buyer.token).get_order(order_id).get("order", {})
    return order.get("status", "")


def buyer_orders(world, buyer: Actor) -> list[dict[str, Any]]:
    return _svc(world, OrderService, buyer.token).list_buyer_orders().get("orders", [])


def poll(check: Callable[[], Any], timeout_s: float = 20.0, interval_s: float = 0.5) -> Any:
    """Return the first truthy `check()`; the last value when the deadline passes."""
    deadline = time.monotonic() + timeout_s
    value = check()
    while not value and time.monotonic() < deadline:
        time.sleep(interval_s)
        value = check()
    return value
