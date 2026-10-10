"""Idempotent re-sends on POST /api/track (tracking-ingest-integrity, area tii-e2e).

The envelope event_id is asserted as "same id twice", never as a value. The warehouse is read
through `tii_support.warehouse_rows` (docker cp of the DuckDB file, opened read-only).
"""

from __future__ import annotations

import uuid

from pytest_bdd import then, when

from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World

_SETTLE_S = 8.0  # the sink batches: wait a further cycle to prove no second row follows


def _x(world: World) -> dict:
    return world.state.extra


def _accepted(resp) -> None:  # noqa: ANN001
    assert resp.status_code == 202, (resp.status_code, resp.text[:300])
    assert resp.json().get("accepted") == 1, resp.text[:300]


@when("a visitor posts the same view (same eventId and anonymousId, unique listing id) twice")
def post_same_view_twice(world: World) -> None:
    x = _x(world)
    x["tii_listing"] = f"e2e-tii-{tii.run_id()}"
    beacon = {
        "type": "view",
        "listingId": x["tii_listing"],
        "sessionId": f"e2e-tii-sess-{tii.run_id()}",
        "anonymousId": f"e2e-anon-{uuid.uuid4()}",
        "path": "/e2e/tii",
        "eventId": str(uuid.uuid4()),
    }
    _accepted(tii.post_track([beacon]))
    _accepted(tii.post_track([beacon]))


@then("both envelopes on analytics.events carry the same event_id")
def same_event_id(world: World) -> None:
    events = tii.kafka_envelopes(_x(world)["tii_listing"])
    assert len(events) == 2, f"expected two envelopes, got {len(events)}"
    ids = {e["event_id"] for e in events}
    assert len(ids) == 1 and "" not in ids, f"event ids differ or are empty: {ids}"


@then("the warehouse holds exactly one row for that listing id")
def warehouse_one_row(world: World) -> None:
    listing = _x(world)["tii_listing"]
    rows = tii.warehouse_rows(
        "SELECT event_id FROM tracking_events WHERE listing_id = ?",
        [listing],
        until=lambda r: len(r) == 1,
        settle_s=_SETTLE_S,
    )
    assert len(rows) == 1, rows


@when("two different anonymous visitors post views with the same eventId")
def two_visitors_same_event_id(world: World) -> None:
    x = _x(world)
    x["tii_listing"] = f"e2e-tii-{tii.run_id()}"
    x["tii_visitors"] = [f"e2e-anon-{uuid.uuid4()}", f"e2e-anon-{uuid.uuid4()}"]
    shared_event_id = str(uuid.uuid4())
    for anon in x["tii_visitors"]:
        beacon = {
            "type": "view",
            "listingId": x["tii_listing"],
            "sessionId": f"e2e-tii-sess-{tii.run_id()}",
            "anonymousId": anon,
            "path": "/e2e/tii",
            "eventId": shared_event_id,
        }
        _accepted(tii.post_track([beacon]))


@then("the warehouse holds one row for each visitor")
def warehouse_row_per_visitor(world: World) -> None:
    x = _x(world)
    wanted = sorted(x["tii_visitors"])
    rows = tii.warehouse_rows(
        "SELECT anonymous_id FROM tracking_events WHERE listing_id = ? ORDER BY anonymous_id",
        [x["tii_listing"]],
        until=lambda r: sorted(a for (a,) in r) == wanted,
        settle_s=_SETTLE_S,
    )
    assert sorted(a for (a,) in rows) == wanted, rows
