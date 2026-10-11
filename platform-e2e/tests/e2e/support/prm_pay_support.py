"""Gateway helpers for payment-refund-model (area prm-pay): cumulative refunds.

Everything goes through the public edge (Connect JSON). Refund ids are caller-chosen and global,
so every scenario names them symbolically ("R1") and the world maps each name to a unique real id.
Wire format: int64 fields are JSON strings, enums are their full names, zero values are omitted.
"""

from __future__ import annotations

import time

import httpx

from tests.e2e.support import plp_support as p
from tests.e2e.support.oic_order_support import Actor, OicWorld, post

PARTIALLY_REFUNDED = p.PARTIALLY_REFUNDED
SOURCE_PREFIX = "PAYMENT_REFUND_SOURCE_"
STATUS_PREFIX = "PAYMENT_STATUS_"
REMAINDER_MESSAGE = "refund amount exceeds the refundable remainder"


def status_name(short: str) -> str:
    return STATUS_PREFIX + short


def source_name(short: str) -> str:
    return SOURCE_PREFIX + short


def refund_id_named(w: OicWorld, name: str) -> str:
    """The unique real refund id behind a symbolic name such as `R1` (stable inside a scenario)."""
    ids = w.data.setdefault("named_refund_ids", {})
    if name not in ids:
        ids[name] = p.new_refund_id(name)
    return ids[name]


def refund_named(
    w: OicWorld, actor: Actor, tx_id: str, amount: int, name: str, reason: str = "e2e"
) -> httpx.Response:
    return p.refund(w, actor, tx_id, amount, reason, refund_id=refund_id_named(w, name))


def refund_without_id(w: OicWorld, actor: Actor, tx_id: str, amount: int) -> httpx.Response:
    return post(
        w,
        actor,
        p.PAYMENT,
        "RefundPayment",
        {"paymentId": tx_id, "amount": amount, "reason": "e2e"},
    )


# ── the payment as a caller reads it ─────────────────────────────────────
def refunds_of(tx: dict) -> list[dict]:
    return tx.get("refunds", []) or []


def refunded_amount(tx: dict) -> int:
    return int(tx.get("refundedAmount", 0) or 0)


def refund_amount(r: dict) -> int:
    return int(r.get("amount", 0) or 0)


def get_payment(w: OicWorld, actor: Actor, order_id: str) -> httpx.Response:
    return post(w, actor, p.PAYMENT, "GetPayment", {"orderId": order_id})


def describe_tx(tx: dict) -> str:
    return (
        f"status={tx.get('status')} refunded={refunded_amount(tx)} "
        f"refunds={[(r.get('id'), r.get('source'), r.get('amount')) for r in refunds_of(tx)]}"
    )


def wait_payment_state(
    w: OicWorld,
    actor: Actor,
    order_id: str,
    status: str,
    refunded: int,
    count: int | None = None,
    timeout: float = p.SETTLE_WAIT_S,
) -> dict:
    """Poll the payment until it reads `status` with `refunded` (and `count` refunds)."""
    deadline = time.monotonic() + timeout
    tx: dict = {}
    while time.monotonic() < deadline:
        try:
            tx = p.payment_of(w, actor, order_id)
        except (AssertionError, httpx.HTTPError):
            tx = {}
        if (
            tx.get("status") == status
            and refunded_amount(tx) == refunded
            and (count is None or len(refunds_of(tx)) == count)
        ):
            return tx
        time.sleep(0.5)
    want = f"{status} refunded={refunded}" + (f" refunds={count}" if count is not None else "")
    raise AssertionError(f"payment of order {order_id} reads [{describe_tx(tx)}], want [{want}]")


# ── ledger references ────────────────────────────────────────────────────
def reference_of(entry: dict) -> str:
    return str(entry.get("referenceId", "") or "")


def deductions(w: OicWorld, seller: Actor) -> list[dict]:
    return p.rows(w, seller, p.DEDUCTION)


def wait_deduction_amounts(
    w: OicWorld, seller: Actor, amounts: list[int], timeout: float = p.SETTLE_WAIT_S
) -> list[dict]:
    """Poll until the seller's REFUND_DEDUCTION entries are exactly `amounts` (any order)."""
    deadline = time.monotonic() + timeout
    got: list[dict] = []
    while True:
        try:
            got = deductions(w, seller)
        except (AssertionError, httpx.HTTPError):
            got = []
        if sorted(int(e["amount"]) for e in got) == sorted(amounts) or time.monotonic() > deadline:
            break
        time.sleep(0.5)
    have = sorted(int(e["amount"]) for e in got)
    assert have == sorted(
        amounts
    ), f"REFUND_DEDUCTION amounts {have}, want {sorted(amounts)}: {got}"
    return got


def check_references(deds: list[dict], tx: dict) -> None:
    """Every deduction references a distinct refund of the payment, of its own amount."""
    by_id: dict[str, dict] = {r.get("id", ""): r for r in refunds_of(tx)}
    seen: set[str] = set()
    for e in deds:
        ref = reference_of(e)
        assert ref, f"deduction {e} carries no reference (want its refund's id): {describe_tx(tx)}"
        assert (
            ref in by_id
        ), f"deduction references {ref!r}, not a refund of the payment: {describe_tx(tx)}"
        assert ref not in seen, f"two deductions reference refund {ref!r}"
        seen.add(ref)
        assert -int(e["amount"]) == refund_amount(
            by_id[ref]
        ), f"deduction {e['amount']} references refund {ref!r} of {refund_amount(by_id[ref])}"
