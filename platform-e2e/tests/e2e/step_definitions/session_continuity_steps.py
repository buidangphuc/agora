"""Frontend session continuity (change: frontend-session-continuity).

Black-box: the browser holds the `session` / `agora_refresh` cookies, the storefront (:3000)
renews or ends the session, and the gateway (:8080) is the oracle for what is persisted
(is the refresh token still accepted, is the access token's session revoked).
Signed-in is observed through the header: logout button vs login link.
"""

from __future__ import annotations

import base64
import json
import os
import re
import time
from concurrent.futures import ThreadPoolExecutor

import httpx
from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import PageName, timeouts
from src.utils import data as fake
from tests.e2e.flows import session_id_of, status_of, wait_until_rejected
from tests.e2e.flows.stack_flow import _docker
from tests.e2e.support.world import World

SETTINGS = get_settings()
AUTH_LOGIN = "/platform.identity.v1.AuthService/Login"
AUTH_REGISTER = "/platform.identity.v1.AuthService/Register"
AUTH_REFRESH = "/platform.identity.v1.AuthService/RefreshSession"
SESSION_REVOKE = "/platform.identity.v1.SessionService/RevokeSession"
SESSION_COOKIE = "session"
REFRESH_COOKIE = "agora_refresh"
LOGOUT = "Đăng xuất"
LOGIN_LINK = "Đăng Nhập"
TOKEN_RE = re.compile(r"^[A-Za-z0-9_-]{43}$")
IDENTITY_CONTAINER = os.environ.get("IDENTITY_CONTAINER", "team-identity-svc")


# ── gateway helpers ──────────────────────────────────────────────────────────
def _post(endpoint: str, body: dict, token: str | None = None) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    return httpx.post(
        f"{SETTINGS.gateway_url.rstrip('/')}{endpoint}", json=body, headers=headers, timeout=15
    )


def _result(resp: httpx.Response) -> dict:
    assert resp.status_code == 200, f"expected 200, got {resp.status_code}: {resp.text[:300]}"
    return resp.json().get("result") or {}


def _refresh(token: str) -> httpx.Response:
    return _post(AUTH_REFRESH, {"refreshToken": token})


def _login(world: World) -> dict:
    ex = world.state.extra
    return _result(_post(AUTH_LOGIN, {"username": ex["username"], "password": ex["password"]}))


def _forged_token(sid: str, exp: int) -> str:
    """An access token the middleware can decode (no signature check) but the gateway rejects."""

    def enc(obj: dict) -> str:
        return base64.urlsafe_b64encode(json.dumps(obj).encode()).rstrip(b"=").decode()

    return f"{enc({'alg': 'RS256', 'typ': 'JWT'})}.{enc({'sub': 'forged', 'sid': sid, 'exp': exp})}.c2ln"


# ── browser cookie helpers ───────────────────────────────────────────────────
def _cookies(world: World) -> dict[str, dict]:
    return {c["name"]: c for c in world.context.cookies([SETTINGS.base_url])}


def _put(world: World, name: str, value: str) -> None:
    world.context.add_cookies(
        [{"name": name, "value": value, "url": SETTINGS.base_url, "httpOnly": True}]
    )


def _ex(world: World) -> dict:
    return world.state.extra


# ── givens: accounts ─────────────────────────────────────────────────────────
@given("a registered buyer")
def registered_buyer(world: World) -> None:
    username = fake.unique_username("continuity")
    _ex(world).update(username=username, password=SETTINGS.seed_password)
    _result(
        _post(
            AUTH_REGISTER,
            {"username": username, "password": SETTINGS.seed_password, "role": "buyer"},
        )
    )


@given("a buyer with a live gateway session")
def live_session(world: World) -> None:
    registered_buyer(world)
    result = _login(world)
    refresh = result.get("refreshToken", "")
    assert refresh, f"login returned no refreshToken: {result}"
    _ex(world).update(
        access=result["token"],
        sid=session_id_of(result["token"]),
        refresh=refresh,
        chain=[refresh],
    )


@given("the buyer's session has been revoked")
def session_revoked(world: World) -> None:
    ex = _ex(world)
    _result(_post(SESSION_REVOKE, {"sessionId": ex["sid"]}, ex["access"]))


