"""Helpers for tracking/loop_events.feature (frontend-loop-events).

Black box: events are read from the `analytics.events` topic (binary protobuf envelopes),
the browser is observed through `window.dataLayer`, `document.cookie` and the `/api/track`
request bodies. Nothing here reads frontend code.
"""

from __future__ import annotations

import base64
import json
import time

from config.settings import get_settings
from tests.e2e.flows.tracking_flow import _fields, consume_tracking_events

# platform.analytics.v1.EventType numbers (see tracking_flow._EVENT_TYPES and the loop intake slice)
FAVORITE = 9
SHARE = 10
SIGN_UP = 15
LOGIN = 16
FOLLOW_SELLER = 17
REFERRAL_REDEEM = 18
DAILY_CHECK_IN = 19
ALERT_SUBSCRIBE = 20

NEGATIVE_WAIT_S = 10.0
POSITIVE_WAIT_S = 10.0
SETTLE_S = 2.5

AUTH_COOKIE = "agora_auth_evt"

# navigator.sendBeacon bodies are Blobs Playwright cannot read from the request: tap them.
_TAP = """
(() => {
  const original = navigator.sendBeacon.bind(navigator);
  navigator.sendBeacon = (url, data) => {
    try {
      if (String(url).includes("/api/track") && window.__e2eLoopRecord) {
        Promise.resolve(data && data.text ? data.text() : String(data))
          .then((t) => window.__e2eLoopRecord(t)).catch(() => {});
      }
    } catch (e) {}
    return original(url, data);
  };
})();
"""


# ── Browser observation ──────────────────────────────────────────────────
def start_beacon_recording(world) -> list[dict]:  # noqa: ANN001
    """Record every beacon the page sends to /api/track (call before navigating)."""
    beacons: list[dict] = []
    seen: set[str] = set()

    def collect(body: str | None) -> None:
        if not body or body in seen:
            return
        try:
            payload = json.loads(body)
        except ValueError:
            return
        seen.add(body)
        beacons.extend(payload if isinstance(payload, list) else [payload])

    def on_request(request) -> None:  # noqa: ANN001
        if "/api/track" in request.url:
            collect(request.post_data)

    world.page.on("request", on_request)
    world.page.expose_function("__e2eLoopRecord", collect)
    world.page.add_init_script(_TAP)
    world.state.extra["loop_beacons"] = beacons
    return beacons


def recorded_session_ids(world) -> set[str]:  # noqa: ANN001
    return {b["sessionId"] for b in world.state.extra.get("loop_beacons", []) if b.get("sessionId")}


def datalayer_entries(page, name: str) -> list[dict]:  # noqa: ANN001
    """Entries of window.dataLayer whose `event` equals `name`."""
    entries = page.evaluate(
        "() => (window.dataLayer || []).map((e) => JSON.parse(JSON.stringify(e)))"
    )
    return [e for e in entries if isinstance(e, dict) and e.get("event") == name]


def document_cookies(page) -> dict[str, str]:  # noqa: ANN001
    raw = page.evaluate("() => document.cookie") or ""
    out: dict[str, str] = {}
    for part in raw.split(";"):
        if "=" in part:
            k, v = part.strip().split("=", 1)
            out[k] = v
    return out


def jwt_subject(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload)).get("sub", "")


def session_token(world) -> str:  # noqa: ANN001
    for c in world.context.cookies():
        if c["name"] == "session":
            return c["value"]
    return ""


# ── analytics.events ─────────────────────────────────────────────────────
def decode(raw) -> dict:
    """Decode an EventEnvelope: event type, listing, session, properties, principal."""
    assert isinstance(raw, (bytes, bytearray)), f"expected binary protobuf envelope, got {raw!r}"
    out: dict = {
        "event_type": 0,
        "listing_id": "",
        "session_id": "",
        "properties": {},
        "principal_id": "",
        "principal_type": 0,
        "raw": bytes(raw),
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


def events_for(marker: str, event_type: int, *, listing_id: str | None = None) -> list[dict]:
    """Events of `event_type` on analytics.events whose envelope carries `marker`."""
    s = get_settings()
    raws = consume_tracking_events(
        s.kafka_brokers, s.kafka_analytics_topic, contains=marker, timeout_s=4.0, settle_s=1.5
    )
    events = [decode(r) for r in raws]
    events = [e for e in events if e["event_type"] == event_type]
    if listing_id is not None:
        events = [e for e in events if e["listing_id"] == listing_id]
    return events


def wait_for_events(
    markers: list[str] | str,
    event_type: int,
    count: int,
    *,
    listing_id: str | None = None,
    within_s: float = POSITIVE_WAIT_S,
) -> list[dict]:
    """Poll until at least `count` events of the type were seen for any marker, or time is up.

    Returns the events seen at the last poll (may be fewer than `count` on timeout, so the
    caller asserts on the exact number).
    """
    if isinstance(markers, str):
        markers = [markers]
    deadline = time.time() + within_s
    found: list[dict] = []
    while True:
        found = _across(markers, event_type, listing_id)
        if len(found) >= count or time.time() >= deadline:
            return found
        time.sleep(1.0)


def settled_events(
    markers: list[str] | str,
    event_type: int,
    count: int,
    *,
    listing_id: str | None = None,
    within_s: float = POSITIVE_WAIT_S,
) -> list[dict]:
    """Wait for `count` events, let the topic settle, then re-read so extras are visible."""
    wait_for_events(markers, event_type, count, listing_id=listing_id, within_s=within_s)
    time.sleep(SETTLE_S)
    if isinstance(markers, str):
        markers = [markers]
    return _across(markers, event_type, listing_id)


def quiet_events(
    markers: list[str] | str, event_type: int, *, listing_id: str | None = None
) -> list[dict]:
    """Wait the negative window in full, then return what is on the topic."""
    time.sleep(NEGATIVE_WAIT_S)
    if isinstance(markers, str):
        markers = [markers]
    return _across(markers, event_type, listing_id)


def _across(markers: list[str], event_type: int, listing_id: str | None) -> list[dict]:
    seen: dict[bytes, dict] = {}
    for marker in markers:
        for event in events_for(marker, event_type, listing_id=listing_id):
            seen[event["raw"]] = event
    return list(seen.values())
