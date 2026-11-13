"""Steps of the price-breakdown scenarios in order/order_events_published.feature
(change order-paid-price-breakdown, initiative monetization).

Black box: the order is placed and paid online through the gateway, the OrderPaidEvent is read
from the real `order.events` topic and compared with what GetOrder reports for the same order.
Stack-gated: needs the broker (`KAFKA_BROKERS`) and confluent-kafka in the runner.
"""

from __future__ import annotations

from typing import Any

from pytest_bdd import parsers, then, when

from config.settings import get_settings
from tests.e2e.flows import consume_order_paid_events
from tests.e2e.step_definitions.order_multishop_checkout_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.order_multishop_checkout_steps import _d, _placed
from tests.e2e.support import multishop_support as ms
from tests.e2e.support import oic_order_support as s

_FIELDS = ("items_subtotal", "shipping_fee", "shop_discount_amount", "platform_discount_amount")


@when(parsers.parse('"{label}" pays the first placed order online'))
def pay_first_online(msw: s.OicWorld, label: str) -> None:
    order = _placed(msw, label)[0]
    buyer = msw.actors[label]
    s.pay_order(msw, buyer, order["id"])
    s.wait_status(msw, buyer, order["id"], s.PAID)


def _event_and_order(msw: s.OicWorld, topic: str, label: str) -> tuple[dict[str, Any], dict]:
    order_id = _placed(msw, label)[0]["id"]
    envelopes = consume_order_paid_events(get_settings().kafka_brokers, topic, order_id)
    assert len(envelopes) == 1, f"expected one OrderPaidEvent for {order_id}, got {len(envelopes)}"
    order = s.get_order(msw, msw.actors[label], order_id)
    _d(msw)["paid_event"] = envelopes[0]
    _d(msw)["paid_order"] = order
    return envelopes[0], order


@then(
    parsers.parse(
        'the single OrderPaidEvent on the "{topic}" topic for the first placed order of '
        '"{label}" matches its GetOrder price breakdown'
    )
)
def event_matches_get_order(msw: s.OicWorld, topic: str, label: str) -> None:
    event, order = _event_and_order(msw, topic, label)
    for name in _FIELDS:
        assert name in event, f"OrderPaidEvent lacks {name} (presence false): {event}"
    discount = ms.n(order.get("discountAmount"))
    platform = ms.n(order.get("platformDiscountAmount"))
    assert event["items_subtotal"] == ms.n(order.get("itemsSubtotal")), (event, order)
    assert event["shipping_fee"] == ms.n(order.get("shippingFee")), (event, order)
    assert event["shop_discount_amount"] == discount - platform, (event, order)
    assert event["platform_discount_amount"] == platform, (event, order)
    assert event["total_amount"] == ms.n(order.get("totalAmount")), (event, order)


@then(
    "that breakdown has a positive shipping fee, a positive platform discount and a positive shop discount"
)
def breakdown_is_non_trivial(msw: s.OicWorld) -> None:
    event = _d(msw)["paid_event"]
    assert event["shipping_fee"] > 0, event
    assert event["platform_discount_amount"] > 0, event
    assert event["shop_discount_amount"] > 0, event


@then(
    "that breakdown reports shop_discount_amount 0 and platform_discount_amount 0 with presence true"
)
def breakdown_zero_present(msw: s.OicWorld) -> None:
    event = _d(msw)["paid_event"]
    assert event.get("shop_discount_amount") == 0, event
    assert event.get("platform_discount_amount") == 0, event
