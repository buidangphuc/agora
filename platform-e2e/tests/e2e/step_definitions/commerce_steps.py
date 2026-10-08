"""Commerce access control + saga steps (port-security-hardening).

Covers security/promotion_access.feature, order/self_purchase.feature,
payment/mock_payment_gate.feature and ops/boot_guard_payment.feature. Everything runs
black box through the gateway (Connect JSON) with real, distinct users; the boot-guard
scenario starts the real team-payment image with ``docker run --rm`` and never touches
the running stack.
"""

from __future__ import annotations

import base64
import json
import os
import subprocess
import time
import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from src.api.services import (
    AddressService,
    AuthService,
    BaseService,
    CartService,
    ListingService,
    OrderService,
    PaymentService,
)
from src.models import Listing
from src.utils import get_test_data_manager
from tests.e2e.support.world import World

PASSWORD = "Sup3r-secret-pass!"
_VOUCHER = "/platform.promotion.v1.VoucherService/"
_SUBSCRIPTION = "/platform.promotion.v1.SubscriptionService/"
_SPONSORED = "/platform.promotion.v1.SponsoredService/"
_PAID_STATES = ("ORDER_STATUS_PAID", "ORDER_STATUS_SHIPPED", "ORDER_STATUS_COMPLETED")
_SETTLE_TIMEOUT_S = 25.0


# ── helpers ──────────────────────────────────────────────────────────────
def _subject(token: str) -> str:
    """The JWT `sub` claim (the principal id). Unverified decode: ids only."""
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload))["sub"]


def _user(role: str) -> dict[str, str]:
    token = AuthService().register(f"e2e_{role}_{uuid.uuid4().hex[:10]}", PASSWORD, role=role)
    return {"token": token, "id": _subject(token)}


def _call(token: str | None, endpoint: str, body: dict) -> httpx.Response:
    svc = BaseService(token=token)
    try:
        return svc.send("POST", endpoint, json_body=body)
    finally:
        svc.close()


def _x(world: World) -> dict:
    return world.state.extra


def _save_address(token: str) -> None:
    AddressService(token=token).create_address(
        recipient_name="Nguyen Van A",
        phone="0912345678",
        street="29 Lieu Giai",
        city="Ha Noi",
        ward="Phuong Lieu Giai",
        district="Quan Ba Dinh",
        is_default=True,
    )


def _stock(world: World) -> int:
    seller = _x(world)["seller"]["token"]
    listing = ListingService(token=seller).get_listing(_x(world)["listing_id"])
    return int(listing.get("stock") or 0)


def _voucher_used(token: str, code: str) -> int:
    resp = _call(token, _VOUCHER + "GetVoucher", {"code": code})
    assert resp.status_code == 200, f"GetVoucher {code}: {resp.status_code} {resp.text}"
    return int((resp.json().get("voucher") or {}).get("used") or 0)


def _wait_paid(token: str, order_id: str) -> dict:
    """The order turns PAID when team-order consumes PaymentSettled from Kafka; poll."""
    orders = OrderService(token=token)
    order: dict = {}
    deadline = time.monotonic() + _SETTLE_TIMEOUT_S
    while time.monotonic() < deadline:
        order = orders.get_order(order_id).get("order", {})
        if order.get("status") in _PAID_STATES:
            break
        time.sleep(0.5)
    return order


# ── actors and seed data ─────────────────────────────────────────────────
@given("a commerce buyer")
@given("a commerce buyer with a saved address")
def commerce_buyer(world: World) -> None:
    buyer = _user("buyer")
    _save_address(buyer["token"])
    _x(world)["buyer"] = buyer


@given("a second commerce buyer")
def second_commerce_buyer(world: World) -> None:
    _x(world)["buyer2"] = _user("buyer")


