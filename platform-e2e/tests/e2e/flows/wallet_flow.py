"""Seller wallet & payout flows."""

from __future__ import annotations

from typing import Any

from src.models import User


def request_seller_payout_via_api(
    world, seller: User, amount: int, bank_code: str = "VCB", account_number: str = "9988776655"
) -> dict[str, Any]:
    """Request a bank payout from seller wallet."""
    world.service_factory.set_token(seller.token)
    res = world.service_factory.payment.request_payout(
        seller_id=seller.username,
        amount=amount,
        bank_code=bank_code,
        account_number=account_number,
        account_name="NGUYEN VAN BAN",
    )
    world.logger.info(f"Requested payout of {amount} VND for {seller.username}")
    return res
