"""Session client context (change: frontend-session-client-context).

Black-box: two real browser contexts with different user agents. The first signs in through
the login form (UA 1); the second carries the same cookies, with the access token expired, so the
storefront middleware refreshes the session while the request comes from UA 2. The Active sessions
page and the gateway's ListSessions are the oracles for the persisted device / IP / last_seen.
"""

from __future__ import annotations

import time
from datetime import datetime

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import SessionService
from src.core import PageFactory
from src.constants import PageName, timeouts
from src.pages.account_security_page import AccountSecurityPage
from src.utils import data as fake
from tests.e2e.flows import session_id_of
from tests.e2e.step_definitions.session_continuity_steps import (
    AUTH_REGISTER,
    SESSION_COOKIE,
    _forged_token,
    _post,
    _result,
)
from tests.e2e.support.world import World

SETTINGS = get_settings()
SECURITY_URL = f"{SETTINGS.base_url}/account/security"


def _ex(world: World) -> dict:
    return world.state.extra


def _new_context(world: World, user_agent: str):  # noqa: ANN202
    ctx = world.context.browser.new_context(
        user_agent=user_agent, ignore_https_errors=True, viewport={"width": 1280, "height": 720}
    )
    world.add_cleanup(ctx.close)
    return ctx


def _sessions(token: str) -> list[dict]:
    svc = SessionService(token=token)
    try:
        return svc.list_sessions()
    finally:
        svc.close()


@given(
    parsers.parse(
        'a registered buyer who signed in through the login form from user agent "{ua}"'
    )
)
def signed_in_from(world: World, ua: str) -> None:
    ex = _ex(world)
    username = fake.unique_username("ctxrefresh")
    _result(
        _post(
            AUTH_REGISTER,
            {"username": username, "password": SETTINGS.seed_password, "role": "buyer"},
        )
    )
    ctx = _new_context(world, ua)
    page = ctx.new_page()
    page.set_default_timeout(SETTINGS.action_timeout_ms)
    page.set_default_navigation_timeout(SETTINGS.navigation_timeout_ms)
    ctx.add_cookies(
        [{"name": "e2e", "value": "1", "url": SETTINGS.base_url},
         {"name": "agora_consent", "value": f"v=1&a=1&m=0&p=0&ts={int(time.time())}",
          "url": SETTINGS.base_url}]
    )
    world.context, world.page = ctx, page
    world.page_factory = PageFactory(page)  # factory pages are bound to the page at hand
    login = world.navigate_to(PageName.LOGIN)
    login.login(username, SETTINGS.seed_password)  # type: ignore[attr-defined]
    page.wait_for_url(f"{SETTINGS.base_url}/", timeout=timeouts.NAVIGATION)
    cookie = next(c for c in ctx.cookies([SETTINGS.base_url]) if c["name"] == SESSION_COOKIE)
    ex.update(
        username=username,
        sid=session_id_of(cookie["value"]),
        access=cookie["value"],
        first_ua=ua,
    )


@given(parsers.parse('the Active sessions page lists the session with device "{device}"'))
def page_lists_device(world: World, device: str) -> None:
    # Positive control: the page does render the device column, so a later absence is meaningful.
    world.page.goto(SECURITY_URL)
    expect(AccountSecurityPage(world.page).sessions_section).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(world.page.get_by_text(device).first).to_be_visible(timeout=timeouts.DEFAULT)
    mine = [s for s in _sessions(_ex(world)["access"]) if s["id"] == _ex(world)["sid"]]
    assert mine and mine[0].get("device") == device, mine
    _ex(world)["created_at"] = mine[0]["createdAt"]


@when(
    parsers.parse(
        'the session cookie expires and the next page load comes from user agent "{ua}"'
    )
)
def expire_and_reload_from(world: World, ua: str) -> None:
    ex = _ex(world)
    time.sleep(2)  # last_seen must be able to move past created_at
    cookies = world.context.cookies([SETTINGS.base_url])
    for c in cookies:
        if c["name"] == SESSION_COOKIE:
            c["value"] = _forged_token(ex["sid"], int(time.time()) - 60)
    assert any(c["name"] == "agora_refresh" for c in cookies), "login set no refresh cookie"
    ctx = _new_context(world, ua)
    ctx.add_cookies(cookies)
    page = ctx.new_page()
    page.set_default_timeout(SETTINGS.action_timeout_ms)
    page.set_default_navigation_timeout(SETTINGS.navigation_timeout_ms)
    world.context, world.page = ctx, page
    page.goto(SECURITY_URL)
    ex["second_ua"] = ua


@then(parsers.parse('the Active sessions page lists that session with device "{device}"'))
def page_lists_new_device(world: World, device: str) -> None:
    expect(AccountSecurityPage(world.page).sessions_section).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(world.page.get_by_text(device).first).to_be_visible(timeout=timeouts.DEFAULT)


@then(parsers.parse('the Active sessions page has no entry with device "{device}"'))
def page_lacks_device(world: World, device: str) -> None:
    sessions_table = world.page.get_by_role("table", name="Phiên đăng nhập", exact=True)
    expect(sessions_table.get_by_text(device)).to_have_count(0)


@then(
    'the stored session has device "AgoraTest/2", a non-empty IP and a last-seen time later '
    "than its created time"
)
def stored_session(world: World) -> None:
    ex = _ex(world)
    cookie = next(
        c for c in world.context.cookies([SETTINGS.base_url]) if c["name"] == SESSION_COOKIE
    )
    mine = [s for s in _sessions(cookie["value"]) if s["id"] == ex["sid"]]
    assert mine, "session missing from ListSessions"
    s = mine[0]
    assert s.get("device") == "AgoraTest/2", s
    assert s.get("ip"), s

    def ts(v: str) -> datetime:
        return datetime.fromisoformat(v.replace("Z", "+00:00"))

    assert ts(s["lastSeen"]) > ts(s["createdAt"]), s
