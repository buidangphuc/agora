"""Consent-aware TrackingEvent contract + GetChannelFunnel authz (ga4-consent-contract).

Old-producer path: a beacon sent with no `agora_consent` cookie and no GA client id must reach
the warehouse unchanged, and on the wire fields 25/26 must read as "no GA id, consent denied".
Field numbers (spec): TrackingEvent.ga_client_id = 25, consent = 26; ConsentState analytics=1,
marketing=2, personalization=3, version=4. The GetChannelFunnel scenarios stay red until
add-ga4-export-import serves the RPC. Login steps live in common_steps.
"""

from __future__ import annotations

import uuid
from datetime import datetime, timedelta, timezone
from typing import Any

import httpx
from pytest_bdd import parsers, then, when

from tests.e2e.flows.tracking_flow import _fields
from tests.e2e.step_definitions.seller_analytics_access_steps import (
    PASSWORD,
    signed_in_seller,  # noqa: F401  (Given a signed-in seller)
)
from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World

_GA_CLIENT_ID = 25
_CONSENT = 26
_FUNNEL = "/platform.analytics.v1.AnalyticsQueryService/GetChannelFunnel"
_COUNTS = ("impressions", "views", "adds", "beginCheckouts", "purchases")


def _x(world: World) -> dict:
    return world.state.extra


def _payload_fields(raw: bytes) -> list[tuple[int, int, Any]]:
    """Fields of the TrackingEvent payload (EventEnvelope field 7)."""
    for number, wire, value in _fields(bytes(raw)):
        if number == 7 and wire == 2:
            return _fields(bytes(value))  # type: ignore[arg-type]
    raise AssertionError("envelope has no payload (field 7)")


def _raw_envelope(listing: str) -> bytes:
    from config.settings import get_settings
    from tests.e2e.flows.tracking_flow import consume_tracking_events

    s = get_settings()
    raws = consume_tracking_events(
        s.kafka_brokers, s.kafka_analytics_topic, contains=listing, timeout_s=30.0
    )
    assert len(raws) == 1, f"expected one envelope for {listing}, got {len(raws)}"
    assert isinstance(raws[0], (bytes, bytearray)), "expected a binary protobuf envelope"
    return bytes(raws[0])


# ── old producer path ────────────────────────────────────────────────────
@when("a visitor posts a view beacon with no consent cookie and no GA client id")
def post_plain_view(world: World) -> None:
    x = _x(world)
    x["cc_listing"] = f"e2e-cc-{tii.run_id()}"
    x["cc_session"] = f"e2e-cc-sess-{tii.run_id()}"
    x["cc_path"] = "/e2e/consent-contract"
    beacon = {
        "type": "view",
        "listingId": x["cc_listing"],
        "sessionId": x["cc_session"],
        "anonymousId": f"e2e-anon-{uuid.uuid4()}",
        "path": x["cc_path"],
        "eventId": str(uuid.uuid4()),
    }
    resp = tii.post_track([beacon])
    assert resp.status_code == 202 and resp.json().get("accepted") == 1, resp.text[:300]


@then("the warehouse holds one row for that listing carrying the beacon's session id and page path")
def warehouse_row_unchanged(world: World) -> None:
    x = _x(world)
    rows = tii.warehouse_rows(
        "SELECT session_id, page_path FROM tracking_events WHERE listing_id = ?",
        [x["cc_listing"]],
        until=lambda r: len(r) == 1,
        settle_s=8.0,
    )
    assert rows == [(x["cc_session"], x["cc_path"])], rows


@then("the envelope on analytics.events has no GA client id")
def envelope_has_no_ga_client_id(world: World) -> None:
    ids = [
        v
        for n, w, v in _payload_fields(_raw_envelope(_x(world)["cc_listing"]))
        if n == _GA_CLIENT_ID
    ]
    assert all(not bytes(v) for v in ids), f"ga_client_id set without consent: {ids}"


