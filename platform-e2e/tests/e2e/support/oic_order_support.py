"""Raw gateway client + helpers for port-order-inventory-correctness (order area).

Every call goes through the public edge (Connect JSON over the gateway). Unlike the
shared ``ServiceFactory`` services these helpers return the raw HTTP response so a
scenario can assert the Connect error code (``resource_exhausted`` ...) and send the
``Idempotency-Key`` header the gateway forwards. Actors are separate registered users
(one seller per listing, distinct buyers for A-vs-B), never shared accounts.
"""

from __future__ import annotations

import json
import os
import threading
import time
import uuid
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

import httpx

from config.settings import get_settings
from tests.e2e.flows.stack_flow import _docker

ORDER = "/platform.order.v1.OrderService"
CART = "/platform.order.v1.CartService"
LISTING = "/platform.listing.v1.ListingService"
PAYMENT = "/platform.payment.v1.PaymentService"
PROMO = "/platform.promotion.v1.VoucherService"
AUTH = "/platform.identity.v1.AuthService"
ADDRESS = "/platform.identity.v1.AddressService"

PENDING = "ORDER_STATUS_PENDING"
PAID = "ORDER_STATUS_PAID"
SHIPPED = "ORDER_STATUS_SHIPPED"
COMPLETED = "ORDER_STATUS_COMPLETED"
CANCELLED = "ORDER_STATUS_CANCELLED"

# Short-TTL overlay (platform-e2e/compose/order-inventory.override.yaml): RESERVATION_TTL=20s,
# RESERVATION_SWEEP_INTERVAL=2s. TTL scenarios wait TTL + 2 x interval + margin.
OVERLAY_TTL_S = float(os.getenv("OIC_RESERVATION_TTL_S", "20"))
OVERLAY_SWEEP_S = float(os.getenv("OIC_SWEEP_INTERVAL_S", "2"))
TTL_WAIT_S = OVERLAY_TTL_S + 2 * OVERLAY_SWEEP_S + 10


def domain_container() -> str:
    return os.getenv("DOMAIN_CONTAINER", "agora-team-domain-svc")


@dataclass
class Actor:
    username: str
    token: str
    role: str
    user_id: str = ""


@dataclass
class OicWorld:
    """Per-scenario bag: actors, listings and the last response of each kind."""

    gateway: str = field(default_factory=lambda: get_settings().gateway_url)
    actors: dict[str, Actor] = field(default_factory=dict)
    listings: dict[str, dict[str, Any]] = field(default_factory=dict)
    orders: dict[str, str] = field(default_factory=dict)
    responses: dict[str, httpx.Response] = field(default_factory=dict)
    data: dict[str, Any] = field(default_factory=dict)


def _sub(token: str) -> str:
    import base64

    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload)).get("sub", "")


def post(
    w: OicWorld,
    actor: Actor | None,
    service: str,
    method: str,
    body: dict | None = None,
    headers: dict[str, str] | None = None,
) -> httpx.Response:
    hdrs = {"Content-Type": "application/json"}
    if actor:
        hdrs["Authorization"] = f"bearer {actor.token}"
    if headers:
        hdrs.update(headers)
    return httpx.post(f"{w.gateway}{service}/{method}", json=body or {}, headers=hdrs, timeout=45)


def code_of(resp: httpx.Response) -> str:
    """Connect error code of a failed call (``ok`` for a 2xx)."""
    if resp.status_code < 400:
        return "ok"
    try:
        return str(resp.json().get("code", f"http_{resp.status_code}"))
    except ValueError:
        return f"http_{resp.status_code}"


def ok(resp: httpx.Response) -> dict:
    assert resp.status_code == 200, f"HTTP {resp.status_code}: {resp.text[:400]}"
    return resp.json() if resp.content else {}


# ── actors / catalogue ───────────────────────────────────────────────────
def register(w: OicWorld, name: str, role: str) -> Actor:
    username = f"oic_{role[:1]}{uuid.uuid4().hex[:10]}"
    resp = post(
        w,
        None,
        AUTH,
        "Register",
        {"username": username, "password": get_settings().seed_password, "role": role},
    )
    token = (ok(resp).get("result") or {}).get("token", "")
    assert token, f"register {role}: {resp.text[:300]}"
    actor = Actor(username, token, role, _sub(token))
    w.actors[name] = actor
    return actor


def login_admin(w: OicWorld) -> Actor:
    from src.utils import get_test_data_manager

    admin = get_test_data_manager().get_user_by_role("admin")
    resp = post(w, None, AUTH, "Login", {"username": admin.username, "password": admin.password})
    token = (ok(resp).get("result") or {}).get("token", "")
    actor = Actor(admin.username, token, "admin", _sub(token))
    w.actors["admin"] = actor
    return actor


