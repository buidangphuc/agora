"""Session flows: token/session helpers and revocation polling over the gateway API.

Revocation reaches the gateway asynchronously (outbox relay -> `identity.events` ->
the gateway's denylist), so scenarios poll with a deadline instead of sleeping.
"""

from __future__ import annotations

import base64
import json
import time

import httpx

from config.settings import get_settings
from src.api.services import SessionService
from src.constants import gateway_endpoints as ep


def session_id_of(token: str) -> str:
    """The `sid` claim of a JWT (no signature check: the test only reads it)."""
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload)).get("sid", "")


def status_of(token: str) -> int:
    """HTTP status of an authenticated ListSessions call with this token."""
    svc = SessionService(token=token)
    try:
        return svc.list_sessions_raw().status_code
    finally:
        svc.close()


def public_status_of(token: str) -> int:
    """HTTP status of a PUBLIC route (anonymous SearchListings) carrying this token."""
    r = httpx.post(
        f"{get_settings().gateway_url.rstrip('/')}{ep.SEARCH_LISTINGS}",
        json={},
        headers={"Content-Type": "application/json", "Authorization": f"bearer {token}"},
        timeout=10,
    )
    return r.status_code


def wait_until_rejected(token: str, timeout_s: float) -> float:
    """Poll until the gateway answers 401 for `token`; return the seconds it took.

    Raises TimeoutError if the token is still accepted after `timeout_s`.
    """
    started = time.monotonic()
    last = 0
    while time.monotonic() - started <= timeout_s:
        last = status_of(token)
        if last == httpx.codes.UNAUTHORIZED:
            return time.monotonic() - started
        time.sleep(0.25)
    raise TimeoutError(f"token still accepted after {timeout_s}s (last status {last})")


def login_with_headers(username: str, password: str, headers: dict[str, str]) -> str:
    """Log in straight against the gateway with extra request headers; return the JWT.

    The e2e runner is NOT in the gateway's TRUSTED_PROXIES, so this is how an untrusted
    client looks to the edge.
    """
    r = httpx.post(
        f"{get_settings().gateway_url.rstrip('/')}{ep.AUTH_LOGIN}",
        json={"username": username, "password": password},
        headers={"Content-Type": "application/json", **headers},
        timeout=15,
    )
    r.raise_for_status()
    return (r.json().get("result") or {}).get("token", "")
