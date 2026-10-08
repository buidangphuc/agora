"""Warehouse ingest time of tracking rows (tracking-ingest-integrity, area tii-e2e)."""

from __future__ import annotations

import uuid

from pytest_bdd import then, when

from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World


def _x(world: World) -> dict:
    return world.state.extra


@when("a visitor posts one view with a unique listing id and the sink writes it")
def post_view_and_wait_for_sink(world: World) -> None:
    x = _x(world)
    beacon = tii.view(f"e2e-tii-sess-{tii.run_id()}", anonymousId=f"e2e-anon-{uuid.uuid4()}")
    x["tii_listing"] = beacon["listingId"]
    resp = tii.post_track([beacon])
    assert resp.status_code == 202, (resp.status_code, resp.text[:300])
    x["tii_rows"] = tii.warehouse_rows(
        "SELECT ingested_at, occurred_at FROM tracking_events WHERE listing_id = ?",
        [x["tii_listing"]],
    )


@then("the warehouse row has a non-null ingested_at that is not earlier than its occurred_at")
def ingested_after_occurred(world: World) -> None:
    rows = _x(world)["tii_rows"]
    assert len(rows) == 1, rows
    ingested_at, occurred_at = rows[0]
    assert ingested_at is not None, "ingested_at is null"
    assert occurred_at is not None, "occurred_at is null"
    assert ingested_at >= occurred_at, f"ingested_at {ingested_at} < occurred_at {occurred_at}"
