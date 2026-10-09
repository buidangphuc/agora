"""Shared helpers for the analytics-data-quality scenarios (area adq-e2e).

Black box through the public gateway: `POST /api/track` for beacons and the Connect JSON RPC
`AnalyticsQueryService/GetTrackingQualityReport` for the report. The sink batches writes, so
every read of "what the report says after my posts" polls until the expectation holds.
"""

from __future__ import annotations

import os
import subprocess
import time
from collections.abc import Callable
from typing import Any

import httpx

from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support import tii_support as tii

REPORT = "/platform.analytics.v1.AnalyticsQueryService/GetTrackingQualityReport"
REPORT_TIMEOUT_S = 90.0
DEFAULT_MISSING_LISTING_MAX = 0.05


def admin_token(world: Any) -> str:
    from src.utils import get_test_data_manager

    admin = get_test_data_manager().get_user_by_role("admin")
    return world.service_factory.auth.login(admin.username, admin.password)


def read_report(token: str | None, window_hours: int | None = 1) -> httpx.Response:
    body: dict[str, Any] = {} if window_hours is None else {"windowHours": window_hours}
    return pe.post_json(REPORT, body, token, timeout=30)


def report_json(token: str, window_hours: int = 1) -> dict[str, Any]:
    resp = read_report(token, window_hours)
    assert resp.status_code == 200, f"report failed: {resp.status_code} {resp.text[:300]}"
    return resp.json()


def type_row(report: dict[str, Any], event_type: str) -> dict[str, Any]:
    """The per-type row for `event_type` (empty dict when the type has no events)."""
    for row in report.get("types", []):
        if row.get("eventType") == event_type:
            return row
    return {}


def poll_report(
    token: str,
    until: Callable[[dict[str, Any]], bool],
    *,
    window_hours: int = 1,
    timeout_s: float = REPORT_TIMEOUT_S,
) -> dict[str, Any]:
    """Re-read the report until `until(report)` holds (the sink batches), else fail."""
    deadline = time.monotonic() + timeout_s
    last: dict[str, Any] = {}
    last_err = ""
    while time.monotonic() < deadline:
        try:
            last = report_json(token, window_hours)
            if until(last):
                return last
        except (AssertionError, httpx.HTTPError) as exc:
            last_err = str(exc)
        time.sleep(2.0)
    raise AssertionError(
        f"the report never satisfied the expectation within {timeout_s:.0f}s; "
        f"last report={last!r} last error={last_err!r}"
    )


def count(report: dict[str, Any], key: str) -> int:
    """Integer field of the report; Connect JSON omits zeros and encodes int64 as strings."""
    return int(report.get(key, 0) or 0)


def post_views(n: int, **extra: Any) -> None:
    """Post `n` valid views (one beacon each, a unique event id each)."""
    import uuid

    for _ in range(n):
        beacon = tii.view(f"e2e-adq-sess-{tii.run_id()}", **extra)
        beacon.setdefault("eventId", str(uuid.uuid4()))
        beacon.setdefault("anonymousId", f"e2e-adq-anon-{uuid.uuid4()}")
        for key in [k for k, v in extra.items() if v is None]:
            beacon.pop(key, None)
        resp = tii.post_track([beacon])
        assert resp.status_code == 202, (resp.status_code, resp.text[:300])
        assert resp.json().get("accepted") == 1, resp.text[:300]


def missing_listing_max() -> float:
    """The configured TRACKING_MISSING_LISTING_MAX_RATIO of the running team-analytics."""
    try:
        env = pe.container_env(tii.analytics_container())
        return float(env.get("TRACKING_MISSING_LISTING_MAX_RATIO", DEFAULT_MISSING_LISTING_MAX))
    except Exception:  # noqa: BLE001 - docker not reachable: fall back to the documented default
        return DEFAULT_MISSING_LISTING_MAX


def dc(*args: str, timeout: int = 240) -> subprocess.CompletedProcess[str]:
    wrapper = os.getenv("DC_WRAPPER", pe.DC_WRAPPER_DEFAULT)
    return subprocess.run(
        [wrapper, *args], check=True, capture_output=True, text=True, timeout=timeout
    )