@given("the session already rotated its refresh token once")
def rotated_once(world: World) -> None:
    ex = _ex(world)
    result = _result(_refresh(ex["chain"][-1]))
    assert result.get("refreshToken"), result
    ex["chain"].append(result["refreshToken"])
    ex["rotated"] = True
    ex["access"] = result["token"]


# ── givens: browser state ────────────────────────────────────────────────────
@given("the browser holds a session cookie that expired a minute ago and the refresh cookie")
def expired_and_refresh(world: World) -> None:
    ex = _ex(world)
    ex["session_cookie"] = _forged_token(ex["sid"], int(time.time()) - 60)
    _put(world, SESSION_COOKIE, ex["session_cookie"])
    _put(world, REFRESH_COOKIE, ex["refresh"])


@given("the browser holds a session cookie expiring exactly now and the refresh cookie")
def expiring_now_and_refresh(world: World) -> None:
    ex = _ex(world)
    ex["session_cookie"] = _forged_token(ex["sid"], int(time.time()))
    _put(world, SESSION_COOKIE, ex["session_cookie"])
    _put(world, REFRESH_COOKIE, ex["refresh"])


@given("the browser holds a session cookie that expired a minute ago and no refresh cookie")
def expired_only(world: World) -> None:
    ex = _ex(world)
    ex["session_cookie"] = _forged_token(ex["sid"], int(time.time()) - 60)
    _put(world, SESSION_COOKIE, ex["session_cookie"])


@given("the browser holds only the refresh cookie")
@given("the browser holds the first refresh token and no session cookie")
def only_refresh(world: World) -> None:
    ex = _ex(world)
    ex.pop("session_cookie", None)
    _put(world, REFRESH_COOKIE, ex["chain"][0] if "rotated" in ex else ex["refresh"])


@given("the browser holds the real session cookie and the refresh cookie")
def real_and_refresh(world: World) -> None:
    ex = _ex(world)
    ex["session_cookie"] = ex["access"]
    _put(world, SESSION_COOKIE, ex["access"])
    _put(world, REFRESH_COOKIE, ex["refresh"])


@given("the browser holds a garbage refresh cookie and no session cookie")
def garbage_refresh(world: World) -> None:
    _ex(world)["refresh"] = "not-a-real-refresh-token"
    _put(world, REFRESH_COOKIE, _ex(world)["refresh"])


@given("the session cookie is removed from the browser")
def remove_session_cookie(world: World) -> None:
    world.context.clear_cookies(name=SESSION_COOKIE)
    _ex(world).pop("session_cookie", None)


# ── identity outage ──────────────────────────────────────────────────────────
def _wait_identity_ready(timeout_s: float = 90.0) -> None:
    deadline = time.monotonic() + timeout_s
    while time.monotonic() < deadline:
        try:
            if _refresh("probe-" + "x" * 37).status_code == httpx.codes.UNAUTHORIZED:
                return
        except httpx.HTTPError:
            pass
        time.sleep(1)
    raise TimeoutError("identity did not recover")


@given("team-identity is stopped")
def identity_stopped(world: World) -> None:
    _docker("stop", IDENTITY_CONTAINER)

    def restore() -> None:
        _docker("start", IDENTITY_CONTAINER)
        _wait_identity_ready()

    _ex(world)["restore_identity"] = restore
    world.add_cleanup(restore)


@when("team-identity is started again")
def identity_started(world: World) -> None:
    _ex(world)["restore_identity"]()


# ── whens ────────────────────────────────────────────────────────────────────
@when("the buyer signs in through the login form")
def sign_in(world: World) -> None:
    page = world.navigate_to(PageName.LOGIN)
    page.login(_ex(world)["username"], _ex(world)["password"])  # type: ignore[attr-defined]
    world.page.wait_for_url(f"{SETTINGS.base_url}/", timeout=timeouts.NAVIGATION)


@when("the buyer signs in through the login form with a wrong password")
def sign_in_wrong(world: World) -> None:
    page = world.navigate_to(PageName.LOGIN)
    page.login(_ex(world)["username"], "definitely-wrong-password")  # type: ignore[attr-defined]


@when("a new buyer signs up through the register form")
def sign_up(world: World) -> None:
    page = world.navigate_to(PageName.REGISTER)
    page.register(fake.unique_username("continuity"), SETTINGS.seed_password, "buyer")  # type: ignore[attr-defined]
    world.page.wait_for_url(f"{SETTINGS.base_url}/", timeout=timeouts.NAVIGATION)


