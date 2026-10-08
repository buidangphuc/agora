"""Gateway flows for port-payment-ledger-integrity (area plp-pay).

Everything goes through the public edge (Connect JSON). Actors are separate registered users
(one seller per scenario, distinct buyers), listings are priced so an order's payment amount is
the spec's figure (shipping is not part of the payment: the order total is price x quantity).
Ledger rows are read through the gateway (`ListLedgerEntries`, `GetWalletBalance`); the
reference of a row (not on the wire) and the store scenarios use psql, see `plp_stack`.
"""

from __future__ import annotations

import time
import uuid
from datetime import datetime, timezone
from typing import Any

import httpx

from tests.e2e.support import oic_order_support as o
from tests.e2e.support import plp_stack as stack
from tests.e2e.support.oic_order_support import Actor, OicWorld, code_of, ok, post

PAYMENT = o.PAYMENT
SETTLE_WAIT_S = 40.0  # two relayer polls (~1-3 s each) with a wide margin
QUIET_S = 3.0

SETTLEMENT = "ORDER_SETTLEMENT"
DEDUCTION = "REFUND_DEDUCTION"
PAYOUT = "PAYOUT"
PAID = "PAYMENT_STATUS_PAID"
REFUNDED = "PAYMENT_STATUS_REFUNDED"
PARTIALLY_REFUNDED = "PAYMENT_STATUS_PARTIALLY_REFUNDED"
SENTINEL_PRICE = 600_000  # >= the free-shipping subtotal: the payment equals the price
SHIPPING_FEE = 35_000
FREE_SHIPPING_FROM = 500_000


def price_for(amount: int) -> int:
    """Listing price whose one-item order pays exactly `amount` (team-order adds a 35000 shipping
    fee below a 500000 subtotal)."""
    return amount if amount >= FREE_SHIPPING_FROM else amount - SHIPPING_FEE


def seller_with_listing(w: OicWorld, amount: int, name: str = "L", stock: int = 200) -> Actor:
    """The scenario's seller (created on first use) and a published listing whose order pays `amount`."""
    seller = w.actors.get("seller") or o.register(w, "seller", "seller")
    o.create_listing(w, name, seller, stock, price=price_for(amount))
    w.listings[name]["amount"] = amount
    return seller


def buyer(w: OicWorld, name: str = "buyer") -> Actor:
    b = w.actors.get(name) or o.register(w, name, "buyer")
    o.ensure_address(w, b)
    return b


# ── payments ─────────────────────────────────────────────────────────────
def pay(w: OicWorld, b: Actor, order_id: str) -> str:
    """Open and settle the mock payment of one order; returns the transaction id."""
    tx_id = o.open_payment(w, b, order_id)
    ok(o.settle_payment(w, b, tx_id))
    w.data.setdefault("tx", {})[order_id] = tx_id
    return tx_id


def place_and_pay(w: OicWorld, b: Actor, listing: str = "L", qty: int = 1) -> tuple[str, str]:
    """A fresh order of `listing`, paid and `Paid` at team-order: (order id, transaction id)."""
    order_id = o.place_order(w, b, listing, qty)
    tx_id = pay(w, b, order_id)
    o.wait_status(w, b, order_id, o.PAID)
    return order_id, tx_id


def payment_of(w: OicWorld, b: Actor, order_id: str) -> dict:
    return ok(post(w, b, PAYMENT, "GetPayment", {"orderId": order_id})).get("transaction", {})


def wait_payment_status(
    w: OicWorld, b: Actor, order_id: str, expected: str, timeout: float = SETTLE_WAIT_S
) -> dict:
    deadline = time.monotonic() + timeout
    tx: dict = {}
    while time.monotonic() < deadline:
        try:
            tx = payment_of(w, b, order_id)
        except (AssertionError, httpx.HTTPError):  # team-payment restarting
            tx = {}
        if tx.get("status") == expected:
            return tx
        time.sleep(0.5)
    raise AssertionError(f"payment of order {order_id} is {tx.get('status')!r}, want {expected!r}")


