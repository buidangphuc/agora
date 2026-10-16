"""Session refresh (change: identity-session-refresh), driven through the gateway only.

AuthService.RefreshSession is public: calls carry no Authorization header. Responses are
Connect JSON (`result.token`, `result.refreshToken`, `result.refreshExpiresAt`); failures are
HTTP 401 with Connect code `unauthenticated`.
"""

from __future__ import annotations

import base64
import re
import secrets
import time
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone

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
THIRTY_DAYS_S = 30 * 24 * 3600
TOLERANCE_S = 5
TOKEN_RE = re.compile(r"^[A-Za-z0-9_-]{43}$")


def _post(endpoint: str, body: dict, token: str | None = None) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    return httpx.post(
        f"{SETTINGS.gateway_url.rstrip('/')}{endpoint}", json=body, headers=headers, timeout=15
    )


def _refresh(refresh_token: str) -> httpx.Response:
    return _post(AUTH_REFRESH, {"refreshToken": refresh_token})


def _result(resp: httpx.Response) -> dict:
    assert resp.status_code == 200, f"expected 200, got {resp.status_code}: {resp.text[:300]}"
    return resp.json().get("result") or {}


def _expires_in_s(result: dict) -> float:
    raw = result.get("refreshExpiresAt")
    assert raw, f"refreshExpiresAt missing: {result}"
    when = datetime.fromisoformat(raw.replace("Z", "+00:00"))
    return (when - datetime.now(timezone.utc)).total_seconds()


def _assert_30_days(result: dict) -> None:
    left = _expires_in_s(result)
    assert abs(left - THIRTY_DAYS_S) <= TOLERANCE_S, f"refresh expiry is {left}s away"


def _assert_unauthenticated(resp: httpx.Response) -> None:
    assert resp.status_code == 401, f"expected 401, got {resp.status_code}: {resp.text[:300]}"
    assert resp.json().get("code") == "unauthenticated", resp.text[:300]


def _new_buyer(world: World) -> tuple[str, str]:
    username = fake.unique_username("refresh")
    world.state.extra["username"] = username
    world.state.extra["password"] = SETTINGS.seed_password
    return username, SETTINGS.seed_password


def _login_raw(world: World) -> dict:
    extra = world.state.extra
    resp = _post(ep.AUTH_LOGIN, {"username": extra["username"], "password": extra["password"]})
    return _result(resp)


@given("a registered buyer")
def registered_buyer(world: World) -> None:
    username, password = _new_buyer(world)
    assert AuthService().register(username, password, "buyer")


@given("a buyer who has logged in and holds a refresh token")
def logged_in_with_refresh(world: World) -> None:
    username, password = _new_buyer(world)
    assert AuthService().register(username, password, "buyer")
    result = _login_raw(world)
    refresh = result.get("refreshToken", "")
    assert refresh, f"login returned no refreshToken: {result}"
    world.state.extra.update(
        access_token=result["token"],
        sid=session_id_of(result["token"]),
        refresh_chain=[refresh],
    )


@given("a buyer signed in on a second device and holding a refresh token on the first")
def two_devices(world: World) -> None:
    logged_in_with_refresh(world)
    other = _login_raw(world)
    world.state.extra["second_token"] = other["token"]


@when("the buyer logs in directly at the gateway and keeps the refresh token")
def login_keep(world: World) -> None:
    world.state.extra["last_result"] = _login_raw(world)


@when("a new buyer registers directly at the gateway")
def register_keep(world: World) -> None:
    username, password = _new_buyer(world)
    resp = _post(ep.AUTH_REGISTER, {"username": username, "password": password, "role": "buyer"})
    world.state.extra["last_result"] = _result(resp)


@then("the refresh token is a 43-character base64url string")
def token_shape(world: World) -> None:
    token = world.state.extra["last_result"].get("refreshToken", "")
    assert TOKEN_RE.match(token), f"unexpected refresh token shape: {token!r}"
    assert len(base64.urlsafe_b64decode(token + "=")) == 32


@then("the refresh expiry is 30 days from now")
def expiry_30d(world: World) -> None:
    _assert_30_days(world.state.extra["last_result"])


def _do_refresh(world: World, token: str) -> httpx.Response:
    resp = _refresh(token)
    world.state.extra["last_response"] = resp
    return resp


def _rotate(world: World) -> None:
    extra = world.state.extra
    result = _result(_do_refresh(world, extra["refresh_chain"][-1]))
    assert result.get("refreshToken"), f"rotation returned no refresh token: {result}"
    extra["refresh_chain"].append(result["refreshToken"])
    extra["last_result"] = result


@when("the buyer refreshes with the current refresh token")
@when("the buyer refreshes with the latest refresh token")
def refresh_current(world: World) -> None:
    extra = world.state.extra
    resp = _do_refresh(world, extra["refresh_chain"][-1])
    if resp.status_code == 200:
        result = resp.json().get("result") or {}
        extra["last_result"] = result
        if result.get("refreshToken"):
            extra["refresh_chain"].append(result["refreshToken"])


@when(parsers.parse("the buyer refreshes {n:d} times in sequence"))
def refresh_n(world: World, n: int) -> None:
    for _ in range(n):
        _rotate(world)