@given(
    parsers.parse(
        "a commerce seller with a published listing priced {price:d} with stock {stock:d}"
    )
)
def commerce_seller_with_listing(world: World, price: int, stock: int) -> None:
    seller = _user("seller")
    listing = Listing(
        title=f"[E2E][Commerce] {uuid.uuid4().hex[:8]}",
        category_id="cat-electronics",
        price=price,
        stock=stock,
        status="published",
        description="Sản phẩm seed tự động cho Commerce access E2E.",
    )
    ListingService(token=seller["token"]).create_listing(listing)
    assert listing.listing_id, "listing was not created"
    _x(world).update(seller=seller, listing_id=listing.listing_id, price=price, stock=stock)


@given("a second commerce seller")
def second_commerce_seller(world: World) -> None:
    _x(world)["seller2"] = _user("seller")


@given("the seller has a saved address")
def seller_has_address(world: World) -> None:
    _save_address(_x(world)["seller"]["token"])


@given(parsers.parse("an admin created a {percent:d} percent voucher with a quota of {quota:d}"))
def admin_creates_voucher(world: World, percent: int, quota: int) -> None:
    admin = get_test_data_manager().get_user_by_role("admin")
    token = AuthService().login(admin.username, admin.password)
    code = f"E2EC{uuid.uuid4().hex[:8].upper()}"
    resp = _call(
        token,
        _VOUCHER + "CreateVoucher",
        {
            "code": code,
            "scope": "VOUCHER_SCOPE_PLATFORM",
            "discountType": "DISCOUNT_TYPE_PERCENT",
            "discountValue": percent,
            "quota": quota,
            "startsAt": "2026-01-01T00:00:00Z",
            "endsAt": "2030-01-01T00:00:00Z",
        },
    )
    assert resp.status_code == 200, f"CreateVoucher: {resp.status_code} {resp.text}"
    _x(world).update(voucher_code=code, voucher_percent=percent)
    assert _voucher_used(token, code) == 0, "fresh voucher already redeemed"


# ── voucher saga RPCs ────────────────────────────────────────────────────
@when(
    parsers.parse(
        'the buyer previews the voucher "{code}" for a {subtotal:d} subtotal'
        " under their own preview namespace"
    )
)
def buyer_previews_own_namespace(world: World, code: str, subtotal: int) -> None:
    buyer = _x(world)["buyer"]
    _x(world)["resp"] = _call(
        buyer["token"],
        _VOUCHER + "ValidateAndReserve",
        {
            "reservationId": f"preview:{buyer['id']}:{code}",
            "code": code,
            "buyerId": buyer["id"],
            "cartSubtotal": subtotal,
        },
    )


@when(parsers.parse('the buyer reserves the voucher "{code}" under the reservation id "{rid}"'))
def buyer_reserves_foreign_id(world: World, code: str, rid: str) -> None:
    buyer = _x(world)["buyer"]
    _x(world)["resp"] = _call(
        buyer["token"],
        _VOUCHER + "ValidateAndReserve",
        {"reservationId": rid, "code": code, "buyerId": buyer["id"], "cartSubtotal": 1_000_000},
    )


@when(
    parsers.parse(
        'the buyer reserves the voucher "{code}" under another buyer\'s preview namespace'
    )
)
def buyer_reserves_other_namespace(world: World, code: str) -> None:
    buyer, other = _x(world)["buyer"], _x(world)["buyer2"]
    _x(world)["resp"] = _call(
        buyer["token"],
        _VOUCHER + "ValidateAndReserve",
        {
            "reservationId": f"preview:{other['id']}:{code}",
            "code": code,
            "buyerId": other["id"],
            "cartSubtotal": 1_000_000,
        },
    )


@when(parsers.parse('an anonymous caller previews the voucher "{code}"'))
def anonymous_previews(world: World, code: str) -> None:
    _x(world)["resp"] = _call(
        None,
        _VOUCHER + "ValidateAndReserve",
        {"reservationId": f"preview:nobody:{code}", "code": code, "cartSubtotal": 1_000_000},
    )


