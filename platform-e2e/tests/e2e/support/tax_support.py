"""Helpers for the tag taxonomy scenarios (area tax): black-box HTTP against team-ai.

The tag routes (`/api/v1/ai/tags/*`) are internal, not routed by the gateway, so the scenarios
call the running team-ai container directly on the host port compose publishes (8001 -> 8000):

    TEAM_AI_URL   default http://localhost:8001

team-ai keeps the taxonomy in process memory and promotion cannot be undone, so every run mints
its own wattage (hence its own `cong-suat-<W>w` slug) that no other test or run uses.
"""

from __future__ import annotations

import os
import random
from typing import Any

import httpx

TAGS = "/api/v1/ai/tags"


def base_url() -> str:
    return os.getenv("TEAM_AI_URL", "http://localhost:8001").rstrip("/")


def post(path: str, body: dict[str, Any]) -> httpx.Response:
    return httpx.post(base_url() + TAGS + path, json=body, timeout=30.0)


def list_tags(status: str = "") -> list[dict[str, Any]]:
    r = httpx.get(base_url() + TAGS, params={"status": status} if status else {}, timeout=30.0)
    assert r.status_code == 200, r.text
    return r.json()["tags"]


def fresh_watts(n: int) -> list[int]:
    """`n` wattages (100..999, so the slug `cong-suat-<W>w` is a 3-digit one) not in the taxonomy."""
    taken = {t["slug"] for t in list_tags()}
    pool = [w for w in range(100, 1000) if f"cong-suat-{w}w" not in taken]
    assert len(pool) >= n, "team-ai taxonomy has no unused wattage left"
    return random.sample(pool, n)


def slug_for(watt: int) -> str:
    return f"cong-suat-{watt}w"
