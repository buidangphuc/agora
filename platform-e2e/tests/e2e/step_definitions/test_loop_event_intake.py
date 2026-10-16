"""Binds tracking/loop_event_intake.feature (gateway-loop-event-intake).

Black box through POST /api/track and the analytics.events topic. Beacon posting and the Kafka
consumer are reused from tii_support / tracking_flow; the envelope decode is local because the
shared helper does not expose event type, properties or principal.
"""

from __future__ import annotations

import json

import httpx
from pytest_bdd import parsers, scenarios, then, when

from config.settings import get_settings
from src.constants import gateway_endpoints as ep
from tests.e2e.flows.tracking_flow import _fields, consume_tracking_events
from tests.e2e.step_definitions.tii_validation_steps import *  # noqa: F401,F403
from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World

scenarios("tracking/loop_event_intake.feature")

_NEG_TIMEOUT_S = 8.0
_PRINCIPAL_ANONYMOUS = 1
_PRINCIPAL_USER = 2
_ORD = {"1st": 0, "2nd": 1, "3rd": 2, "4th": 3}


def _x(world: World) -> dict:
    return world.state.extra


def _run(world: World) -> str:
    x = _x(world)
    if "loop_run" not in x:
        x["loop_run"] = f"e2e-loop-{tii.run_id()}"
        x["loop_markers"] = []
        x["loop_resps"] = []
    return x["loop_run"]


def _next_marker(world: World) -> str:
    run = _run(world)
    markers = _x(world)["loop_markers"]
    marker = f"{run}-{len(markers)}"
    markers.append(marker)
    return marker


def _post(world: World, beacon, *, cookie: str | None = None) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if cookie:
        headers["Cookie"] = f"session={cookie}"
    resp = httpx.post(
        f"{tii.gateway_url()}{ep.TRACK}", json=beacon, headers=headers, timeout=15
    )
    x = _x(world)
    x["loop_resps"].append(resp)
    x["tii_resp"] = resp  # consumed by the shared "response is 202 with accepted" step
    return resp


def _beacon(marker: str, type_: str, properties: dict | None, listing: str = "") -> dict:
    body: dict = {"type": type_, "sessionId": marker, "path": "/e2e/loop", "listingId": listing}
    if properties is not None:
        body["properties"] = properties
    return body


def _decode(raw) -> dict:
    assert isinstance(raw, (bytes, bytearray)), f"expected binary protobuf envelope, got {raw!r}"
    out: dict = {
        "event_type": 0,
        "listing_id": "",
        "session_id": "",
        "properties": {},
        "principal_id": "",
        "principal_type": 0,
    }
    for number, wire, value in _fields(bytes(raw)):
        if number == 4 and wire == 2:  # EventEnvelope.principal
            for n, w, v in _fields(bytes(value)):  # type: ignore[arg-type]
                if n == 1 and w == 2:
                    out["principal_id"] = bytes(v).decode()  # type: ignore[arg-type]
                elif n == 2 and w == 0:
                    out["principal_type"] = v
        elif number == 7 and wire == 2:  # EventEnvelope.payload = TrackingEvent
            for n, w, v in _fields(bytes(value)):  # type: ignore[arg-type]
                if n == 1 and w == 0:
                    out["event_type"] = v
                elif n == 2 and w == 2:
                    out["listing_id"] = bytes(v).decode()  # type: ignore[arg-type]
                elif n == 3 and w == 2:
                    out["session_id"] = bytes(v).decode()  # type: ignore[arg-type]
                elif n == 9 and w == 2:  # map<string,string> entry
                    entry = {k: bytes(val).decode() for k, _, val in _fields(bytes(v))}  # type: ignore[arg-type]
                    out["properties"][entry.get(1, "")] = entry.get(2, "")
    return out


def _events(marker: str, *, timeout_s: float = 30.0) -> list[dict]:
    s = get_settings()
    raws = consume_tracking_events(
        s.kafka_brokers, s.kafka_analytics_topic, contains=marker, timeout_s=timeout_s
    )
    return [_decode(r) for r in raws]


def _one_event(world: World, index: int) -> dict:
    marker = _x(world)["loop_markers"][index]
    events = _events(marker)
    assert len(events) == 1, f"expected exactly one event for {marker}, got {len(events)}"
    return events[0]


# ── Posting ──────────────────────────────────────────────────────────────
@when(parsers.parse('a visitor posts a single "{type_}" beacon with no properties'))
def post_no_properties(world: World, type_: str) -> None:
    _post(world, _beacon(_next_marker(world), type_, None))


@when(parsers.parse('a visitor posts a single "{type_}" beacon with properties {props}'))
def post_with_properties(world: World, type_: str, props: str) -> None:
    _post(world, _beacon(_next_marker(world), type_, json.loads(props)))


@when(
    parsers.parse(
        'a visitor posts a single "{type_}" beacon for listing "{listing}" with properties {props}'
    )
)
def post_for_listing(world: World, type_: str, listing: str, props: str) -> None:
    _post(world, _beacon(_next_marker(world), type_, json.loads(props), listing))


