"""Edge scrubbing of free text on POST /api/track (tracking-ingest-integrity, area tii-e2e)."""

from __future__ import annotations

from pytest_bdd import parsers, then, when

from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World


def _x(world: World) -> dict:
    return world.state.extra


def _post_view(world: World, **fields: str) -> None:
    x = _x(world)
    x["tii_marker"] = f"e2e-tii-{tii.run_id()}"
    resp = tii.post_track(tii.view(x["tii_marker"], **fields))
    assert resp.status_code == 202, (resp.status_code, resp.text[:300])
    assert resp.json().get("accepted") == 1, resp.text[:300]


def _the_event(world: World) -> dict[str, str]:
    marker = _x(world)["tii_marker"]
    events = tii.kafka_envelopes(marker)
    assert len(events) == 1, f"expected exactly one event for {marker}, got {len(events)}"
    return events[0]


@when(parsers.parse('a visitor posts a view whose query is "{query}"'))
def post_view_with_query(world: World, query: str) -> None:
    _post_view(world, query=query)


@when(parsers.parse('a visitor posts a view whose referrer is "{referrer}"'))
def post_view_with_referrer(world: World, referrer: str) -> None:
    _post_view(world, referrer=referrer)


@then(parsers.parse('the event on analytics.events has search_query "{expected}"'))
def event_has_search_query(world: World, expected: str) -> None:
    got = _the_event(world)["search_query"]
    assert got == expected, f"search_query {got!r} != {expected!r}"


@then(parsers.parse('the event on analytics.events has referrer "{expected}"'))
def event_has_referrer(world: World, expected: str) -> None:
    got = _the_event(world)["referrer"]
    assert got == expected, f"referrer {got!r} != {expected!r}"
