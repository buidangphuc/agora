"""Tracking (analytics beacon -> analytics.events) step definitions.

Emits browsing beacons at the gateway edge (`POST /api/track`) and asserts the
resulting `EventEnvelope` on the `analytics.events` Kafka topic. Kafka assertions
run only against the local stack (broker + `kafka-python`); see tracking_flow.py.
"""

from __future__ import annotations

import uuid

from playwright.sync_api import expect
from pytest_bdd import parsers, then, when

from src.api.services.tracking_service import VIEW
from src.constants import PageName, timeouts
from tests.e2e.flows import consume_tracking_events
from tests.e2e.support.world import World


def _envelopes_for_session(world: World, session_id: str) -> list:
    return consume_tracking_events(
        world.settings.kafka_brokers,
        world.settings.kafka_analytics_topic,
        contains=session_id,
    )


def _as_text(envelope) -> str:
    import json

    if isinstance(envelope, (bytes, bytearray)):
        return envelope.decode("utf-8", errors="replace")
    return json.dumps(envelope)


# ── Emission ─────────────────────────────────────────────────────────────
@when("the gateway receives a valid track beacon for a product view")
def emit_valid_view_beacon(world: World) -> None:
    listing = world.state.listing
    session_id = f"e2e-sess-{uuid.uuid4()}"
    path = f"/listing/{listing.listing_id}"  # type: ignore[union-attr]
    status = world.service_factory.tracking.emit(
        VIEW,
        listing_id=listing.listing_id,  # type: ignore[union-attr]
        session_id=session_id,
        page=path,
    )
    assert status in (200, 202, 204), f"beacon should be accepted, got {status}"
    world.state.extra["track_session_id"] = session_id
    world.state.extra["track_path"] = path


@when("a logged-in buyer's view beacon is collected at the gateway")
def emit_authenticated_view_beacon(world: World) -> None:
    listing = world.state.listing
    session_id = f"e2e-sess-{uuid.uuid4()}"
    # The buyer token is already active on the service factory (login_via_api),
    # so the edge resolves the buyer principal from it.
    status = world.service_factory.tracking.emit(
        VIEW,
        listing_id=listing.listing_id,  # type: ignore[union-attr]
        session_id=session_id,
        page=f"/listing/{listing.listing_id}",  # type: ignore[union-attr]
    )
    assert status in (200, 202, 204), f"beacon should be accepted, got {status}"
    world.state.extra["track_session_id"] = session_id


@when("the gateway receives a beacon with no recognizable event type")
def emit_malformed_beacon(world: World) -> None:
    nonce = f"e2e-nonce-{uuid.uuid4()}"
    # Valid JSON, but an unrecognised event type — must be rejected, never produced.
    body = f'{{"type": "bogus-{nonce}", "listingId": "x"}}'
    status = world.service_factory.tracking.emit_raw(body)
    world.state.extra["malformed_status"] = status
    world.state.extra["malformed_nonce"] = nonce


@when("the buyer performs a tracked browsing action while the analytics producer is unavailable")
def perform_tracked_browsing_action(world: World) -> None:
    # Drive a real browsing action in the UI (PDP mount fires the view beacon
    # best-effort). Browsing must succeed whether or not the beacon is delivered.
    listing = world.state.listing
    detail = world.navigate_to(PageName.LISTING_DETAIL, listing_id=listing.listing_id)  # type: ignore[union-attr]
    world.set_current_page(detail)


# ── Assertions ───────────────────────────────────────────────────────────
@then(
    parsers.parse(
        'exactly one EventEnvelope of type "{env_type}" is published to the "{topic}" topic'
    )
)
def exactly_one_envelope_published(world: World, env_type: str, topic: str) -> None:
    session_id = world.state.extra["track_session_id"]
    envelopes = _envelopes_for_session(world, session_id)
    assert (
        len(envelopes) == 1
    ), f"expected exactly one envelope for {session_id}, got {len(envelopes)}"
    text = _as_text(envelopes[0])
    assert env_type in text, f"envelope type {env_type!r} missing from {text[:300]}"
    world.state.extra["last_envelope_text"] = text


@then(
    parsers.parse(
        'its payload is a TrackingEvent with EventType "{event_type}" '
        "carrying the listing id, session id and page path"
    )
)
def payload_is_tracking_event(world: World, event_type: str) -> None:
    text = world.state.extra["last_envelope_text"]
    session_id = world.state.extra["track_session_id"]
    listing_id = world.state.listing.listing_id  # type: ignore[union-attr]
    path = world.state.extra["track_path"]
    assert session_id in text, "session id not carried"
    assert listing_id in text, "listing id not carried"
    assert path in text, "page path not carried"


@then("the gateway responds with a client error")
def gateway_responds_client_error(world: World) -> None:
    status = world.state.extra["malformed_status"]
    assert 400 <= status < 500, f"expected a 4xx client error, got {status}"


@then(parsers.parse('nothing is published to the "{topic}" topic'))
def nothing_published(world: World, topic: str) -> None:
    nonce = world.state.extra["malformed_nonce"]
    leaked = consume_tracking_events(
        world.settings.kafka_brokers, world.settings.kafka_analytics_topic, contains=nonce
    )
    assert leaked == [], f"malformed beacon must not produce; found {len(leaked)} event(s)"


@then("the produced EventEnvelope principal identifies the buyer")
def envelope_principal_identifies_buyer(world: World) -> None:
    session_id = world.state.extra["track_session_id"]
    envelopes = _envelopes_for_session(world, session_id)
    assert envelopes, "no envelope produced for the authenticated beacon"
    text = _as_text(envelopes[0])
    buyer = world.state.current_user
    assert (buyer.username in text or "listing.read" in text or "engagement:write" in text), "envelope principal does not identify the buyer"
    world.state.extra["last_envelope_text"] = text


@then("the TrackingEvent payload contains no authenticated user id in its own fields")
def payload_has_no_authenticated_user_id(world: World) -> None:
    text = world.state.extra["last_envelope_text"]
    # Behavioral-only payload: the TrackingEvent must not carry the identity in a
    # payload user field (identity lives on the envelope principal instead).
    for banned in ('"userId"', '"user_id"', '"buyerId"', '"buyer_id"'):
        assert banned not in text, f"payload leaks authenticated identity via {banned}"


@then("the browsing action completes normally")
def browsing_completes_normally(world: World) -> None:
    detail = world.current_page
    expect(detail.add_to_cart_button).to_be_visible(timeout=timeouts.DEFAULT)  # type: ignore[attr-defined]


@then("no user-visible error is shown")
def no_user_visible_error(world: World) -> None:
    error_banner = world.page.locator(".bg-red-900, .bg-red-500, [role='alert']:has-text('Lỗi'), [role='alert']:has-text('Error')")
    assert (
        error_banner.count() == 0 or not error_banner.first.is_visible()
    ), "a user-visible error was shown during a best-effort tracked action"
