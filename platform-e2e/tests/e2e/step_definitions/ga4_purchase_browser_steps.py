"""Browser side of add-ga4-purchase-relay (tracking/ga4_purchase_browser.feature).

Black box: window.dataLayer, the /api/track beacon bodies captured by ga4_datalayer_steps, and
the warehouse `tracking_events` table.
"""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.constants import timeouts
from tests.e2e.step_definitions.ga4_datalayer_steps import _start_recording
from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World


@when("the buyer starts recording tracking beacons")
def start_recording(world: World) -> None:
    _start_recording(world)


@when("the buyer places the order and lands on the order list")
def place_order(world: World) -> None:
    from tests.e2e.step_definitions.d2_checkout_steps import d2

    button = world.page.get_by_role("button", name="Đặt hàng").first
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    button.click()
    world.page.wait_for_url(
        re.compile(r".*/(account/orders|checkout/pay).*"), timeout=timeouts.NAVIGATION
    )
    orders = d2.buyer_orders(world).list_buyer_orders().get("orders", [])
    assert len(orders) == 1, orders
    world.state.order_id = orders[0]["id"]


def _purchase_beacons(world: World) -> list[dict]:
    return [
        b
        for batch in world.state.extra["track_batches"]
        for b in batch
        if str(b.get("type", "")).lower() == "purchase"
    ]


@then("window.dataLayer holds 0 purchase entries")
def no_purchase_in_datalayer(world: World) -> None:
    entries = world.page.evaluate(
        "() => (window.dataLayer || []).filter((e) => e && e.event === 'purchase')"
    )
    assert entries == [], f"purchase pushed to dataLayer: {entries}"


@then("the /api/track requests carry exactly 1 purchase beacon with the id of the created order")
def beacon_purchase(world: World) -> None:
    beacons = _purchase_beacons(world)
    assert len(beacons) == 1, f"purchase beacons: {beacons}"
    tx = beacons[0].get("transactionId") or beacons[0].get("transaction_id")
    assert tx == world.state.order_id, beacons[0]


@then("tracking_events has one purchase row with transaction_id equal to the created order id")
def warehouse_purchase(world: World) -> None:
    rows = tii.warehouse_rows(
        "SELECT event_type, transaction_id FROM tracking_events WHERE transaction_id = ?",
        [world.state.order_id],
        until=lambda r: any("purchase" in str(x[0]).lower() for x in r),
        settle_s=3.0,
    )
    purchases = [r for r in rows if "purchase" in str(r[0]).lower()]
    assert len(purchases) == 1, rows
