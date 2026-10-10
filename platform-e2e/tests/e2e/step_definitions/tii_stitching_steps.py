"""Identity stitching views of the warehouse (tracking-ingest-integrity, area tii-e2e).

`tracking_events_resolved.user_key` is read from a read-only copy of the DuckDB file (see
`tii_support.warehouse_rows`). The buyer id is the JWT subject of a freshly registered buyer.
"""

from __future__ import annotations

import uuid

from pytest_bdd import parsers, then, when

from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World

_SQL = "SELECT listing_id, user_key FROM tracking_events_resolved WHERE listing_id IN ({})"


def _x(world: World) -> dict:
    return world.state.extra


def _post(beacon: dict, token: str | None = None) -> None:
    resp = tii.post_track([beacon], token=token)
    assert resp.status_code == 202, (resp.status_code, resp.text[:300])
    assert resp.json().get("accepted") == 1, resp.text[:300]


def _resolved(listings: list[str]) -> dict[str, str]:
    marks = ", ".join("?" for _ in listings)
    rows = tii.warehouse_rows(
        _SQL.format(marks), listings, until=lambda r: {lid for lid, _ in r} == set(listings)
    )
    return {lid: key for lid, key in rows}


@when(
    "a visitor posts a view anonymously with anonymousId X, then logs in as a new buyer "
    "and posts a view with the same anonymousId X"
)
def anonymous_then_logged_in(world: World) -> None:
    x = _x(world)
    anon = f"e2e-anon-{uuid.uuid4()}"
    before = tii.view(f"e2e-tii-sess-{tii.run_id()}", anonymousId=anon)
    after = tii.view(f"e2e-tii-sess-{tii.run_id()}", anonymousId=anon)
    _post(before)
    token, buyer_id = tii.register_buyer()
    _post(after, token=token)
    x["tii_buyer_id"] = buyer_id
    x["tii_listings"] = [before["listingId"], after["listingId"]]


@then("in tracking_events_resolved both views have user_key equal to the buyer's id")
def both_resolve_to_buyer(world: World) -> None:
    x = _x(world)
    resolved = _resolved(x["tii_listings"])
    assert resolved == dict.fromkeys(x["tii_listings"], x["tii_buyer_id"]), resolved


@when("a visitor posts a view anonymously with a fresh anonymousId Y and never logs in")
def anonymous_only(world: World) -> None:
    x = _x(world)
    x["tii_anon"] = f"e2e-anon-{uuid.uuid4()}"
    beacon = tii.view(f"e2e-tii-sess-{tii.run_id()}", anonymousId=x["tii_anon"])
    _post(beacon)
    x["tii_listings"] = [beacon["listingId"]]


@then(parsers.parse('in tracking_events_resolved that view has user_key "anon:Y"'))
def anonymous_key(world: World) -> None:
    x = _x(world)
    resolved = _resolved(x["tii_listings"])
    assert resolved == {x["tii_listings"][0]: f"anon:{x['tii_anon']}"}, resolved


@when(
    "a visitor posts a view anonymously with anonymousId Z, then two different new buyers "
    "each post a view with the same anonymousId Z"
)
def anonymous_then_two_accounts(world: World) -> None:
    x = _x(world)
    x["tii_anon"] = anon = f"e2e-anon-{uuid.uuid4()}"
    pre = tii.view(f"e2e-tii-sess-{tii.run_id()}", anonymousId=anon)
    _post(pre)
    owners = {pre["listingId"]: f"anon:{anon}"}
    for _ in range(2):
        token, buyer_id = tii.register_buyer()
        beacon = tii.view(f"e2e-tii-sess-{tii.run_id()}", anonymousId=anon)
        _post(beacon, token=token)
        owners[beacon["listingId"]] = buyer_id
    x["tii_owners"] = owners


@then(
    parsers.parse(
        'in tracking_events_resolved the anonymous view keeps "anon:Z" and each buyer\'s view '
        "has that buyer's id"
    )
)
def ambiguous_not_stitched(world: World) -> None:
    owners: dict[str, str] = _x(world)["tii_owners"]
    resolved = _resolved(list(owners))
    assert resolved == owners, resolved
