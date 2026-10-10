"""Helpers for the tag taxonomy scenarios (area tax): black-box HTTP against team-ai.

The tag routes (`/api/v1/ai/tags/*`) are internal, not routed by the gateway, so the scenarios
call the running team-ai container directly on the host port compose publishes (8001 -> 8000):

    TEAM_AI_URL            default http://localhost:8001
    TEAM_AI_SERVICE_TOKEN  bearer of the read-only service principal (scope ai.classify)
    TEAM_AI_ADMIN_TOKEN    bearer of the admin principal (AUTH_ADMIN_BEARER_TOKEN), for explore/promote

Since change tag-routes-authz every tag route needs one of them; the defaults are the local-dev values the
compose stack gives team-ai (design.md "Deployment needs").

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


def service_token() -> str:
    return os.getenv("TEAM_AI_SERVICE_TOKEN", "agora-local-team-ai-service")


def admin_token() -> str:
    return os.getenv("TEAM_AI_ADMIN_TOKEN", "agora-local-team-ai-admin")


MUTATING = ("/explore", "/promote")
ANONYMOUS = object()  # `token=ANONYMOUS`: send no Authorization header


def _headers(path: str, token: Any) -> dict[str, str]:
    if token is ANONYMOUS:
        return {}
    if token is None:  # default: the least privileged principal the route accepts
        token = admin_token() if path in MUTATING else service_token()
    return {"Authorization": f"Bearer {token}"}


def post(path: str, body: dict[str, Any], token: Any = None) -> httpx.Response:
    return httpx.post(
        base_url() + TAGS + path, json=body, headers=_headers(path, token), timeout=30.0
    )


def get(params: dict[str, str] | None = None, token: Any = None) -> httpx.Response:
    return httpx.get(base_url() + TAGS, params=params, headers=_headers("", token), timeout=30.0)


def list_tags(status: str = "") -> list[dict[str, Any]]:
    r = get({"status": status} if status else {})
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