def create_listing(
    w: OicWorld, name: str, seller: Actor, stock: int, price: int = 100_000
) -> dict[str, Any]:
    resp = post(
        w,
        seller,
        LISTING,
        "CreateListing",
        {
            "listing": {
                "title": f"[OIC] {name} {uuid.uuid4().hex[:6]}",
                "categoryId": "cat-electronics",
                "price": price,
                "stock": stock,
                "status": "LISTING_STATUS_PUBLISHED",
                "currency": "VND",
                "description": "oic order e2e",
            }
        },
    )
    listing = ok(resp).get("listing") or {}
    assert listing.get("id"), resp.text
    w.listings[name] = {"id": listing["id"], "seller": seller, "initial": stock}
    return w.listings[name]


def listing_stock(w: OicWorld, name: str) -> int:
    resp = post(w, None, LISTING, "GetListing", {"id": w.listings[name]["id"]})
    return int(ok(resp)["listing"].get("stock", 0))


def set_listing_stock(w: OicWorld, name: str, stock: int) -> None:
    """Seller restocks (full-replacement UpdateListing, as the seller UI does)."""
    entry = w.listings[name]
    cur = ok(post(w, None, LISTING, "GetListing", {"id": entry["id"]}))["listing"]
    cur["stock"] = stock
    ok(post(w, entry["seller"], LISTING, "UpdateListing", {"listing": cur}))


def wait_stock(
    w: OicWorld, name: str, expected: int, timeout: float = 30.0, stay: float = 0
) -> int:
    """Poll until the listing stock equals ``expected`` (then hold for ``stay`` seconds)."""
    deadline = time.monotonic() + timeout
    got = -1
    while time.monotonic() < deadline:
        got = listing_stock(w, name)
        if got == expected:
            break
        time.sleep(1)
    assert got == expected, f"listing {name} stock {got}, expected {expected}"
    end = time.monotonic() + stay
    while time.monotonic() < end:
        time.sleep(1)
        got = listing_stock(w, name)
        assert got == expected, f"listing {name} stock drifted to {got}, expected {expected}"
    return got


# ── cart / checkout ──────────────────────────────────────────────────────
def ensure_address(w: OicWorld, buyer: Actor) -> None:
    post(
        w,
        buyer,
        ADDRESS,
        "CreateAddress",
        {
            "recipientName": "Nguyen Van A",
            "phone": "0912345678",
            "street": "29 Lieu Giai",
            "city": "Ha Noi",
            "ward": "Phuong Lieu Giai",
            "district": "Quan Ba Dinh",
            "isDefault": True,
        },
    )


def add_to_cart(w: OicWorld, buyer: Actor, listing: str, qty: int) -> None:
    ok(post(w, buyer, CART, "AddToCart", {"listingId": w.listings[listing]["id"], "quantity": qty}))


def cart_items(w: OicWorld, buyer: Actor) -> list[dict]:
    cart = ok(post(w, buyer, CART, "GetCart", {})).get("cart") or {}
    return cart.get("items", [])


def checkout(
    w: OicWorld,
    buyer: Actor,
    key: str | None = None,
    voucher: str = "",
) -> httpx.Response:
    body: dict[str, Any] = {"paymentMethod": "PAYMENT_METHOD_COD"}
    if voucher:
        body["voucherCode"] = voucher
    headers = {"Idempotency-Key": key} if key else None
    return post(w, buyer, ORDER, "CreateOrder", body, headers)


def order_ids(resp: httpx.Response) -> list[str]:
    return sorted(o["id"] for o in ok(resp).get("orders", []))


def buyer_orders(w: OicWorld, buyer: Actor) -> list[dict]:
    return ok(post(w, buyer, ORDER, "ListBuyerOrders", {})).get("orders", [])


def get_order(w: OicWorld, actor: Actor, order_id: str) -> dict:
    return ok(post(w, actor, ORDER, "GetOrder", {"id": order_id})).get("order", {})


def order_status(w: OicWorld, actor: Actor, order_id: str) -> str:
    return get_order(w, actor, order_id).get("status", "")


def place_order(
    w: OicWorld, buyer: Actor, listing: str, qty: int, key: str | None = None, voucher: str = ""
) -> str:
    """One-listing checkout from a clean cart; returns the new order id."""
    post(w, buyer, CART, "ClearCart", {})
    ensure_address(w, buyer)
    add_to_cart(w, buyer, listing, qty)
    orders = ok(checkout(w, buyer, key, voucher)).get("orders", [])
    assert len(orders) == 1, orders
    return orders[0]["id"]


