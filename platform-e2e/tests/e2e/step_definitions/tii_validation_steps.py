"""Per-event validation of POST /api/track (tracking-ingest-integrity, area tii-e2e)."""

from __future__ import annotations

from pytest_bdd import parsers, then, when

from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World


def _x(world: World) -> dict:
    return world.state.extra


@when(
    'a visitor posts a batch of three events, two valid views carrying unique markers and one of type "teleport"'
)
def post_mixed_batch(world: World) -> None:
    x = _x(world)
    x["tii_run"] = f"e2e-tii-{tii.run_id()}"
    x["tii_valid"] = [f"{x['tii_run']}-a", f"{x['tii_run']}-b"]
    batch = [
        tii.view(x["tii_valid"][0]),
        {**tii.view(f"{x['tii_run']}-bad"), "type": "teleport"},
        tii.view(x["tii_valid"][1]),
    ]
    x["tii_resp"] = tii.post_track(batch)


@when("a visitor posts a batch with one valid view and one view whose properties has 21 keys")
def post_oversized_properties(world: World) -> None:
    x = _x(world)
    x["tii_run"] = f"e2e-tii-{tii.run_id()}"
    x["tii_valid"] = [f"{x['tii_run']}-ok"]
    too_many = {f"k{i}": "v" for i in range(21)}
    batch = [
        tii.view(x["tii_valid"][0]),
        tii.view(f"{x['tii_run']}-big", properties=too_many),
    ]
    x["tii_resp"] = tii.post_track(batch)


@then(parsers.parse("the response is 202 with accepted {accepted:d} and dropped {dropped:d}"))
def response_counts(world: World, accepted: int, dropped: int) -> None:
    resp = _x(world)["tii_resp"]
    assert resp.status_code == 202, (resp.status_code, resp.text[:300])
    assert resp.json() == {"accepted": accepted, "dropped": dropped}, resp.text[:300]


@then("both valid markers reach analytics.events")
def both_valid_reach_kafka(world: World) -> None:
    x = _x(world)
    seen = [e["session_id"] for e in tii.kafka_envelopes(x["tii_run"])]
    assert sorted(seen) == sorted(x["tii_valid"]), f"expected {x['tii_valid']}, got {seen}"


@then("only the valid view reaches analytics.events")
def only_valid_reaches_kafka(world: World) -> None:
    x = _x(world)
    seen = [e["session_id"] for e in tii.kafka_envelopes(x["tii_run"])]
    assert seen == x["tii_valid"], f"expected only {x['tii_valid']}, got {seen}"
