"""Shared helpers for the tracking-ingest-integrity scenarios (area tii-e2e).

Black box through the public gateway edge (`POST /api/track`), observed on two surfaces:

* the `analytics.events` Kafka topic (binary protobuf envelopes, decoded here without stubs);
* the team-analytics DuckDB warehouse. The image is distroless (no `docker exec`) and DuckDB
  allows one writer, so the live file cannot be opened concurrently. `warehouse_rows` therefore
  `docker cp`s `analytics.duckdb` and `analytics.duckdb.wal` into a temp dir and opens the COPY
  read-only with the `duckdb` Python package (a read-only open replays the WAL). A copy can be
  torn while the sink writes, so it is polled and re-copied until the expected rows appear.

Container and path come from the environment with the local `agora` compose defaults:

    TEAM_ANALYTICS_CONTAINER   agora-team-analytics-svc
    WAREHOUSE_DB_PATH          /data/analytics.duckdb
"""

from __future__ import annotations

import base64
import json
import os
import shutil
import subprocess
import tempfile
import time
import uuid
from collections.abc import Callable
from pathlib import Path
from typing import Any

import httpx

from config.settings import get_settings
from src.constants import gateway_endpoints as ep
from tests.e2e.flows.tracking_flow import _fields, consume_tracking_events

PASSWORD = "Sup3r-secret-pass!"
WAREHOUSE_TIMEOUT_S = 90.0

# EventEnvelope fields / TrackingEvent fields (proto field numbers).
_ENV_EVENT_ID = 1
_ENV_PAYLOAD = 7
_EV_LISTING_ID = 2
_EV_SESSION_ID = 3
_EV_ANONYMOUS_ID = 4
_EV_PAGE_PATH = 5
_EV_REFERRER = 6
_EV_SEARCH_QUERY = 8


def gateway_url() -> str:
    return get_settings().gateway_url.rstrip("/")


def run_id() -> str:
    return uuid.uuid4().hex[:12]


def view(marker_session: str, **extra: Any) -> dict[str, Any]:
    """A valid view beacon carrying `marker_session` as its session id."""
    body: dict[str, Any] = {
        "type": "view",
        "listingId": f"e2e-tii-{uuid.uuid4().hex[:10]}",
        "sessionId": marker_session,
        "path": "/e2e/tii",
    }
    body.update(extra)
    return body


def post_track(body: Any, token: str | None = None) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    return httpx.post(f"{gateway_url()}{ep.TRACK}", json=body, headers=headers, timeout=15)


def register_buyer() -> tuple[str, str]:
    """Register a brand new buyer through the gateway; returns (token, user id = JWT sub)."""
    username = f"e2e-tii-{uuid.uuid4().hex[:12]}"
    resp = httpx.post(
        f"{gateway_url()}{ep.AUTH_REGISTER}",
        json={"username": username, "password": PASSWORD, "role": "buyer"},
        timeout=30,
    )
    resp.raise_for_status()
    token = (resp.json().get("result") or {}).get("token", "")
    assert token, f"register returned no token: {resp.text[:200]}"
    return token, jwt_subject(token)