def put_in_status(w: OicWorld, order_id: str, buyer: Actor, seller: Actor, target: str) -> None:
    """Drive a fresh Pending order to ``target`` along legal paths only.

    Paid: online mock payment (settlement consumer). Shipped/Completed: the seller.
    """
    if target == PENDING:
        return
    if target == PAID:
        pay_order(w, buyer, order_id)
        wait_status(w, buyer, order_id, PAID)
        return
    if target in (SHIPPED, COMPLETED):
        ok(post(w, seller, ORDER, "UpdateOrderStatus", {"id": order_id, "status": SHIPPED}))
        if target == COMPLETED:
            ok(post(w, seller, ORDER, "UpdateOrderStatus", {"id": order_id, "status": COMPLETED}))
        return
    raise ValueError(target)


def pay_order(w: OicWorld, buyer: Actor, order_id: str, success: bool = True) -> None:
    """Open and settle an online mock payment (publishes PaymentSettled)."""
    tx = ok(
        post(
            w,
            buyer,
            PAYMENT,
            "CreatePayment",
            {"orderId": order_id, "method": "PAYMENT_METHOD_MOCK_BANK"},
        )
    ).get("transaction", {})
    ok(
        post(
            w,
            buyer,
            PAYMENT,
            "ProcessMockPayment",
            {"transactionId": tx.get("id", ""), "simulateSuccess": success},
        )
    )


def open_payment(w: OicWorld, buyer: Actor, order_id: str) -> str:
    tx = ok(
        post(
            w,
            buyer,
            PAYMENT,
            "CreatePayment",
            {"orderId": order_id, "method": "PAYMENT_METHOD_MOCK_BANK"},
        )
    ).get("transaction", {})
    assert tx.get("id"), tx
    return tx["id"]


def settle_payment(w: OicWorld, buyer: Actor, tx_id: str) -> httpx.Response:
    return post(
        w, buyer, PAYMENT, "ProcessMockPayment", {"transactionId": tx_id, "simulateSuccess": True}
    )


def wait_status(
    w: OicWorld, actor: Actor, order_id: str, expected: str, timeout: float = 25.0
) -> str:
    deadline = time.monotonic() + timeout
    got = ""
    while time.monotonic() < deadline:
        got = order_status(w, actor, order_id)
        if got == expected:
            return got
        time.sleep(0.5)
    raise AssertionError(f"order {order_id} is {got!r}, expected {expected!r}")


def stays_status(
    w: OicWorld, actor: Actor, order_id: str, expected: str, seconds: float = 6.0
) -> None:
    """The order is ``expected`` now and still is after ``seconds`` (late consumers settle)."""
    end = time.monotonic() + seconds
    while True:
        got = order_status(w, actor, order_id)
        assert got == expected, f"order {order_id} is {got!r}, expected {expected!r}"
        if time.monotonic() >= end:
            return
        time.sleep(1)


def saga(w: OicWorld, actor: Actor, order_id: str) -> dict:
    return ok(post(w, actor, ORDER, "GetSagaState", {"orderId": order_id}))


def step(saga_view: dict, prefix: str) -> dict:
    """The saga step whose name starts with ``prefix`` (e.g. ``3.``)."""
    for s in saga_view.get("steps", []):
        if s.get("name", "").startswith(prefix):
            return s
    raise AssertionError(f"no saga step {prefix!r} in {saga_view}")


# ── concurrency ──────────────────────────────────────────────────────────
def race(calls: list[Callable[[], httpx.Response]]) -> list[httpx.Response]:
    """Run ``calls`` released together by a barrier; results in call order."""
    barrier = threading.Barrier(len(calls))
    results: list[httpx.Response | BaseException | None] = [None] * len(calls)

    def run(i: int) -> None:
        try:
            barrier.wait(timeout=20)
            results[i] = calls[i]()
        except BaseException as exc:  # noqa: BLE001 - surfaced below
            results[i] = exc

    threads = [threading.Thread(target=run, args=(i,)) for i in range(len(calls))]
    for t in threads:
        t.start()
    for t in threads:
        t.join(60)
    for r in results:
        if isinstance(r, BaseException) or r is None:
            raise AssertionError(f"racing call failed to complete: {r!r}")
    return results  # type: ignore[return-value]


# ── fault injection (destructive lane only) ──────────────────────────────
def stop_domain(w: OicWorld) -> None:
    """docker stop team-domain; registers the restart as a cleanup in ``w.data``."""
    name = domain_container()
    _docker("stop", name)
    w.data["domain_stopped"] = name


def start_domain(w: OicWorld) -> None:
    name = w.data.pop("domain_stopped", None)
    if name:
        _docker("start", name)
        wait_domain_up(w)


def wait_domain_up(w: OicWorld, timeout: float = 90.0) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            r = post(w, None, LISTING, "GetListing", {"id": next(iter(w.listings.values()))["id"]})
            if r.status_code == 200:
                return
        except httpx.HTTPError:
            pass
        time.sleep(1)
    raise TimeoutError("team-domain did not come back")
