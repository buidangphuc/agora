"""Tracking-collector edge-policy steps (port-edge-authz-residuals, edge-stream-and-http-policy).

The beacon flood is sent as one anonymous visitor straight at `POST /api/track` on the
gateway; the proof that a throttled request produced nothing is read from the
`analytics.events` Kafka topic with the existing consumer helper (KAFKA_BROKERS).
"""

from __future__ import annotations

import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from tests.e2e.flows import consume_tracking_events
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

FLOOD = 40


TRACK_BURST_MAX = 200


def _x(world: World) -> dict:
    return world.state.extra


def _beacon(marker: str) -> list[dict]:
    return [{"type": "view", "listingId": "e2e-edge", "sessionId": marker, "path": "/e2e/edge"}]


# ── beacon flood ─────────────────────────────────────────────────────────
@given("the analytics.events topic is readable")
def topic_readable(world: World) -> None:
    # An unmatched marker scans the topic end to end quickly when it is small; this only
    # proves the consumer can reach the broker, so the later "none reached" check is not vacuous.
    consume_tracking_events(
        world.settings.kafka_brokers,
        world.settings.kafka_analytics_topic,
        contains=f"e2e-probe-{uuid.uuid4().hex}",
        timeout_s=3,
    )


@when(
    parsers.parse(
        "one anonymous visitor sends {count:d} POST /api/track batches back to back, "
        "each with one event carrying a unique marker"
    )
)
def send_flood(world: World, count: int) -> None:
    x = _x(world)
    run = uuid.uuid4().hex[:10]
    x["pe_run"] = f"e2e-flood-{run}-"
    sent: list[tuple[str, int]] = []
    base = pe.gateway_url()
    burst = int(pe.container_env(pe.gateway_container()).get("TRACK_RATE_LIMIT_BURST", "20"))
    if burst > TRACK_BURST_MAX:
        # The stack raises the collector limit for parallel e2e (every browser shares the
        # docker host IP); exceed a gateway that runs the shipped defaults instead.
        base = pe.private_gateway(
            world, {"TRACK_RATE_LIMIT_RPS": "5", "TRACK_RATE_LIMIT_BURST": "20"}
        )
    with httpx.Client(timeout=15) as client:
        for i in range(count):
            marker = f"{x['pe_run']}{i:03d}"
            resp = client.post(f"{base}/api/track", json=_beacon(marker))
            sent.append((marker, resp.status_code))
    x["pe_sent"] = sent


@then("some requests answer HTTP 429")
def some_429(world: World) -> None:
    statuses = [s for _, s in _x(world)["pe_sent"]]
    assert 429 in statuses, f"no request was throttled: {sorted(set(statuses))}"
    assert any(s in (200, 202, 204) for s in statuses), f"nothing was accepted: {set(statuses)}"


@then("none of the markers of the 429 requests reach analytics.events")
def throttled_markers_absent(world: World) -> None:
    x = _x(world)
    sent: list[tuple[str, int]] = x["pe_sent"]
    throttled = {m for m, s in sent if s == 429}
    accepted = {m for m, s in sent if s in (200, 202, 204)}
    envelopes = consume_tracking_events(
        world.settings.kafka_brokers,
        world.settings.kafka_analytics_topic,
        contains=x["pe_run"],
        timeout_s=30,
        max_events=5000,
    )
    text = "\n".join(
        e.decode("utf-8", errors="replace") if isinstance(e, (bytes, bytearray)) else str(e)
        for e in envelopes
    )
    leaked = sorted(m for m in throttled if m in text)
    assert not leaked, f"{len(leaked)} throttled markers reached analytics.events: {leaked[:5]}"
    # Control: the topic is being read, and accepted beacons do produce their marker.
    assert any(m in text for m in accepted), "no accepted marker was found on analytics.events"


# ── request id echo ──────────────────────────────────────────────────────
@when(
    parsers.parse(
        'a visitor sends one valid POST /api/track batch with a random X-Request-Id starting "{prefix}"'
    )
)
def send_one_beacon(world: World, prefix: str) -> None:
    x = _x(world)
    x["pe_request_id"] = pe.rid(prefix.rstrip("-"))
    x["pe_track"] = httpx.post(
        f"{pe.gateway_url()}/api/track",
        json=_beacon(f"e2e-track-{uuid.uuid4().hex[:10]}"),
        headers={"X-Request-Id": x["pe_request_id"]},
        timeout=15,
    )


@then("the track response is accepted and carries that X-Request-Id")
def track_echoes(world: World) -> None:
    x = _x(world)
    resp: httpx.Response = x["pe_track"]
    assert resp.status_code in (200, 202, 204), (resp.status_code, resp.text[:200])
    assert resp.headers.get("x-request-id") == x["pe_request_id"], dict(resp.headers)