@then(parsers.parse("the voucher call returns {status:d}"))
@then(parsers.parse("the plan call returns {status:d}"))
def call_returns(world: World, status: int) -> None:
    resp: httpx.Response = _x(world)["resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"


@then(parsers.parse("the preview is valid with a discount of {amount:d}"))
def preview_is_valid(world: World, amount: int) -> None:
    body = _x(world)["resp"].json()
    assert body.get("valid") is True, f"preview not valid: {body}"
    assert int(body.get("discountAmount") or 0) == amount, f"discount != {amount}: {body}"


# ── plans, ads, entitlements ─────────────────────────────────────────────
@when(parsers.parse('the buyer subscribes to the plan "{plan_id}"'))
def buyer_subscribes(world: World, plan_id: str) -> None:
    _x(world)["resp"] = _call(
        _x(world)["buyer"]["token"], _SUBSCRIPTION + "Subscribe", {"planId": plan_id}
    )


def _create_campaign(token: str, listing_id: str, bid: int) -> httpx.Response:
    return _call(
        token,
        _SPONSORED + "CreateAdCampaign",
        {"listingId": listing_id, "budget": 100_000, "bid": bid},
    )


@when("the second seller creates an ad campaign for the first seller's listing")
def second_seller_advertises(world: World) -> None:
    _x(world)["resp"] = _create_campaign(
        _x(world)["seller2"]["token"], _x(world)["listing_id"], 1000
    )


@when(
    parsers.parse("the seller creates an ad campaign for their own listing with a bid of {bid:d}")
)
def seller_advertises_own(world: World, bid: int) -> None:
    _x(world)["resp"] = _create_campaign(_x(world)["seller"]["token"], _x(world)["listing_id"], bid)


@when("the seller reads the entitlements of the second seller")
def seller_reads_foreign_entitlements(world: World) -> None:
    _x(world)["resp"] = _call(
        _x(world)["seller"]["token"],
        _SUBSCRIPTION + "GetEntitlements",
        {"sellerId": _x(world)["seller2"]["id"]},
    )


# ── checkout through the saga ────────────────────────────────────────────
def _checkout(world: World, quantity: int, voucher: bool) -> None:
    x = _x(world)
    token = x["buyer"]["token"]
    CartService(token=token).clear_cart()
    CartService(token=token).add_to_cart(listing_id=x["listing_id"], quantity=quantity)
    payload: dict = {"paymentMethod": "PAYMENT_METHOD_MOCK_BANK"}
    if voucher:
        payload["voucherCode"] = x["voucher_code"]
    created = OrderService(token=token).create_order(payload)
    order_id = created["orders"][0]["id"]
    x.update(order_id=order_id, quantity=quantity)
    x["payment"] = PaymentService(token=token).mock_pay(order_id)


@when(
    parsers.parse(
        "the buyer checks out {quantity:d} of the listing with the voucher"
        " and pays with the mock payment"
    )
)
def checkout_with_voucher(world: World, quantity: int) -> None:
    _checkout(world, quantity, voucher=True)


@when(
    parsers.parse("the buyer checks out {quantity:d} of the listing and pays with the mock payment")
)
def checkout_without_voucher(world: World, quantity: int) -> None:
    _checkout(world, quantity, voucher=False)


@then("the payment succeeds")
def payment_succeeds(world: World) -> None:
    tx = _x(world)["payment"].get("transaction") or {}
    assert tx.get("status") == "PAYMENT_STATUS_PAID", f"payment not settled: {_x(world)['payment']}"


@then("the order becomes paid")
def order_becomes_paid(world: World) -> None:
    x = _x(world)
    order = _wait_paid(x["buyer"]["token"], x["order_id"])
    assert order.get("status") in _PAID_STATES, f"order {x['order_id']} is {order.get('status')!r}"
    x["order"] = order


@then(parsers.parse("the order is paid with the {percent:d} percent voucher discount applied"))
def order_paid_with_discount(world: World, percent: int) -> None:
    x = _x(world)
    order = _wait_paid(x["buyer"]["token"], x["order_id"])
    assert order.get("status") in _PAID_STATES, f"order {x['order_id']} is {order.get('status')!r}"
    subtotal = x["price"] * x["quantity"]
    expected = subtotal - subtotal * percent // 100  # subtotal >= 500k ships free
    assert int(order["totalAmount"]) == expected, f"total {order['totalAmount']} != {expected}"
    assert expected < subtotal, "the voucher did not reduce the total"


@then("the voucher redemption is committed once")
def voucher_committed_once(world: World) -> None:
    x = _x(world)
    token = x["buyer"]["token"]
    order = _wait_paid(token, x["order_id"])
    assert order.get("status") in _PAID_STATES, f"order {x['order_id']} is {order.get('status')!r}"
    deadline = time.monotonic() + _SETTLE_TIMEOUT_S
    used = _voucher_used(token, x["voucher_code"])
    while used < 1 and time.monotonic() < deadline:
        time.sleep(0.5)
        used = _voucher_used(token, x["voucher_code"])
    assert used == 1, f"voucher {x['voucher_code']} redeemed {used} times, expected exactly 1"


@then(parsers.parse("the listing's stock has decreased by {delta:d}"))
def stock_decreased(world: World, delta: int) -> None:
    x = _x(world)
    assert _stock(world) == x["stock"] - delta, f"stock {_stock(world)} != {x['stock'] - delta}"


# ── a seller buying their own listing ────────────────────────────────────
@when("the seller adds their own listing to their cart and places the order")
def seller_buys_own_listing(world: World) -> None:
    x = _x(world)
    token = x["seller"]["token"]
    CartService(token=token).add_to_cart(listing_id=x["listing_id"], quantity=1)
    x["resp"] = _call(
        token,
        "/platform.order.v1.OrderService/CreateOrder",
        {"paymentMethod": "PAYMENT_METHOD_COD"},
    )


@then(parsers.parse("the order is refused with HTTP {status:d}"))
def order_refused(world: World, status: int) -> None:
    resp: httpx.Response = _x(world)["resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"
    assert "own listing" in resp.text, f"unexpected refusal reason: {resp.text}"


@then(parsers.parse("the listing's stock is unchanged at {stock:d}"))
def stock_unchanged(world: World, stock: int) -> None:
    assert _stock(world) == stock, f"stock {_stock(world)} != {stock}"


@then("the seller has no orders")
def seller_has_no_orders(world: World) -> None:
    orders = OrderService(token=_x(world)["seller"]["token"]).list_buyer_orders()
    assert not orders.get("orders"), f"an order was created for the seller: {orders}"


# ── boot guard (black-box docker run) ────────────────────────────────────
@when(
    parsers.parse('the team-payment image is started with ENV "{env}" and MOCK_PAYMENTS "{mock}"')
)
def start_payment_image(world: World, env: str, mock: str) -> None:
    image = os.getenv("PAYMENT_BOOT_IMAGE", "agora-team-payment:local")
    # DATABASE_ENABLED=false is the minimum for config loading to reach the guard.
    cmd = [
        "docker", "run", "--rm",
        "-e", f"ENV={env}",
        "-e", f"MOCK_PAYMENTS={mock}",
        "-e", "DATABASE_ENABLED=false",
        image,
    ]  # fmt: skip
    proc = subprocess.run(cmd, capture_output=True, text=True, timeout=90, check=False)
    _x(world)["boot"] = proc
    world.logger.info(f"{image} exited {proc.returncode}: {proc.stderr.strip()[:300]}")


@then("the process exits non-zero")
def process_exits_nonzero(world: World) -> None:
    proc = _x(world)["boot"]
    assert proc.returncode not in (0, 125, 126, 127), (
        f"expected the service to exit by itself with a failure, got {proc.returncode}: "
        f"{proc.stderr[:300]}"
    )


@then("its log names MOCK_PAYMENTS")
def log_names_mock_payments(world: World) -> None:
    proc = _x(world)["boot"]
    assert (
        "MOCK_PAYMENTS" in proc.stdout + proc.stderr
    ), f"log does not name MOCK_PAYMENTS: {proc.stdout[-300:]} {proc.stderr[-300:]}"