# ── ledger reads ─────────────────────────────────────────────────────────
def ledger(w: OicWorld, seller: Actor) -> list[dict]:
    resp = post(w, seller, PAYMENT, "ListLedgerEntries", {"page": {"pageSize": 200}})
    return ok(resp).get("entries", [])


def rows(w: OicWorld, seller: Actor, type_: str, amount: int | None = None) -> list[dict]:
    got = [e for e in ledger(w, seller) if e.get("type") == type_]
    if amount is not None:
        got = [e for e in got if int(e.get("amount", 0)) == amount]
    return got


def balance(w: OicWorld, seller: Actor) -> int:
    return int(ok(post(w, seller, PAYMENT, "GetWalletBalance", {})).get("balance", 0))


def wallet_balance(w: OicWorld, seller: Actor) -> int:
    wallet = ok(post(w, seller, PAYMENT, "GetSellerWallet", {})).get("wallet") or {}
    return int(wallet.get("balance", 0))


def wait_rows(
    w: OicWorld,
    seller: Actor,
    type_: str,
    amount: int,
    count: int,
    timeout: float = SETTLE_WAIT_S,
) -> list[dict]:
    """Poll until the seller has `count` rows of `type_`/`amount`; returns them."""
    deadline = time.monotonic() + timeout
    got: list[dict] = []
    while time.monotonic() < deadline:
        try:
            got = rows(w, seller, type_, amount)
        except (AssertionError, httpx.HTTPError):  # team-payment restarting
            got = []
        if len(got) >= count:
            return got
        time.sleep(0.5)
    raise AssertionError(
        f"seller has {len(got)} {type_} row(s) of {amount} after {timeout:.0f}s, want {count}: "
        f"{ledger(w, seller)}"
    )


def wait_balance(w: OicWorld, seller: Actor, expected: int, timeout: float = SETTLE_WAIT_S) -> int:
    deadline = time.monotonic() + timeout
    got = balance(w, seller)
    while got != expected and time.monotonic() < deadline:
        time.sleep(0.5)
        got = balance(w, seller)
    assert got == expected, f"seller balance {got}, want {expected}: {ledger(w, seller)}"
    return got


def settle_sentinel(w: OicWorld, seller: Actor) -> None:
    """Pay a sentinel order of the same seller and wait for its credit.

    The settlement consumer reads one ordered partition, so once the sentinel is credited every
    earlier event of the scenario has been applied (or parked): absence checks that follow are
    not racing the consumer.
    """
    n = w.data["sentinels"] = w.data.get("sentinels", 0) + 1
    name = f"SENT{n}"
    price = SENTINEL_PRICE + n
    o.create_listing(w, name, seller, 20, price=price)
    b = buyer(w, "sentinel_buyer")
    order_id = o.place_order(w, b, name, 1)
    pay(w, b, order_id)
    o.wait_status(w, b, order_id, o.PAID)
    wait_rows(w, seller, SETTLEMENT, price, 1)
    w.data.setdefault("sentinel_amounts", []).append(price)


def non_sentinel(w: OicWorld, seller: Actor, type_: str) -> list[dict]:
    sent = set(w.data.get("sentinel_amounts", []))
    return [e for e in rows(w, seller, type_) if int(e["amount"]) not in sent]


def quiet(w: OicWorld, check, seconds: float = QUIET_S) -> None:
    """`check()` holds now and still holds after `seconds` (a late consumer would break it)."""
    check()
    end = time.monotonic() + seconds
    while time.monotonic() < end:
        time.sleep(0.5)
    check()


