"""Helpers for the recommendation online evaluation scenarios (area roe-e2e).

Black box through the gateway: `POST /api/track` for recommendation beacons and the Connect JSON
RPC `AnalyticsQueryService/GetRecommendationPerformance` for the report. Every scenario uses its
own placement id and model version (`e2e-roe-...`) so its rows are isolated from other tests; the
sink batches writes, so reads poll.
"""

from __future__ import annotations

import time
import uuid
from collections.abc import Callable
from typing import Any

import httpx

from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support import tii_support as tii

REPORT = "/platform.analytics.v1.AnalyticsQueryService/GetRecommendationPerformance"
POLL_TIMEOUT_S = 60.0


def unique(prefix: str) -> str:
    return f"e2e-roe-{prefix}-{uuid.uuid4().hex[:10]}"


def beacon(
    event_type: str,
    *,
    listing_id: str,
    anonymous_id: str,
    impression_id: str,
    placement_id: str,
    model_version: str,
) -> dict[str, Any]:
    body = tii.view(
        f"e2e-roe-sess-{tii.run_id()}",
        type=event_type,
        listingId=listing_id,
        eventId=str(uuid.uuid4()),
        anonymousId=anonymous_id,
        impressionId=impression_id,
        placementId=placement_id,
        modelVersion=model_version,
    )
    return body


def post(beacons: list[dict[str, Any]], token: str | None = None) -> None:
    resp = tii.post_track(beacons, token)
    assert resp.status_code == 202, (resp.status_code, resp.text[:300])
    assert resp.json().get("accepted") == len(beacons), resp.text[:300]


def read_report(token: str | None, window_hours: int = 1) -> httpx.Response:
    return pe.post_json(REPORT, {"windowHours": window_hours}, token, timeout=30)


def report_json(token: str) -> dict[str, Any]:
    resp = read_report(token)
    assert resp.status_code == 200, f"report failed: {resp.status_code} {resp.text[:300]}"
    return resp.json()


def poll_report(
    token: str, until: Callable[[dict[str, Any]], bool], *, timeout_s: float = POLL_TIMEOUT_S
) -> dict[str, Any]:
    deadline = time.monotonic() + timeout_s
    last: dict[str, Any] = {}
    last_err = ""
    while time.monotonic() < deadline:
        try:
            last = report_json(token)
            if until(last):
                return last
        except (AssertionError, httpx.HTTPError) as exc:
            last_err = str(exc)
        time.sleep(2.0)
    raise AssertionError(
        f"the report never satisfied the expectation within {timeout_s:.0f}s; "
        f"last report={last!r} last error={last_err!r}"
    )


def row(report: dict[str, Any], placement: str, model: str) -> dict[str, Any]:
    """The (placement, model) row, or {} when absent. Zero values are omitted by Connect JSON."""
    for r in report.get("rows", []):
        if r.get("placementId") == placement and r.get("modelVersion") == model:
            return r
    return {}


def num(r: dict[str, Any], key: str) -> int:
    return int(r.get(key, 0) or 0)


def fallback_share(report: dict[str, Any], placement: str) -> float:
    for f in report.get("fallback", []):
        if f.get("placementId") == placement:
            return float(f.get("fallbackShare", 0) or 0)
    return 0.0