@then(
    "the envelope on analytics.events grants no consent, with analytics, marketing and "
    "personalization false and version 0"
)
def envelope_consent_denied(world: World) -> None:
    consent = [
        v for n, w, v in _payload_fields(_raw_envelope(_x(world)["cc_listing"])) if n == _CONSENT
    ]
    for raw in consent:  # unset (absent) reads as denied; if present it must say so explicitly
        granted = {n: v for n, w, v in _fields(bytes(raw)) if n in (1, 2, 3, 4) and v}
        assert not granted, f"consent present but not denied: {granted}"


# ── GetChannelFunnel ─────────────────────────────────────────────────────
def _ts(dt: datetime) -> str:
    return dt.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _call(world: World, body: dict) -> httpx.Response:
    return world.service_factory.analytics.send("POST", _FUNNEL, json_body=body)


def _last_30d() -> dict:
    now = datetime.now(timezone.utc)
    return {"from": _ts(now - timedelta(days=30)), "to": _ts(now)}


@when("the seller requests GetChannelFunnel for their own seller id")
def seller_calls_funnel(world: World) -> None:
    x = _x(world)
    _x(world)["cc_resp"] = _call(world, {"seller_id": x["seller_id"], **_last_30d()})


@when("a signed-in buyer requests GetChannelFunnel with no seller id")
def buyer_calls_funnel(world: World) -> None:
    username = f"e2e_buyer_{uuid.uuid4().hex[:10]}"
    token = world.service_factory.auth.register(username, PASSWORD, role="buyer")
    world.service_factory.set_token(token)
    _x(world)["cc_resp"] = _call(world, _last_30d())


@when("the admin requests GetChannelFunnel for the last 30 days with no seller id")
def admin_all_sellers(world: World) -> None:
    _x(world)["cc_all"] = _call(world, _last_30d())


@when("the admin requests GetChannelFunnel for the last 30 days for a seller with no events")
def admin_unknown_seller(world: World) -> None:
    _x(world)["cc_one"] = _call(
        world, {"seller_id": f"e2e-none-{uuid.uuid4().hex[:8]}", **_last_30d()}
    )


@when("the admin requests GetChannelFunnel with from after to")
def admin_inverted_range(world: World) -> None:
    now = datetime.now(timezone.utc)
    _x(world)["cc_resp"] = _call(world, {"from": _ts(now), "to": _ts(now - timedelta(days=7))})


@when("the admin requests GetChannelFunnel over a range that holds no events")
def admin_empty_range(world: World) -> None:
    _x(world)["cc_resp"] = _call(
        world, {"from": "2001-01-01T00:00:00Z", "to": "2001-01-02T00:00:00Z"}
    )


@then(parsers.parse("the channel funnel call returns {status:d} and no rows"))
def funnel_denied(world: World, status: int) -> None:
    resp: httpx.Response = _x(world)["cc_resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"
    assert "rows" not in resp.text, resp.text


@then(parsers.parse("the channel funnel call returns {status:d} with zero rows"))
def funnel_empty(world: World, status: int) -> None:
    resp: httpx.Response = _x(world)["cc_resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"
    assert not resp.json().get("rows"), resp.text


@then(parsers.parse("the channel funnel call returns {status:d}"))
def funnel_status(world: World, status: int) -> None:
    resp: httpx.Response = _x(world)["cc_resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"


@then("both channel funnel calls return 200")
def both_ok(world: World) -> None:
    for key in ("cc_all", "cc_one"):
        r: httpx.Response = _x(world)[key]
        assert r.status_code == 200, f"{key}: {r.status_code} {r.text}"


@then("the seller-scoped funnel has no rows")
def scoped_no_rows(world: World) -> None:
    assert not _x(world)["cc_one"].json().get("rows"), _x(world)["cc_one"].text


def _total(resp: httpx.Response, field: str) -> int:
    return sum(int(r.get(field, 0)) for r in resp.json().get("rows", []))


@then("no channel funnel count of the seller-scoped call exceeds the all-sellers total")
def scoped_le_all(world: World) -> None:
    for f in _COUNTS:
        assert _total(_x(world)["cc_one"], f) <= _total(_x(world)["cc_all"], f), f
