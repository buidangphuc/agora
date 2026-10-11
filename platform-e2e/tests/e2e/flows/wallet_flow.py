"""Seller wallet & payout flows."""

from __future__ import annotations

import time
from typing import Any

from src.models import User


def request_seller_payout_via_api(
    world, seller: User, amount: int, bank_code: str = "VCB", account_number: str = "9988776655"
) -> dict[str, Any]:
    """Request a bank payout from seller wallet."""
    world.service_factory.set_token(seller.token)
    res = world.service_factory.payment.request_payout(
        seller_id="",  # the caller's own wallet; the user id, never the username
        amount=amount,
        bank_code=bank_code,
        account_number=account_number,
        account_name="NGUYEN VAN BAN",
    )
    world.logger.info(f"Requested payout of {amount} VND for {seller.username}")
    return res


def settle_seeded_order_to_seller(world, timeout: float = 20.0) -> int:
    """Have the @needsOrder buyer pay the seeded order, then wait for the seller's credit.

    The wallet ledger is the single source of truth for seller money: only a settled
    payment credits it (there is no seeded balance), so a payout scenario needs this
    first. Returns the seller's ledger balance; leaves the seller's token set.
    """
    buyer = world.state.extra["seeded_buyer"]
    seller = world.state.seeded_seller
    assert seller and seller.token, "scenario must be tagged @needsSeller @needsOrder"
    world.service_factory.set_token(buyer.token)
    world.service_factory.payment.mock_pay(world.state.order_id, 5_000_000, success=True)
    world.service_factory.set_token(seller.token)
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        balance = int(world.service_factory.payment.wallet_balance() or 0)
        if balance > 0:
            return balance
        time.sleep(0.5)
    raise AssertionError("the paid order never credited the seller's wallet ledger")
