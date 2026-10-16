"""Binds tracking/loop_event_storage.feature (change analytics-loop-event-storage).

Black box: the TrackingEvent payload is hand-encoded here (field numbers from the public
analytics.proto), wrapped with ga4_relay_support.envelope, produced straight onto
analytics.events and read back from the warehouse via tii_support.warehouse_rows.
"""

from __future__ import annotations

import json
import uuid

from pytest_bdd import scenarios, then, when

from config.settings import get_settings
from tests.e2e.support import ga4_relay_support as relay
from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World

scenarios("tracking/loop_event_storage.feature")

# EventType numbers 15-21 from platform.analytics.v1.EventType.
_NEW_TYPES = {
    15: "sign_up",
    16: "login",
    17: "follow_seller",
    18: "referral_redeem",
    19: "daily_check_in",
    20: "alert_subscribe",
    21: "ai_interaction",
}
_NAME_TO_NUMBER = {v: k for k, v in _NEW_TYPES.items()}
_SETTLE_S = 10.0


def _x(world: World) -> dict:
    return world.state.extra


def _payload(
    event_type: int,
    session: str,
    *,
    listing_id: str = "",
    properties: dict[str, str] | None = None,
    ga_client_id: str = "",
    analytics: bool | None = None,
) -> bytes:
    body = relay.f_varint(1, event_type) + relay.f_bytes(2, listing_id) + relay.f_bytes(3, session)
    for key, value in (properties or {}).items():
        body += relay.f_bytes(9, relay.f_bytes(1, key) + relay.f_bytes(2, value))
    body += relay.f_bytes(25, ga_client_id)
    if analytics is not None:
        body += relay.f_bytes(26, relay.f_varint(1, 1 if analytics else 0) + relay.f_varint(4, 1))
    return body


def _send(raw: bytes) -> None:
    relay.produce(get_settings().kafka_analytics_topic, relay.uid("loop"), raw)


@when(
    "one tracking event of each new loop type, each with a unique session id, "
    "is produced to analytics.events"
)
def produce_each_type(world: World) -> None:
    sessions: dict[str, str] = {}
    for number, name in _NEW_TYPES.items():
        session = f"e2e-loop-{name}-{uuid.uuid4().hex[:10]}"
        sessions[name] = session
        raw = relay.envelope(relay.TRACKING_TYPE, _payload(number, session), "e2e-loop-principal")
        _send(raw)
    _x(world)["loop_sessions"] = sessions


@then("the warehouse holds exactly one row per produced event whose event_type is the matching lowercase name")
def one_row_per_type(world: World) -> None:
    for name, session in _x(world)["loop_sessions"].items():
        rows = tii.warehouse_rows(
            "SELECT event_id, event_type FROM tracking_events WHERE session_id = ?",
            [session],
            until=lambda r: len(r) >= 1,
            timeout_s=30.0,
            settle_s=2.0,
        )
        assert len(rows) == 1, (name, rows)
        assert rows[0][1] == name, f"{name}: stored as {rows[0][1]!r}"


@when(
    'an ai_interaction event with properties surface=assistant and action=accepted '
    'and listing id "l-1" is produced by a principal'
)
def produce_ai_interaction(world: World) -> None:
    x = _x(world)
    x["loop_principal"] = f"e2e-loop-user-{uuid.uuid4().hex[:8]}"
    x["loop_session"] = f"e2e-loop-props-{uuid.uuid4().hex[:10]}"
    payload = _payload(
        _NAME_TO_NUMBER["ai_interaction"],
        x["loop_session"],
        listing_id="l-1",
        properties={"surface": "assistant", "action": "accepted"},
    )
    _send(relay.envelope(relay.TRACKING_TYPE, payload, x["loop_principal"]))


@then("the warehouse row carries exactly those properties, that listing id and the principal id")
def props_preserved(world: World) -> None:
    x = _x(world)
    rows = tii.warehouse_rows(
        "SELECT event_type, properties, listing_id, principal_id FROM tracking_events "
        "WHERE session_id = ?",
        [x["loop_session"]],
        timeout_s=30.0,
    )
    assert len(rows) == 1, rows
    event_type, props, listing, principal = rows[0]
    assert event_type == "ai_interaction", f"stored as {event_type!r}"
    assert json.loads(props) == {"surface": "assistant", "action": "accepted"}, props
    assert listing == "l-1", listing
    assert principal == x["loop_principal"], principal


@when("a sign_up event carrying a ga_client_id and analytics consent false is produced to analytics.events")
def produce_sign_up_no_consent(world: World) -> None:
    x = _x(world)
    x["loop_session"] = f"e2e-loop-consent-{uuid.uuid4().hex[:10]}"
    payload = _payload(
        _NAME_TO_NUMBER["sign_up"],
        x["loop_session"],
        ga_client_id="GA1.1.e2e.loop",
        analytics=False,
    )
    _send(relay.envelope(relay.TRACKING_TYPE, payload, f"e2e-loop-user-{uuid.uuid4().hex[:8]}"))


@then("the warehouse row for it has an empty ga_client_id")
def client_id_blank(world: World) -> None:
    rows = tii.warehouse_rows(
        "SELECT event_type, ga_client_id FROM tracking_events WHERE session_id = ?",
        [_x(world)["loop_session"]],
        timeout_s=30.0,
    )
    assert len(rows) == 1, rows
    assert rows[0][0] == "sign_up", f"stored as {rows[0][0]!r}"
    assert not rows[0][1], f"ga_client_id not blanked: {rows[0][1]!r}"


@when("the same login envelope is produced to analytics.events twice")
def produce_login_twice(world: World) -> None:
    x = _x(world)
    x["loop_session"] = f"e2e-loop-replay-{uuid.uuid4().hex[:10]}"
    raw = relay.envelope(
        relay.TRACKING_TYPE,
        _payload(_NAME_TO_NUMBER["login"], x["loop_session"]),
        f"e2e-loop-user-{uuid.uuid4().hex[:8]}",
    )
    _send(raw)
    _send(raw)


@then("the warehouse holds exactly one row for that event id after a 10 second settle")
def replay_one_row(world: World) -> None:
    rows = tii.warehouse_rows(
        "SELECT event_id, event_type FROM tracking_events WHERE session_id = ?",
        [_x(world)["loop_session"]],
        until=lambda r: len(r) == 1 and r[0][1] == "login",
        settle_s=_SETTLE_S,
        timeout_s=45.0,
    )
    assert len(rows) == 1, rows