@when(parsers.parse("{seconds:d} seconds pass"))
def wait(seconds: int) -> None:
    time.sleep(seconds)


@when("the buyer replays the first refresh token immediately")
@when("the buyer replays the first refresh token")
def replay_first(world: World) -> None:
    resp = _do_refresh(world, world.state.extra["refresh_chain"][0])
    if resp.status_code == 200:
        world.state.extra["last_result"] = resp.json().get("result") or {}


@when("the buyer logs out of the session")
def logout(world: World) -> None:
    extra = world.state.extra
    svc = SessionService(token=extra["access_token"])
    try:
        svc.revoke_session(extra["sid"])
    finally:
        svc.close()


@when("the buyer refreshes with the same refresh token from two tabs at once")
def two_tabs(world: World) -> None:
    token = world.state.extra["refresh_chain"][0]
    with ThreadPoolExecutor(max_workers=2) as pool:
        futures = [pool.submit(_refresh, token) for _ in range(2)]
        responses = [f.result() for f in futures]
    world.state.extra["tab_results"] = [_result(r) for r in responses]


@then("the refresh returns an access token for the original session id")
def access_same_sid(world: World) -> None:
    extra = world.state.extra
    assert session_id_of(extra["last_result"]["token"]) == extra["sid"]


@then("the refresh returns a different refresh token expiring in 30 days")
def new_refresh(world: World) -> None:
    extra = world.state.extra
    result = extra["last_result"]
    assert result["refreshToken"] not in extra["refresh_chain"][:-1]
    assert TOKEN_RE.match(result["refreshToken"])
    _assert_30_days(result)


@then("the refresh needed no Authorization header")
def anonymous_call(world: World) -> None:
    # _refresh() never sets Authorization; a 200 therefore proves RefreshSession is public.
    assert world.state.extra["last_response"].status_code == 200


@then("exactly one tab receives a new refresh token")
def one_new_token(world: World) -> None:
    results = world.state.extra["tab_results"]
    assert sum(1 for r in results if r.get("refreshToken")) == 1, results


@then("the other tab receives an access token with an empty refresh token")
def other_tab_access_only(world: World) -> None:
    extra = world.state.extra
    bare = [r for r in extra["tab_results"] if not r.get("refreshToken")]
    assert len(bare) == 1, extra["tab_results"]
    assert session_id_of(bare[0]["token"]) == extra["sid"]


@then("the session is still listed as active")
def session_active(world: World) -> None:
    extra = world.state.extra
    fresh = next(r["token"] for r in extra["tab_results"] if r.get("refreshToken"))
    svc = SessionService(token=fresh)
    try:
        mine = [s for s in svc.list_sessions() if s["id"] == extra["sid"]]
    finally:
        svc.close()
    assert mine and not mine[0].get("revoked"), mine


@then("the replay returns an access token for the original session id with no refresh token")
def replay_access_only(world: World) -> None:
    extra = world.state.extra
    result = _result(extra["last_response"])
    assert session_id_of(result["token"]) == extra["sid"]
    assert not result.get("refreshToken"), "grace replay must not mint a refresh token"


@then("the second refresh token is still accepted")
@then("the latest refresh token is still accepted")
def latest_accepted(world: World) -> None:
    result = _result(_refresh(world.state.extra["refresh_chain"][-1]))
    assert result.get("token")


@then("the refresh is rejected as unauthenticated")
def rejected(world: World) -> None:
    _assert_unauthenticated(world.state.extra["last_response"])


@then("the latest refresh token is rejected as unauthenticated")
def latest_rejected(world: World) -> None:
    _assert_unauthenticated(_refresh(world.state.extra["refresh_chain"][-1]))


@then("no access token was issued")
def no_access(world: World) -> None:
    body = world.state.extra["last_response"].json()
    assert not (body.get("result") or {}).get("token"), body


@then("the first device's session is not active in the second device's session list")
def session_gone(world: World) -> None:
    extra = world.state.extra
    svc = SessionService(token=extra["second_token"])
    try:
        mine = [s for s in svc.list_sessions() if s["id"] == extra["sid"]]
    finally:
        svc.close()
    assert not mine or all(s.get("revoked") for s in mine), mine


@when("an anonymous caller refreshes with a random 43-character token")
def refresh_random(world: World) -> None:
    _do_refresh(world, secrets.token_urlsafe(32))


@when("an anonymous caller refreshes with an empty token")
def refresh_empty(world: World) -> None:
    _do_refresh(world, "")


@when(parsers.parse("an anonymous caller refreshes with a token of {n:d} bytes"))
def refresh_sized(world: World, n: int) -> None:
    _do_refresh(world, "a" * n)


@then("refresh failures for unknown, revoked, empty and 257-byte tokens carry an identical message")
def identical_messages(world: World) -> None:
    revoked = world.state.extra["refresh_chain"][0]
    responses = [
        _refresh(secrets.token_urlsafe(32)),
        _refresh(revoked),
        _refresh(""),
        _refresh("a" * 257),
    ]
    for r in responses:
        _assert_unauthenticated(r)
    bodies = {(r.json().get("code"), r.json().get("message")) for r in responses}
    assert len(bodies) == 1, bodies
