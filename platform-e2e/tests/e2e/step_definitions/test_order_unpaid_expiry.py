"""Binds order/unpaid_expiry.feature (order-unpaid-expiry, q10-order-lifecycle)."""

from __future__ import annotations

import time

import pytest
from pytest_bdd import parsers, scenarios, then, when

from tests.e2e.step_definitions.oic_order_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.oic_order_steps import oic  # noqa: F401  (fixture)
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import order_lifecycle_support as ol

scenarios("order/unpaid_expiry.feature")

_METHOD = {"MOCK_MOMO": "PAYMENT_METHOD_MOCK_MOMO", "COD": "PAYMENT_METHOD_COD"}


def _place(oic, buyer, listing, qty, method, slot):
    b = oic.actors[buyer]
    oic.orders[slot] = ol.place(oic, b, listing, qty, _METHOD[method])
    oic.data[f"{slot}:placed"] = time.monotonic()
    oic.data[f"{slot}:buyer"] = b


@when(parsers.parse('"{buyer}" places a {method} order for {qty:d} of "{listing}"'))
def when_place(oic, buyer, method, qty, listing):
    _place(oic, buyer, listing, qty, method, "order")


@when(parsers.parse('"{buyer}" places a control order paid by {method} for {qty:d} of "{listing}"'))
def when_place_control(oic, buyer, method, qty, listing):
    _place(oic, buyer, listing, qty, method, "control")


@then("the order has a payment deadline")
def then_has_deadline(oic):
    o = s.get_order(oic, oic.data["order:buyer"], oic.orders["order"])
    assert o.get("paymentDueAt"), f"online order has no paymentDueAt: {o}"
    oic.data["due"] = o["paymentDueAt"]


@then("the order has no payment deadline")
def then_no_deadline(oic):
    o = s.get_order(oic, oic.data["order:buyer"], oic.orders["order"])
    assert not o.get("paymentDueAt"), f"COD order has a deadline: {o}"


@then("exactly one payment reminder is published for the order before its deadline")
def then_reminder(oic):
    evs = ol.events_for(oic.orders["order"], ol.REMINDER_TYPE, wait_s=ol.DEADLINE_S)
    assert len(evs) == 1, f"expected one OrderPaymentReminder, got {evs}"
    oic.data["reminder"] = evs[0]
    # emitted before the order expired
    assert s.order_status(oic, oic.data["order:buyer"], oic.orders["order"]) in (
        s.PENDING,
        s.CANCELLED,
    )


@then("the order carries the same payment deadline in the reminder")
def then_reminder_due(oic):
    from datetime import datetime

    due = oic.data["due"]
    secs = int(datetime.fromisoformat(due.replace("Z", "+00:00")).timestamp())
    assert oic.data["reminder"].get("payment_due_at") == secs


@then(parsers.parse('the stock of "{listing}" is {n:d}'))
def then_stock(oic, listing, n):
    s.wait_stock(oic, listing, n, timeout=ol.EXPIRY_WAIT_S if n == 10 else 10)


@then("the order is still Pending")
def then_still_pending(oic):
    assert s.order_status(oic, oic.data["order:buyer"], oic.orders["order"]) == s.PENDING


@when("the payment deadline passes without payment")
def when_deadline_passes(oic):
    ol.wait_cancelled(oic, oic.data["order:buyer"], oic.orders["order"], ol.EXPIRY_WAIT_S)


@when("the control order expires")
def when_control_expires(oic):
    ol.wait_cancelled(oic, oic.data["control:buyer"], oic.orders["control"], ol.EXPIRY_WAIT_S)


@then(parsers.parse('the order is Cancelled with cancel reason "{reason}"'))
def then_cancelled_reason(oic, reason):
    b, oid = oic.data["order:buyer"], oic.orders["order"]
    s.wait_status(oic, b, oid, s.CANCELLED, timeout=ol.EXPIRY_WAIT_S)
    assert ol.cancel_reason(oic, b, oid) == reason


@then(parsers.parse('the control order is Cancelled with reason "{reason}"'))
def then_control_reason(oic, reason):
    b, oid = oic.data["control:buyer"], oic.orders["control"]
    assert s.order_status(oic, b, oid) == s.CANCELLED
    assert ol.cancel_reason(oic, b, oid) == reason


@then(
    parsers.parse(
        "exactly one OrderCancelled is published for the order with previous status Pending"
        ' and reason "{reason}"'
    )
)
def then_cancelled_event(oic, reason):
    evs = ol.events_for(oic.orders["order"], ol.CANCELLED_TYPE, wait_s=20)
    assert len(evs) == 1, f"expected one OrderCancelled, got {evs}"
    assert evs[0].get("previous_status") == ol.STATUS_ENUM[s.PENDING]
    assert evs[0].get("reason_code") == ol.REASON_ENUM[reason], evs[0]


@then("no OrderCancelled is published for the order")
def then_no_cancelled_event(oic):
    # positive control (the control order's OrderCancelled) is asserted by the control steps
    evs = ol.events_for(oic.orders["order"], ol.CANCELLED_TYPE, wait_s=6, settle_s=0)
    assert evs == [], f"COD order was cancelled: {evs}"


@when(parsers.parse('"{buyer}" cancels the order with reason "{reason}"'))
def when_cancel_reason(oic, buyer, reason):
    oic.responses["last"] = s.post(
        oic,
        oic.actors[buyer],
        s.ORDER,
        "CancelOrder",
        {"id": oic.orders["order"], "reasonCode": reason},
    )
