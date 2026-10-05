"""Steps for the real order.events assertion (ADR-0013): a PAID order is published
by team-order's outbox relayer as an OrderPaidEvent envelope with its line items.

Stack-gated: needs the broker (`KAFKA_BROKERS`) and confluent-kafka in the runner.
"""

from __future__ import annotations

from pytest_bdd import parsers, then

from tests.e2e.flows import consume_order_paid_events
from tests.e2e.support.world import World


@then(
    parsers.parse(
        'exactly one OrderPaidEvent envelope for the order is published to the "{topic}" topic'
    )
)
def exactly_one_order_paid_envelope(world: World, topic: str) -> None:
    order_id = world.state.order_id
    envelopes = consume_order_paid_events(world.settings.kafka_brokers, topic, order_id)
    assert len(envelopes) == 1, f"expected one OrderPaidEvent for {order_id}, got {len(envelopes)}"
    world.state.extra["order_paid_envelope"] = envelopes[0]


@then("its payload carries the order id as the key and the order's line items")
def payload_carries_order_id_and_items(world: World) -> None:
    env = world.state.extra["order_paid_envelope"]
    order_id = world.state.order_id
    assert env["_key"] == order_id, f"partition key {env['_key']!r} != order id {order_id!r}"
    assert env["order_id"] == order_id
    assert env["items"], "OrderPaidEvent must carry at least one line item"
    for item in env["items"]:
        assert item.get("listing_id"), f"line item missing listing_id: {item}"
        assert item.get("seller_id"), f"line item missing seller_id: {item}"
        assert item.get("quantity", 0) >= 1, f"line item missing quantity: {item}"
        assert item.get("unit_price", 0) > 0, f"line item missing unit_price: {item}"
    listing = world.state.listing
    if listing is not None:
        assert listing.listing_id in {i["listing_id"] for i in env["items"]}
