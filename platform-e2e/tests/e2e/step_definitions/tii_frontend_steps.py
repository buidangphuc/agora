"""Browser-side check of the tracker's eventId (tracking-ingest-integrity, area tii-e2e).

Reuses the PDP beacon recorder of pdp_streaming_steps: it taps `navigator.sendBeacon` and the
`fetch` fallback, so the beacons asserted here are what the page really sent to /api/track.
"""

from __future__ import annotations

import re
import time

from pytest_bdd import then, when

from tests.e2e.step_definitions.pdp_streaming_steps import _open_listing_recording
from tests.e2e.support.world import World

_UUID = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$", re.I)
_FLUSH_S = 5  # the beacon queue flushes every 2 s or at 20 events


@when("a buyer opens a product page in the storefront")
def open_product_page(world: World) -> None:
    _open_listing_recording(world)
    beacons: list[dict] = world.state.extra["beacons"]
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline and not any(b.get("type") == "view" for b in beacons):
        world.page.wait_for_timeout(500)
    # Scroll so viewability-gated impression beacons are produced as well, then let them flush.
    world.page.evaluate("window.scrollTo(0, document.body.scrollHeight)")
    world.page.wait_for_timeout(_FLUSH_S * 1000)


@then("every beacon the page sends to /api/track carries a distinct UUID eventId")
def every_beacon_has_distinct_event_id(world: World) -> None:
    beacons: list[dict] = world.state.extra["beacons"]
    assert any(b.get("type") == "view" for b in beacons), f"no view beacon was sent: {beacons}"
    ids = [b.get("eventId") for b in beacons]
    bad = [b for b in beacons if not _UUID.match(str(b.get("eventId") or ""))]
    assert not bad, f"beacons without a UUID eventId: {bad}"
    assert len(set(ids)) == len(ids), f"eventIds are not distinct: {ids}"