def jwt_subject(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return str(json.loads(base64.urlsafe_b64decode(payload))["sub"])


# ── Kafka ────────────────────────────────────────────────────────────────
def kafka_envelopes(marker: str, *, timeout_s: float = 30.0) -> list[dict[str, str]]:
    """Decoded envelopes on analytics.events whose bytes contain `marker`.

    Each item: event_id, listing_id, session_id, anonymous_id, page_path, referrer,
    search_query.
    """
    settings = get_settings()
    raws = consume_tracking_events(
        settings.kafka_brokers,
        settings.kafka_analytics_topic,
        contains=marker,
        timeout_s=timeout_s,
    )
    return [decode_envelope(r) for r in raws]


def _text(value: Any) -> str:
    return bytes(value).decode("utf-8") if isinstance(value, (bytes, bytearray)) else ""


def decode_envelope(raw: Any) -> dict[str, str]:
    assert isinstance(raw, (bytes, bytearray)), f"expected a binary protobuf envelope, got {raw!r}"
    out = dict.fromkeys(
        (
            "event_id",
            "listing_id",
            "session_id",
            "anonymous_id",
            "page_path",
            "referrer",
            "search_query",
        ),
        "",
    )
    names = {
        _EV_LISTING_ID: "listing_id",
        _EV_SESSION_ID: "session_id",
        _EV_ANONYMOUS_ID: "anonymous_id",
        _EV_PAGE_PATH: "page_path",
        _EV_REFERRER: "referrer",
        _EV_SEARCH_QUERY: "search_query",
    }
    for number, wire, value in _fields(bytes(raw)):
        if number == _ENV_EVENT_ID and wire == 2:
            out["event_id"] = _text(value)
        elif number == _ENV_PAYLOAD and wire == 2:
            for inner, inner_wire, inner_value in _fields(bytes(value)):  # type: ignore[arg-type]
                if inner in names and inner_wire == 2:
                    out[names[inner]] = _text(inner_value)
    return out


# ── Warehouse ────────────────────────────────────────────────────────────
def analytics_container() -> str:
    return os.getenv("TEAM_ANALYTICS_CONTAINER", "agora-team-analytics-svc")


def warehouse_path() -> str:
    return os.getenv("WAREHOUSE_DB_PATH", "/data/analytics.duckdb")


def _copy_warehouse(dest: Path) -> Path:
    """docker cp the DuckDB file and its WAL into `dest`; returns the copied db path."""
    src = warehouse_path()
    for suffix in ("", ".wal"):
        res = subprocess.run(
            ["docker", "cp", f"{analytics_container()}:{src}{suffix}", str(dest)],
            capture_output=True,
            text=True,
            timeout=60,
        )
        if res.returncode != 0 and suffix == "":
            raise RuntimeError(f"docker cp of {src} failed: {res.stderr.strip()}")
        # A missing .wal just means the sink has checkpointed; tolerated.
    return dest / Path(src).name


def warehouse_query(sql: str, params: list[Any] | None = None) -> list[tuple]:
    """One read-only query against a fresh copy of the live warehouse."""
    import duckdb  # imported lazily: only the warehouse scenarios need it

    tmp = Path(tempfile.mkdtemp(prefix="tii-wh-"))
    try:
        db = _copy_warehouse(tmp)
        con = duckdb.connect(str(db), read_only=True)
        try:
            return con.execute(sql, params or []).fetchall()
        finally:
            con.close()
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def warehouse_rows(
    sql: str,
    params: list[Any] | None = None,
    *,
    until: Callable[[list[tuple]], bool] = bool,
    settle_s: float = 0.0,
    timeout_s: float = WAREHOUSE_TIMEOUT_S,
) -> list[tuple]:
    """Poll `sql` on fresh copies until `until(rows)` holds, then return the rows.

    The sink batches writes and a copy can be torn (duckdb.Error), so failures are retried.
    With `settle_s`, once `until` holds the query is repeated after that pause and must still
    hold: this is what makes "exactly one row" meaningful rather than "one row so far".
    The last rows (or the last error) are reported when the deadline passes.
    """
    deadline = time.monotonic() + timeout_s
    last: list[tuple] = []
    last_err: Exception | None = None
    while time.monotonic() < deadline:
        try:
            last = warehouse_query(sql, params)
            last_err = None
            if until(last):
                if not settle_s:
                    return last
                time.sleep(settle_s)
                last = warehouse_query(sql, params)
                if until(last):
                    return last
        except Exception as exc:  # noqa: BLE001 - torn copy / not yet created: retry
            last_err = exc
        time.sleep(2.0)
    raise AssertionError(
        f"warehouse never satisfied the expectation within {timeout_s:.0f}s; "
        f"last rows={last!r} last error={last_err!r}"
    )