# ── refunds ──────────────────────────────────────────────────────────────
def new_refund_id(label: str = "") -> str:
    """A refund id that is unique across scenarios and runs (refund ids are global keys).

    1-64 characters of [A-Za-z0-9._:-], as RefundPayment requires.
    """
    return f"e2e-{uuid.uuid4().hex[:20]}" + (f"-{label}" if label else "")


def refund(
    w: OicWorld,
    actor: Actor,
    tx_id: str,
    amount: int,
    reason: str = "e2e",
    refund_id: str | None = None,
) -> httpx.Response:
    """RefundPayment through the gateway; `refund_id` is required by the API, so a fresh one is
    generated when the caller gives none. The id used is left in `w.data["last_refund_id"]`."""
    rid = refund_id or new_refund_id()
    w.data["last_refund_id"] = rid
    w.data.setdefault("refund_ids", []).append(rid)
    return post(
        w,
        actor,
        PAYMENT,
        "RefundPayment",
        {"paymentId": tx_id, "amount": amount, "reason": reason, "refundId": rid},
    )


# ── payouts ──────────────────────────────────────────────────────────────
def wallet_payout(w: OicWorld, seller: Actor, amount: int) -> httpx.Response:
    return post(w, seller, PAYMENT, "RequestWalletPayout", {"amount": amount})


def bank_payout(w: OicWorld, seller: Actor, amount: int) -> httpx.Response:
    return post(
        w,
        seller,
        PAYMENT,
        "RequestPayout",
        {
            "amount": amount,
            "bankCode": "VCB",
            "accountNumber": "0123456789",
            "accountName": "E2E SELLER",
        },
    )


def payout_history(w: OicWorld, seller: Actor) -> list[dict]:
    return ok(post(w, seller, PAYMENT, "ListPayoutHistory", {})).get("payouts", [])


def parse_ts(text: str) -> datetime:
    return datetime.fromisoformat(text.replace("Z", "+00:00")).astimezone(timezone.utc)


def wait_past_window(created_at: str, margin_s: float = 4.0) -> None:
    """Block until the credit written at `created_at` is older than the hold window.

    Deadline loop on the wall clock (host and containers share it), never a fixed sleep; fails
    fast, naming the overlay, when the window is too long to wait out.
    """
    window = stack.require_hold_wait(margin_s)
    deadline = parse_ts(created_at).timestamp() + window + margin_s
    while time.time() < deadline:
        time.sleep(min(1.0, max(0.1, deadline - time.time())))


def credit_and_age(w: OicWorld, seller: Actor, b: Actor, listing: str, price: int) -> str:
    """One paid order of `listing`, its credit observed and older than the hold window."""
    stack.require_hold_wait()  # fail fast before paying anything
    order_id, tx_id = place_and_pay(w, b, listing)
    entry = wait_rows(w, seller, SETTLEMENT, price, 1)[-1]
    wait_past_window(entry["createdAt"])
    w.data.setdefault("tx", {})[order_id] = tx_id
    return order_id


def fresh_credit(
    w: OicWorld, seller: Actor, b: Actor, listing: str, price: int
) -> tuple[str, dict]:
    """One paid order of `listing` and its observed (fresh) credit entry."""
    order_id, _ = place_and_pay(w, b, listing)
    entry = wait_rows(w, seller, SETTLEMENT, price, 1)[-1]
    return order_id, entry


def message_of(resp: httpx.Response) -> str:
    try:
        return str(resp.json().get("message", ""))
    except ValueError:
        return resp.text


def describe(resp: httpx.Response) -> dict[str, Any]:
    return {"status": resp.status_code, "code": code_of(resp), "body": resp.text[:300]}


def wait_payment_up(w: OicWorld, seller: Actor, timeout: float = 90.0) -> None:
    """Block until team-payment answers a wallet read through the gateway again."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            if post(w, seller, PAYMENT, "GetWalletBalance", {}).status_code == 200:
                return
        except httpx.HTTPError:
            pass
        time.sleep(1)
    raise TimeoutError("team-payment did not come back")
