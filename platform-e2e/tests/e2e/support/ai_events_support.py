"""Helpers for tracking/ai_interaction_events.feature (frontend-ai-interaction-events).

Black box: `ai_interaction` beacons are observed in the browser through the same
`navigator.sendBeacon` tap the loop-event features use (`loop_events_support._TAP`, imported, not
copied), grouped per `/api/track` batch, together with the gateway's HTTP status for each request.
Nothing here reads frontend code.
"""

from __future__ import annotations

import json
import time

from tests.e2e.support import loop_events_support as lp

ALLOWED_KEYS = {"surface", "action"}
SETTLE_S = 6.0  # the tracking queue flushes after ~2 s idle; leave margin
POSITIVE_WAIT_S = 15.0

_TAP = lp._TAP.replace("__e2eLoopRecord", "__e2eAiRecord")


def start_recording(world) -> None:  # noqa: ANN001
    """Record every /api/track batch (body + gateway status). Call before navigating."""
    batches: list[list[dict]] = []
    statuses: list[dict] = []
    seen: set[str] = set()

    def collect(body: str | None) -> None:
        if not body or body in seen:
            return
        try:
            payload = json.loads(body)
        except ValueError:
            return
        seen.add(body)
        batches.append(payload if isinstance(payload, list) else [payload])

    def on_request(request) -> None:  # noqa: ANN001
        if "/api/track" in request.url:
            collect(request.post_data)

    def on_response(response) -> None:  # noqa: ANN001
        if "/api/track" not in response.url:
            return
        try:
            body = response.request.post_data or ""
        except Exception:  # noqa: BLE001
            body = ""
        statuses.append({"status": response.status, "has_ai": "ai_interaction" in body, "body": body})

    world.page.on("request", on_request)
    world.page.on("response", on_response)
    world.page.expose_function("__e2eAiRecord", collect)
    world.page.add_init_script(_TAP)
    world.state.extra["ai_batches"] = batches
    world.state.extra["ai_statuses"] = statuses


def all_beacons(world) -> list[dict]:  # noqa: ANN001
    return [b for batch in world.state.extra.get("ai_batches", []) for b in batch]


def ai_events(world, surface: str | None = None, action: str | None = None) -> list[dict]:  # noqa: ANN001
    """`ai_interaction` beacons captured so far, optionally filtered by surface / action."""
    out = []
    for beacon in all_beacons(world):
        if beacon.get("type") != "ai_interaction":
            continue
        props = beacon.get("properties") or {}
        if surface is not None and props.get("surface") != surface:
            continue
        if action is not None and props.get("action") != action:
            continue
        out.append(beacon)
    return out


def wait_for(world, count: int, surface=None, action=None, within_s=POSITIVE_WAIT_S) -> list[dict]:  # noqa: ANN001
    """Poll until `count` matching events were seen (or time is up); returns what was seen."""
    deadline = time.time() + within_s
    while True:
        found = ai_events(world, surface, action)
        if len(found) >= count or time.time() >= deadline:
            return found
        world.page.wait_for_timeout(500)


def settled(world, count: int, surface=None, action=None) -> list[dict]:  # noqa: ANN001
    """Wait for `count`, then let the queue flush once more so extra events become visible."""
    wait_for(world, count, surface, action)
    world.page.wait_for_timeout(int(SETTLE_S * 1000))
    return ai_events(world, surface, action)


def quiet(world, surface=None, action=None) -> list[dict]:  # noqa: ANN001
    """Wait the full negative window, then return what was captured."""
    world.page.wait_for_timeout(int(SETTLE_S * 1000))
    return ai_events(world, surface, action)


def describe(events: list[dict]) -> str:
    return json.dumps(
        [{"p": e.get("properties"), "listingId": e.get("listingId")} for e in events],
        ensure_ascii=False,
    )
