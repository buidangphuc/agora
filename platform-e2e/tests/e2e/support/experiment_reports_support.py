"""Helpers for the analytics-experiment-reports scenarios (retention, experiment readout, AI surface).

Black box through the public gateway: `POST /api/track` for beacons and the Connect JSON RPCs
`AnalyticsQueryService/{GetRetentionCohorts,GetExperimentReadout,GetAiSurfaceReport}` for the
reports. Every scenario uses its own flag key or model version so its rows are isolated; the sink
batches writes, so reads poll.
"""

from __future__ import annotations

import time
import uuid
from collections.abc import Callable
from datetime import datetime, timedelta, timezone
from typing import Any

import httpx

from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support import tii_support as tii

SERVICE = "/platform.analytics.v1.AnalyticsQueryService/"
RETENTION = SERVICE + "GetRetentionCohorts"
READOUT = SERVICE + "GetExperimentReadout"
AI_SURFACE = SERVICE + "GetAiSurfaceReport"
POLL_TIMEOUT_S = 90.0
HOLD_S = 8.0


def unique(prefix: str) -> str:
    return f"e2e-exr-{prefix}-{uuid.uuid4().hex[:8]}"


def iso(moment: datetime) -> str:
    return moment.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def now() -> datetime:
    return datetime.now(timezone.utc)


def monday(moment: datetime) -> datetime:
    day = moment.astimezone(timezone.utc).replace(hour=0, minute=0, second=0, microsecond=0)
    return day - timedelta(days=day.weekday())


def post_beacons(beacons: list[dict[str, Any]], token: str | None = None) -> None:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
        headers["Cookie"] = f"session={token}"
    resp = httpx.post(
        f"{tii.gateway_url()}/api/track", json=beacons, headers=headers, timeout=15
    )
    assert resp.status_code == 202, (resp.status_code, resp.text[:300])
    assert resp.json().get("accepted") == len(beacons), resp.text[:300]


def beacon(event_type: str, **extra: Any) -> dict[str, Any]:
    body: dict[str, Any] = {
        "type": event_type,
        "sessionId": unique("sess"),
        "path": "/e2e/exr",
        "eventId": str(uuid.uuid4()),
    }
    body.update(extra)
    return body


def exposure(flag: str, variant: str, anonymous_id: str | None = None) -> dict[str, Any]:
    extra: dict[str, Any] = {"properties": {"flag": flag, "variant": variant}}
    if anonymous_id:
        extra["anonymousId"] = anonymous_id
    return beacon("experiment_exposure", **extra)


def ai_event(surface: str, action: str, model_version: str) -> dict[str, Any]:
    return beacon(
        "ai_interaction",
        anonymousId=unique("anon"),
        modelVersion=model_version,
        properties={"surface": surface, "action": action},
    )


def call(path: str, body: dict[str, Any], token: str | None) -> httpx.Response:
    return pe.post_json(path, body, token, timeout=30)


def readout_body(flag: str, days: int = 1) -> dict[str, Any]:
    return {"flag": flag, "from": iso(now() - timedelta(days=days)), "to": iso(now() + timedelta(minutes=5))}


def ai_body(days: int = 1) -> dict[str, Any]:
    return {"from": iso(now() - timedelta(days=days)), "to": iso(now() + timedelta(minutes=5))}


def retention_body(from_: datetime, weeks: int) -> dict[str, Any]:
    return {"from": iso(from_), "weeks": weeks}


def ok_json(resp: httpx.Response) -> dict[str, Any]:
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    return resp.json()


def poll(
    path: str,
    make_body: Callable[[], dict[str, Any]],
    token: str,
    until: Callable[[dict[str, Any]], bool],
    what: str,
    timeout_s: float = POLL_TIMEOUT_S,
) -> dict[str, Any]:
    deadline = time.monotonic() + timeout_s
    last: Any = None
    while time.monotonic() < deadline:
        last = ok_json(call(path, make_body(), token))
        if until(last):
            return last
        time.sleep(2.0)
    raise AssertionError(f"{what}: never satisfied within {timeout_s:.0f}s, last={last!r}")


def variant_row(report: dict[str, Any], variant: str) -> dict[str, Any]:
    for row in report.get("variants", []):
        if row.get("variant") == variant:
            return row
    return {}


def ai_row(report: dict[str, Any], surface: str, model_version: str) -> dict[str, Any]:
    for row in report.get("rows", []):
        if row.get("surface") == surface and row.get("modelVersion") == model_version:
            return row
    return {}


def num(row: dict[str, Any], key: str) -> int:
    return int(row.get(key, 0) or 0)