@given("the visitor loads the home page")
@when("the visitor loads the home page")
def load_home(world: World) -> None:
    ex = _ex(world)
    ex["before"] = {k: v["value"] for k, v in _cookies(world).items()}
    world.page.goto(f"{SETTINGS.base_url}/")
    world.page.wait_for_load_state("domcontentloaded")


@when("the visitor loads the home page counting navigations")
def load_home_counting(world: World) -> None:
    navs: list[str] = []
    world.page.on(
        "request",
        lambda req: navs.append(req.url) if req.resource_type == "document" else None,
    )
    _ex(world)["navs"] = navs
    load_home(world)


@then("the document was requested exactly once")
def navigated_once(world: World) -> None:
    world.page.wait_for_load_state("networkidle")
    assert len(_ex(world)["navs"]) == 1, _ex(world)["navs"]


@given("the shopper clicks logout")
@when("the shopper clicks logout")
def click_logout(world: World) -> None:
    ex = _ex(world)
    ex["cookies_before_logout"] = [
        {"name": n, "value": c["value"]} for n, c in _cookies(world).items()
    ]
    btn = world.page.get_by_role("button", name=LOGOUT)
    expect(btn).to_be_visible(timeout=timeouts.DEFAULT)
    btn.click()
    expect(world.page.get_by_role("link", name=LOGIN_LINK)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@when("the old refresh cookie is put back in the browser without a session cookie")
def put_back(world: World) -> None:
    for c in _ex(world)["cookies_before_logout"]:
        if c["name"] == REFRESH_COOKIE:
            _put(world, c["name"], c["value"])
    world.context.clear_cookies(name=SESSION_COOKIE)


@when(
    parsers.parse(
        "{n:d} requests for the home page carry the same refresh cookie and no session cookie at once"
    )
)
def parallel_requests(world: World, n: int) -> None:
    ex = _ex(world)

    def hit() -> httpx.Response:
        return httpx.get(
            f"{SETTINGS.base_url}/",
            headers={"Cookie": f"{REFRESH_COOKIE}={ex['refresh']}"},
            follow_redirects=True,
            timeout=30,
        )

    with ThreadPoolExecutor(max_workers=n) as pool:
        ex["responses"] = [f.result() for f in [pool.submit(hit) for _ in range(n)]]


# ── thens: cookies ───────────────────────────────────────────────────────────
def _assert_cookie(world: World, name: str, max_age: int) -> dict:
    cookie = _cookies(world).get(name)
    assert cookie, f"{name} cookie missing; have {list(_cookies(world))}"
    left = cookie["expires"] - time.time()
    assert abs(left - max_age) <= 60, f"{name} lasts {left}s, expected {max_age}s"
    return cookie


@then(parsers.parse("the browser holds a session cookie lasting {seconds:d} seconds"))
def session_cookie_lasts(world: World, seconds: int) -> None:
    _assert_cookie(world, SESSION_COOKIE, seconds)


@then(
    parsers.parse(
        'the browser holds an agora_refresh cookie that is httpOnly, lax, path "/" and lasts {seconds:d} seconds'
    )
)
def refresh_cookie_attrs(world: World, seconds: int) -> None:
    cookie = _assert_cookie(world, REFRESH_COOKIE, seconds)
    assert cookie["httpOnly"] is True, cookie
    assert cookie["sameSite"] == "Lax", cookie
    assert cookie["path"] == "/", cookie


@then("the agora_refresh cookie is secure exactly when the session cookie is secure")
def secure_matches(world: World) -> None:
    cookies = _cookies(world)
    assert cookies[REFRESH_COOKIE]["secure"] == cookies[SESSION_COOKIE]["secure"], cookies


@then("the agora_refresh cookie keeps the attributes of a login")
def refresh_attrs_after_rotation(world: World) -> None:
    refresh_cookie_attrs(world, 2592000)
    secure_matches(world)
    assert _cookies(world)[SESSION_COOKIE]["httpOnly"] is True
    _assert_cookie(world, SESSION_COOKIE, 3600)


@then("the browser holds no session cookie")
def no_session_cookie(world: World) -> None:
    assert SESSION_COOKIE not in _cookies(world), _cookies(world)[SESSION_COOKIE]


@then("the browser holds no agora_refresh cookie")
def no_refresh_cookie(world: World) -> None:
    assert REFRESH_COOKIE not in _cookies(world)


@then("the session cookie is unchanged")
def session_unchanged(world: World) -> None:
    assert _cookies(world)[SESSION_COOKIE]["value"] == _ex(world)["before"][SESSION_COOKIE]


@then("the agora_refresh cookie is unchanged")
def refresh_unchanged(world: World) -> None:
    assert _cookies(world)[REFRESH_COOKIE]["value"] == _ex(world)["before"][REFRESH_COOKIE]


@then("the browser session cookie is a usable token of the same session")
def session_usable(world: World) -> None:
    ex = _ex(world)
    value = _cookies(world).get(SESSION_COOKIE, {}).get("value", "")
    assert value, "no session cookie was set"
    assert value != ex.get("session_cookie"), "the stale session cookie was not replaced"
    assert session_id_of(value) == ex["sid"]
    assert status_of(value) == httpx.codes.OK


@then("the agora_refresh cookie was rotated and still works")
def refresh_rotated(world: World) -> None:
    ex = _ex(world)
    value = _cookies(world).get(REFRESH_COOKIE, {}).get("value", "")
    assert TOKEN_RE.match(value), f"no rotated refresh cookie: {value!r}"
    assert value != ex["refresh"], "agora_refresh was not rotated"
    assert _result(_refresh(value)).get("token")


# ── thens: render ────────────────────────────────────────────────────────────
@then("the page renders signed in without a second navigation")
def signed_in(world: World) -> None:
    expect(world.page.get_by_role("button", name=LOGOUT)).to_be_visible(timeout=timeouts.DEFAULT)


@then("the page renders anonymous")
def anonymous(world: World) -> None:
    expect(world.page.get_by_role("link", name=LOGIN_LINK)).to_be_visible(timeout=timeouts.DEFAULT)
    expect(world.page.get_by_role("button", name=LOGOUT)).to_have_count(0)


@then("the shopper lands on the home page anonymous")
def landed_anonymous(world: World) -> None:
    assert world.page.url.rstrip("/") == SETTINGS.base_url.rstrip("/"), world.page.url
    anonymous(world)


@then("the browser is still anonymous")
def still_anonymous(world: World) -> None:
    world.page.goto(f"{SETTINGS.base_url}/")
    anonymous(world)


@then("a login error is displayed")
def login_error(world: World) -> None:
    page = world.get_page(PageName.LOGIN)
    expect(page.error_message).to_be_visible(timeout=timeouts.DEFAULT)  # type: ignore[attr-defined]


@then("every response renders signed in")
def every_signed_in(world: World) -> None:
    for r in _ex(world)["responses"]:
        assert r.status_code == 200, r.status_code
        assert LOGOUT in r.text, "a parallel request rendered anonymous"
        assert LOGIN_LINK not in r.text


# ── thens: persisted state at the gateway ────────────────────────────────────
@then("the buyer's session is still active")
def session_active(world: World) -> None:
    ex = _ex(world)
    time.sleep(2)  # revocation reaches the gateway asynchronously; give it a window to show
    assert status_of(ex["access"]) == httpx.codes.OK, "session was revoked"


@then("a refresh token handed back to the browser still works")
def handed_back_works(world: World) -> None:
    ex = _ex(world)
    handed = []
    for r in ex["responses"]:
        for line in r.headers.get_list("set-cookie"):
            m = re.match(rf"{REFRESH_COOKIE}=([^;]+)", line)
            if m:
                handed.append(m.group(1))
    for token in handed or [ex["refresh"]]:
        assert _result(_refresh(token)).get("token")


@then("the gateway rejects the old access token")
def access_rejected(world: World) -> None:
    wait_until_rejected(_ex(world)["access"], 20)


@then("the gateway rejects the refresh token")
def refresh_rejected(world: World) -> None:
    resp = _refresh(_ex(world)["refresh"])
    assert resp.status_code == 401, resp.text[:300]
    assert resp.json().get("code") == "unauthenticated"


@then("the original refresh token still refreshes")
def original_still_works(world: World) -> None:
    assert _result(_refresh(_ex(world)["refresh"])).get("token")
