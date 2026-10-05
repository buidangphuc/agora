"""Stack fault injection for scenarios that need a REAL backend failure.

Stops a stack container through the docker CLI (the e2e runner is on the same host as
the compose stack) and hands back a restore callable. No test-only hooks in production
code: the failure is the service being genuinely down, which the gateway surfaces as
Unavailable and the frontend must render as an error, never as an empty result.
"""

from __future__ import annotations

import subprocess
import time
from collections.abc import Callable

import httpx

_DOCKER_TIMEOUT_S = 60


def _docker(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["docker", *args],
        check=True,
        capture_output=True,
        text=True,
        timeout=_DOCKER_TIMEOUT_S,
    )


def stop_container(name: str) -> Callable[[], None]:
    """`docker stop <name>`; returns a function that starts it again."""
    _docker("stop", name)

    def restore() -> None:
        _docker("start", name)

    return restore


def wait_for_search(gateway_url: str, timeout_s: float = 60.0) -> None:
    """Block until an anonymous SearchListings through the gateway succeeds again."""
    url = f"{gateway_url.rstrip('/')}/platform.search.v1.SearchService/SearchListings"
    deadline = time.monotonic() + timeout_s
    last = "no response"
    while time.monotonic() < deadline:
        try:
            r = httpx.post(url, json={}, headers={"Content-Type": "application/json"}, timeout=5)
            if r.status_code == httpx.codes.OK:
                return
            last = f"HTTP {r.status_code}"
        except httpx.HTTPError as exc:
            last = str(exc)
        time.sleep(1)
    raise TimeoutError(f"search did not recover through the gateway within {timeout_s}s ({last})")
