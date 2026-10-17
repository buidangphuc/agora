"""Seeding and polling helpers for frontend/order_lifecycle_ui.feature.

Everything goes through the gateway as the real buyer and seller; nothing is mocked. Timer
scenarios rely on compose/order-inventory.override.yaml (payment deadline 45s, auto-complete
12s, return window 20s, ship-by 60s, sweep 2s) and poll for the outcome.
"""

from __future__ import annotations

import time
from collections.abc import Callable
from typing import Any

from src.api.services import CartService, OrderService, PaymentService
from tests.e2e.support.order_lifecycle_support import ts

MOMO = "PAYMENT_METHOD_MOCK_MOMO"
PAYMENT = "/platform.payment.v1.PaymentService/"


def place(buyer_token: str, listing_id: str, method: str = MOMO, *, pay_tx: bool = False) -> str:
    """One-item checkout; `pay_tx` also opens the pending payment transaction (as checkout does)."""
    CartService(token=buyer_token).add_to_cart(listing_id, 1)
    res = OrderService(token=buyer_token).create_order({"paymentMethod": method})
    orders = res.get("orders", [])
    order_id = orders[0]["id"] if orders else res["order"]["id"]
    if pay_tx:
        PaymentService(token=buyer_token).create_payment(order_id, method)
    return order_id


def transaction_id(buyer_token: str, order_id: str) -> str:
    res = PaymentService(token=buyer_token).create_payment(order_id, MOMO)
    return (res.get("transaction") or {}).get("id", "")


def settle_payment(buyer_token: str, order_id: str, success: bool) -> None:
    tx = transaction_id(buyer_token, order_id)
    assert tx, f"no payment transaction for {order_id}"
    PaymentService(token=buyer_token).post(
        PAYMENT + "ProcessMockPayment", {"transactionId": tx, "simulateSuccess": success}
    )


def get_order(token: str, order_id: str) -> dict[str, Any]:
    return OrderService(token=token).get_order(order_id).get("order", {})


def wait_order(
    token: str, order_id: str, pred: Callable[[dict[str, Any]], bool], timeout: float, what: str
) -> dict[str, Any]:
    deadline = time.monotonic() + timeout
    order: dict[str, Any] = {}
    while time.monotonic() < deadline:
        order = get_order(token, order_id)
        if pred(order):
            return order
        time.sleep(1.0)
    raise AssertionError(f"order {order_id} never reached {what}: {order}")


def paid_order(buyer_token: str, listing_id: str) -> str:
    order_id = place(buyer_token, listing_id)
    settle_payment(buyer_token, order_id, True)
    wait_order(buyer_token, order_id, lambda o: o.get("status") == "ORDER_STATUS_PAID", 20, "PAID")
    return order_id


def payment_status(buyer_token: str, order_id: str) -> str:
    res = PaymentService(token=buyer_token).post(PAYMENT + "GetPayment", {"orderId": order_id})
    return (res.get("transaction") or {}).get("status", "")


def sleep_until(epoch_s: float) -> None:
    while time.time() < epoch_s:
        time.sleep(0.5)


__all__ = [
    "MOMO",
    "get_order",
    "paid_order",
    "payment_status",
    "place",
    "settle_payment",
    "sleep_until",
    "transaction_id",
    "ts",
    "wait_order",
]
