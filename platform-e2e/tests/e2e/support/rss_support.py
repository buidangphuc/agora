"""Shared helpers for the recs-serving-safeguards scenarios (area rss-e2e).

Black box: recommendations are read through the gateway (Connect JSON); the serving keys and the
feature-store online keys are read and written in the stack's Redis (host-published port, same
RESP client the featurestore scenarios use). Every key a scenario writes is saved first and
restored exactly (value and TTL), also when the scenario fails.
"""

from __future__ import annotations

import base64
import json
import os
import subprocess
import time
from typing import Any

from src.api.services import GatewayError
from src.api.services.recommendation_service import CONTEXT_HOMEPAGE
from tests.e2e.flows.fsm_job_flow import Online
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

SERVING_DB = 0  # team-ai's RECS_* cache (REDIS_DATABASE of the stack)
FEATURES_DB = 2  # the feature store's online DB
SERVING_KEY = "recs:v1:serving"
FALLBACK_VERSION = "serving-fallback"
SETTLE_S = 20.0  # the 5 s pointer memo plus slack
POLL_S = 0.5
REDIS_CONTAINER = os.getenv("REDIS_CONTAINER", "agora-redis-1")


def wrapper() -> str:
    return os.getenv("DC_WRAPPER", pe.DC_WRAPPER_DEFAULT)


def compose(*args: str, timeout: int = 180) -> None:
    subprocess.run([wrapper(), *args], check=True, capture_output=True, text=True, timeout=timeout)


def jwt_subject(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return str(json.loads(base64.urlsafe_b64decode(payload))["sub"])


def buyer_id(world: World) -> str:
    user = world.state.current_user
    assert user is not None and user.token, "no logged-in buyer"
    return jwt_subject(user.token)


# ── Redis with exact restore ─────────────────────────────────────────────
class Keys:
    """Writes keys in one Redis DB, remembering what was there so `restore` puts it back."""

    def __init__(self, db: int) -> None:
        self.db = db
        self._saved: dict[str, tuple[str | None, int]] = {}
        self._redis = Online()
        self._redis.call("SELECT", str(db))

    def get(self, key: str) -> str | None:
        return self._redis.get(key)

    def _save(self, key: str) -> None:
        if key not in self._saved:
            self._saved[key] = (self._redis.get(key), int(self._redis.call("PTTL", key)))

    def put(self, key: str, value: str) -> None:
        self._save(key)
        self._redis.call("SET", key, value)

    def remove(self, key: str) -> None:
        self._save(key)
        self._redis.call("DEL", key)

    def restore(self) -> None:
        try:
            for key, (value, pttl) in self._saved.items():
                if value is None:
                    self._redis.call("DEL", key)
                elif pttl > 0:
                    self._redis.call("SET", key, value, "PX", str(pttl))
                else:
                    self._redis.call("SET", key, value)
            self._saved.clear()
        finally:
            self._redis.close()


def keys(world: World, db: int) -> Keys:
    """A Keys writer whose restore is registered as scenario teardown."""
    handle = Keys(db)
    world.add_cleanup(handle.restore)
    return handle


def serving_prefix(redis: Keys) -> str:
    """`recs:v1:gen:<serving>` while a pointer exists, else the unscoped `recs:v1`."""
    pointer = redis.get(SERVING_KEY)
    return f"recs:v1:gen:{pointer}" if pointer else "recs:v1"


# ── Recommend through the gateway ────────────────────────────────────────
def recommend(world: World, limit: int = 10) -> dict[str, Any]:
    return world.service_factory.recommendation.recommend(context=CONTEXT_HOMEPAGE, limit=limit)


def ids(response: dict[str, Any]) -> list[str]:
    items = sorted(response.get("items") or [], key=lambda it: it.get("rank", 0))
    return [it.get("listingId", "") for it in items]


def await_recommend(
    world: World, accept, what: str, timeout_s: float = SETTLE_S, limit: int = 10
) -> dict[str, Any]:
    """Poll Recommend until `accept(response)`; the failure names the last answer."""
    deadline = time.monotonic() + timeout_s
    last: Any = None
    while True:
        try:
            last = recommend(world, limit)
            if accept(last):
                return last
        except GatewayError as exc:
            last = exc
        assert time.monotonic() < deadline, f"{what}: still {last!r} after {timeout_s:.0f}s"
        time.sleep(POLL_S)


def await_serving_again(world: World, timeout_s: float = 90.0) -> None:
    """Teardown helper: team-ai answers with a real (non fallback) model again."""
    try:
        await_recommend(
            world,
            lambda r: bool(r.get("modelVersion")) and r.get("modelVersion") != FALLBACK_VERSION,
            "team-ai did not serve a real model again",
            timeout_s,
        )
    except AssertionError as exc:
        print(f"[rss] {exc}")


def ai_log_lines(needle: str) -> list[str]:
    out = pe.docker("logs", "--tail", "5000", pe.ai_container(), check=False)
    return [line for line in (out.stdout + out.stderr).splitlines() if needle in line]