@when(
    parsers.parse(
        'a visitor posts a single "{type_}" beacon whose property value has {n:d} characters'
    )
)
def post_long_value(world: World, type_: str, n: int) -> None:
    _post(world, _beacon(_next_marker(world), type_, {"code": "x" * n}))


@when(parsers.parse('a visitor posts a single "{type_}" beacon whose properties has {n:d} keys'))
def post_many_keys(world: World, type_: str, n: int) -> None:
    _post(world, _beacon(_next_marker(world), type_, {f"k{i}": "v" for i in range(n)}))


@when(
    parsers.parse(
        'the signed-in buyer posts a single "{type_}" beacon with the session cookie '
        'and a body userId "{other}"'
    )
)
def post_as_buyer(world: World, type_: str, other: str) -> None:
    buyer = _x(world).get("seeded_buyer")
    assert buyer and buyer.token, "no seeded buyer; scenario must be tagged @needsBuyer"
    beacon = _beacon(_next_marker(world), type_, None)
    beacon["userId"] = other
    _x(world)["loop_buyer_id"] = tii.jwt_subject(buyer.token)
    _post(world, beacon, cookie=buyer.token)


@when(
    parsers.parse(
        'a visitor posts a batch of two "{type_}" beacons, the first with properties {first} '
        "and the second with properties {second}"
    )
)
def post_batch(world: World, type_: str, first: str, second: str) -> None:
    batch = [
        _beacon(_next_marker(world), type_, json.loads(first)),
        _beacon(_next_marker(world), type_, json.loads(second)),
    ]
    _post(world, batch)


# ── Responses ────────────────────────────────────────────────────────────
@then(parsers.parse("every response is {status:d}"))
def every_response_is(world: World, status: int) -> None:
    for resp in _x(world)["loop_resps"]:
        assert resp.status_code == status, (status, resp.status_code, resp.text[:300])


@then(parsers.parse("the responses are {a:d}, {b:d}, {c:d} and {d:d}"))
def responses_are(world: World, a: int, b: int, c: int, d: int) -> None:
    got = [r.status_code for r in _x(world)["loop_resps"]]
    assert got == [a, b, c, d], got


# ── Kafka ────────────────────────────────────────────────────────────────
@then(parsers.parse("exactly one EventEnvelope with EventType {event_type:d} reaches analytics.events"))
def one_event_of_type(world: World, event_type: int) -> None:
    event = _one_event(world, 0)
    assert event["event_type"] == event_type, event


@then(parsers.parse("the events of the beacons have EventTypes {a:d}, {b:d}, {c:d} and {d:d}"))
def events_have_types(world: World, a: int, b: int, c: int, d: int) -> None:
    got = [_one_event(world, i)["event_type"] for i in range(4)]
    assert got == [a, b, c, d], got


@then(parsers.parse("the event of the {nth} beacon carries exactly the properties {props}"))
def event_properties(world: World, nth: str, props: str) -> None:
    assert _one_event(world, _ORD[nth])["properties"] == json.loads(props)


@then(parsers.parse('the event of the {nth} beacon carries listing id "{listing}"'))
def event_listing(world: World, nth: str, listing: str) -> None:
    assert _one_event(world, _ORD[nth])["listing_id"] == listing


@then("the published EventEnvelope principal is the buyer")
def principal_is_buyer(world: World) -> None:
    event = _one_event(world, 0)
    assert event["principal_type"] == _PRINCIPAL_USER, event
    assert event["principal_id"] == _x(world)["loop_buyer_id"], event


@then(parsers.parse('the published EventEnvelope principal is not "{other}"'))
def principal_is_not(world: World, other: str) -> None:
    assert _one_event(world, 0)["principal_id"] != other


@then("the published EventEnvelope principal is of type anonymous")
def principal_anonymous(world: World) -> None:
    assert _one_event(world, 0)["principal_type"] == _PRINCIPAL_ANONYMOUS


@then("nothing is published to analytics.events for the posted beacons")
def nothing_published(world: World) -> None:
    leaked = _events(_run(world), timeout_s=_NEG_TIMEOUT_S)
    assert leaked == [], f"rejected beacons must not produce; found {len(leaked)} event(s)"


@then("only the beacons answered 202 reach analytics.events")
def only_accepted_reach_kafka(world: World) -> None:
    x = _x(world)
    expected = sorted(
        m for m, r in zip(x["loop_markers"], x["loop_resps"], strict=True) if r.status_code == 202
    )
    seen = sorted(e["session_id"] for e in _events(_run(world)))
    assert seen == expected, f"expected {expected}, got {seen}"
    for r in x["loop_resps"]:
        if r.status_code == 202:
            assert r.json() == {"accepted": 1, "dropped": 0}, r.text[:300]


@then("only the first beacon of the batch reaches analytics.events")
def only_first_of_batch(world: World) -> None:
    x = _x(world)
    seen = [e["session_id"] for e in _events(_run(world))]
    assert seen == [x["loop_markers"][0]], f"expected only {x['loop_markers'][0]}, got {seen}"
