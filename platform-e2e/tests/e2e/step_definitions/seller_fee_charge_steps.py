"""Steps for the seller fee gateway scenario (payment-seller-fee-charge).

Built on the payment-ledger steps (`plp_steps`): seller/buyer actors and the credited order.
"""

from __future__ import annotations

import uuid

from pytest_bdd import parsers, then, when

from tests.e2e.step_definitions.plp_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.plp_steps import _last, _seller
from tests.e2e.support import plp_support as p
from tests.e2e.support.oic_order_support import PAYMENT, post


def _snapshot(plp) -> list[tuple[str, str, str]]:
    return sorted(
        (e.get("id", ""), e.get("type", ""), str(e.get("amount", "")))
        for e in p.ledger(plp, _seller(plp))
    )


@when("the seller calls ChargeSellerFee through the gateway with a fresh reference")
def seller_calls_charge_seller_fee(plp):
    plp.data["ledger_before_fee"] = _snapshot(plp)
    seller = _seller(plp)
    plp.responses["last"] = post(
        plp,
        seller,
        PAYMENT,
        "ChargeSellerFee",
        {
            "sellerId": seller.user_id,
            "amount": 1000,
            "kind": "SUBSCRIPTION_FEE",
            "referenceId": f"e2e-fee-{uuid.uuid4().hex[:12]}",
        },
    )


@then(parsers.parse("the gateway answers HTTP {status:d} to the fee call"))
def gateway_answers_to_fee_call(plp, status):
    got = _last(plp)
    assert got.status_code == status, f"HTTP {got.status_code}: {got.text[:300]}"


@then("the seller's ledger has no SUBSCRIPTION_FEE entry")
def seller_has_no_fee_entry(plp):
    fees = p.rows(plp, _seller(plp), "SUBSCRIPTION_FEE")
    assert not fees, f"a fee was booked through the gateway: {fees}"


@then("the seller's ledger is unchanged by the fee call")
def seller_ledger_unchanged(plp):
    assert _snapshot(plp) == plp.data["ledger_before_fee"]
