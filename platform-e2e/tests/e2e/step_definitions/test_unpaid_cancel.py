# ruff: noqa: F811
"""Binds payment/unpaid_cancel.feature (payment-unpaid-cancel, q10-order-lifecycle).

Placement, payment-opening and the deadline wait reuse the order-unpaid-expiry helpers; payment
reads go through the gateway as the buyer.
"""

from __future__ import annotations

from pytest_bdd import parsers, scenarios, then, when

from tests.e2e.step_definitions.oic_order_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.oic_order_steps import oic  # noqa: F401  (fixture)
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import order_lifecycle_support as ol
from tests.e2e.support import plp_support as p

scenarios("payment/unpaid_cancel.feature")

_MOMO = "PAYMENT_METHOD_MOCK_MOMO"


def _open(oic, buyer, order_id) -> str:
    tx = s.ok(
        s.post(oic, buyer, s.PAYMENT, "CreatePayment", {"orderId": order_id, "method": _MOMO})
    ).get("transaction", {})
    assert tx.get("id"), tx
    return tx["id"]


def _simulate(oic, buyer, tx_id, success: bool):
    return s.post(
        oic,
        buyer,
        s.PAYMENT,
        "ProcessMockPayment",
        {"transactionId": tx_id, "simulateSuccess": success},
    )


@when(
    parsers.parse(
        '"{buyer}" places a MOCK_MOMO order for {qty:d} of "{listing}" and opens its payment'
    )
)
def when_place_and_open(oic, buyer, qty, listing):
    b = oic.actors[buyer]
    oid = ol.place(oic, b, listing, qty, "PAYMENT_METHOD_MOCK_MOMO")
    oic.orders["order"] = oid
    oic.data["order:buyer"] = b
    oic.data["order:tx"] = _open(oic, b, oid)


@when(
    parsers.parse(
        '"{buyer}" places a control MOCK_MOMO order for {qty:d} of "{listing}", '
        "opens its payment and pays it"
    )
)
def when_control_paid(oic, buyer, qty, listing):
    b = oic.actors[buyer]
    oid = ol.place(oic, b, listing, qty, "PAYMENT_METHOD_MOCK_MOMO")
    oic.orders["control"] = oid
    oic.data["control:buyer"] = b
    s.ok(_simulate(oic, b, _open(oic, b, oid), True))


@then("the control order is Paid")
def then_control_paid(oic):
    s.wait_status(oic, oic.data["control:buyer"], oic.orders["control"], s.PAID)


@then(parsers.parse("the payment of the order reads {status}"))
def then_payment_reads(oic, status):
    tx = p.payment_of(oic, oic.data["order:buyer"], oic.orders["order"])
    assert tx.get("status") == status, f"payment reads {tx.get('status')!r}, want {status}: {tx}"


@then(parsers.parse("within 30 s the payment of the order reads {status}"))
def then_payment_reads_within(oic, status):
    p.wait_payment_status(oic, oic.data["order:buyer"], oic.orders["order"], status, timeout=30)


@when(parsers.parse('"{buyer}" pays the order\'s transaction successfully'))
def when_pay_success(oic, buyer):
    oic.responses["last"] = _simulate(oic, oic.actors[buyer], oic.data["order:tx"], True)


@when(parsers.parse('"{buyer}" pays the order\'s transaction with a simulated failure'))
def when_pay_failure(oic, buyer):
    oic.responses["last"] = _simulate(oic, oic.actors[buyer], oic.data["order:tx"], False)


@when(parsers.parse('"{buyer}" cancels the unpaid order'))
def when_cancel_unpaid(oic, buyer):
    oic.responses["last"] = s.post(
        oic, oic.actors[buyer], s.ORDER, "CancelOrder", {"id": oic.orders["order"]}
    )
    s.ok(oic.responses["last"])


@then("the order stays Cancelled")
def then_stays_cancelled(oic):
    s.stays_status(oic, oic.data["order:buyer"], oic.orders["order"], s.CANCELLED, seconds=3)


@when("the payment deadline passes without payment")
def when_deadline_passes(oic):
    ol.wait_cancelled(oic, oic.data["order:buyer"], oic.orders["order"], ol.EXPIRY_WAIT_S)
