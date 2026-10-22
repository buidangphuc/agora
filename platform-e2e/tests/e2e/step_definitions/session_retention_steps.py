"""Session / login-history retention (change: identity-session-retention).

Black-box: gateway API for behaviour, a read-only SELECT on identity_db to prove the row
is really gone (not just hidden by the API). The 12 s wait is a real retention timer
(sweeper runs every 2 s with 5 s retention on the override stack), polled with a deadline.
"""

from __future__ import annotations

import subprocess
import time

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService, SessionService
from src.constants import gateway_endpoints as ep
from src.utils import data as fake
from tests.e2e.flows import session_id_of
from tests.e2e.support.world import World

SETTINGS = get_settings()
AUTH_REFRESH = "/platform.identity.v1.AuthService/RefreshSession"
SHORT_RETENTION = "SESSION_RETENTION=5s"


def _post(endpoint: str, body: dict, token: str | None = None) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    return httpx.post(
        f"{SETTINGS.gateway_url.rstrip('/')}{endpoint}", json=body, headers=headers, timeout=15
    )


def _sessions(token: str) -> list[dict]:
    svc = SessionService(token=token)
    try:
        return svc.list_sessions()
    finally:
        svc.close()


def _identity_psql(sql: str) -> str:
    import os

    res = subprocess.run(
        [
            "docker", "exec", os.getenv("POSTGRES_CONTAINER", "agora-postgres-1"),
            "psql", "-U", "postgres", "-d", "identity_db", "-v", "ON_ERROR_STOP=1", "-At", "-c", sql,
        ],
        capture_output=True,
        text=True,
        timeout=60,
    )  # fmt: skip
    assert res.returncode == 0, f"psql failed: {res.stderr.strip()[:400]}"
    return res.stdout.strip()


def _lit(value: str) -> str:
    assert value.replace("-", "").replace("_", "").isalnum(), f"unexpected id {value!r}"
    return "'" + value + "'"


@given("the stack runs without the short-retention override")
def stack_is_default(world: World) -> None:
    import os

    res = subprocess.run(
        ["docker", "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}",
         os.getenv("IDENTITY_CONTAINER", "team-identity-svc")],
        capture_output=True, text=True, timeout=30,
    )  # fmt: skip
    assert res.returncode == 0, res.stderr.strip()[:300]
    assert SHORT_RETENTION not in res.stdout.split(), "stack is running the short-retention override"


@given("a buyer who is signed in on two devices with a refresh token on the active one")
def two_devices(world: World) -> None:
    username = fake.unique_username("retain")
    password = SETTINGS.seed_password
    assert AuthService().register(username, password, "buyer")
    r1 = _post(ep.AUTH_LOGIN, {"username": username, "password": password})
    r2 = _post(ep.AUTH_LOGIN, {"username": username, "password": password})
    assert r1.status_code == 200 and r2.status_code == 200, (r1.text[:200], r2.text[:200])
    other, active = r1.json()["result"], r2.json()["result"]
    assert active.get("refreshToken"), "login returned no refresh token"
    world.state.extra.update(
        other_token=other["token"],
        active_token=active["token"],
        active_refresh=active["refreshToken"],
        other_sid=session_id_of(other["token"]),
        active_sid=session_id_of(active["token"]),
    )
    assert world.state.extra["other_sid"] != world.state.extra["active_sid"]


@given("ListSessions shows both sessions")
def lists_both(world: World) -> None:
    x = world.state.extra
    ids = {s["id"] for s in _sessions(x["active_token"])}
    assert {x["other_sid"], x["active_sid"]} <= ids, ids


@when("the buyer revokes the other device's session from the active device")
def revoke_other(world: World) -> None:
    x = world.state.extra
    svc = SessionService(token=x["active_token"])
    try:
        svc.revoke_session(x["other_sid"])
    finally:
        svc.close()


@when(parsers.parse("{seconds:d} seconds pass"))
def wait_retention(world: World, seconds: int) -> None:
    # Real retention timer: the purge is time-driven, there is no event to await.
    time.sleep(seconds)


@then("ListSessions returns only the active session")
def only_active(world: World) -> None:
    x = world.state.extra
    ids = {s["id"] for s in _sessions(x["active_token"])}
    assert x["active_sid"] in ids, ids
    assert x["other_sid"] not in ids, f"revoked session still listed: {ids}"


@then("the revoked session row is gone from identity_db")
def row_gone(world: World) -> None:
    x = world.state.extra
    n = _identity_psql(f"select count(*) from sessions where id = {_lit(x['other_sid'])}")
    assert n == "0", f"revoked session row still in identity_db (count {n})"
    n = _identity_psql(f"select count(*) from sessions where id = {_lit(x['active_sid'])}")
    assert n == "1", "active session row must remain"


@then("the active session still accepts RefreshSession")
def active_refreshes(world: World) -> None:
    resp = _post(AUTH_REFRESH, {"refreshToken": world.state.extra["active_refresh"]})
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"


@then("ListSessions still returns the revoked session")
def revoked_still_listed(world: World) -> None:
    x = world.state.extra
    ids = {s["id"] for s in _sessions(x["active_token"])}
    assert x["other_sid"] in ids, f"revoked session vanished on the default stack: {ids}"


@then("the revoked session row in identity_db carries a revoked_at timestamp")
def revoked_at_set(world: World) -> None:
    x = world.state.extra
    out = _identity_psql(
        f"select revoked from sessions where id = {_lit(x['other_sid'])} and revoked_at is not null"
    )
    assert out == "t", f"revoked session has no revoked_at (got {out!r})"


def _history(token: str) -> list[dict]:
    resp = _post(ep.SESSION_LOGIN_HISTORY, {}, token)
    assert resp.status_code == 200, resp.text[:300]
    return resp.json().get("events") or []


def _key(e: dict) -> tuple:
    return (e.get("id") or "", e.get("createdAt"), e.get("ip"), e.get("userAgent"))


@given("a buyer who logs in once and sees a login history entry")
def login_once(world: World) -> None:
    username = fake.unique_username("retainlh")
    password = SETTINGS.seed_password
    assert AuthService().register(username, password, "buyer")
    r = _post(ep.AUTH_LOGIN, {"username": username, "password": password})
    assert r.status_code == 200, r.text[:200]
    token = r.json()["result"]["token"]
    events = _history(token)
    assert events, "login produced no login-history entry"
    world.state.extra.update(lh_token=token, lh_before={_key(e) for e in events})


@then("ListLoginHistory no longer contains that login entry")
def history_purged(world: World) -> None:
    x = world.state.extra
    after = {_key(e) for e in _history(x["lh_token"])}
    assert not (after & x["lh_before"]), f"login entries survived the purge: {after & x['lh_before']}"
