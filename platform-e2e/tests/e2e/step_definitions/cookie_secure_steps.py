"""Auth cookie Secure flag (change: frontend-cookie-secure).

Black-box: the Secure attribute is read from the Set-Cookie response headers the storefront
sends to the browser (the stack runs with COOKIE_SECURE unset, so the x-forwarded-proto rule
is what is observed). Generic login / session steps come from session_continuity_steps.
"""

from __future__ import annotations

import time

from playwright.sync_api import Response
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import PageName, timeouts
from tests.e2e.step_definitions.session_continuity_steps import (  # noqa: F401
    _cookies,
    _forged_token,
    _put,
)
from tests.e2e.support.world import World

SETTINGS = get_settings()
AUTH_COOKIES = ("session", "agora_refresh", "agora_auth_evt")


def _ex(world: World) -> dict:
    return world.state.extra


def _capture(world: World) -> list[tuple[str, str]]:
    """Start (once) collecting (name, raw Set-Cookie line) for every response of the page."""
    ex = _ex(world)
    if "set_cookies" not in ex:
        sink: list[tuple[str, str]] = []
        ex["set_cookies"] = sink

        def on_response(resp: Response) -> None:
            try:
                raw = resp.all_headers().get("set-cookie", "")
            except Exception:  # noqa: BLE001 - response gone (navigation); nothing to record
                return
            for line in filter(None, raw.split("\n")):
                sink.append((line.split("=", 1)[0].strip(), line))

        world.page.on("response", on_response)
    return ex["set_cookies"]


def _set_header(world: World, proto: str) -> None:
    world.context.set_extra_http_headers({"x-forwarded-proto": proto})
    _capture(world)


def _is_secure(line: str) -> bool:
    return any(p.strip().lower() == "secure" for p in line.split(";")[1:])


def _lines(world: World, names: tuple[str, ...]) -> dict[str, list[str]]:
    sink = _capture(world)
    return {n: [line for name, line in sink if name == n and "=;" not in line] for n in names}


def _assert_secure(world: World, names: tuple[str, ...], secure: bool) -> None:
    got = _lines(world, names)
    for name in names:
        assert got[name], f"no Set-Cookie with a value for {name}; saw {[n for n, _ in _capture(world)]}"
        flags = [_is_secure(line) for line in got[name]]
        assert all(f is secure for f in flags), f"{name}: expected secure={secure}, got {got[name]}"


@given(parsers.parse('requests carry the x-forwarded-proto header "{proto}"'))
def carry_header(world: World, proto: str) -> None:
    _set_header(world, proto)


@given("the Set-Cookie headers are recorded")
def record_headers(world: World) -> None:
    _capture(world)


@when(
    parsers.parse(
        'the browser is cleared and the buyer signs in again with the x-forwarded-proto header "{proto}"'
    )
)
def relogin_with_header(world: World, proto: str) -> None:
    world.context.clear_cookies()
    _ex(world)["set_cookies"].clear()
    _set_header(world, proto)
    page = world.navigate_to(PageName.LOGIN)
    page.login(_ex(world)["username"], _ex(world)["password"])  # type: ignore[attr-defined]
    world.page.wait_for_url(f"{SETTINGS.base_url}/", timeout=timeouts.NAVIGATION)


@when(
    parsers.parse(
        'the session expires again and requests carry the x-forwarded-proto header "{proto}"'
    )
)
def expire_again_with_header(world: World, proto: str) -> None:
    ex = _ex(world)
    ex["set_cookies"].clear()
    refresh = _cookies(world)["agora_refresh"]["value"]
    ex["refresh"] = refresh
    ex["session_cookie"] = _forged_token(ex["sid"], int(time.time()) - 60)
    world.context.clear_cookies(name="session")
    _put(world, "session", ex["session_cookie"])
    _set_header(world, proto)


@then(
    "every Set-Cookie for session, agora_refresh and agora_auth_evt carries the Secure attribute"
)
def all_secure(world: World) -> None:
    _assert_secure(world, AUTH_COOKIES, True)


@then(
    "every Set-Cookie for session, agora_refresh and agora_auth_evt omits the Secure attribute"
)
def none_secure(world: World) -> None:
    _assert_secure(world, AUTH_COOKIES, False)


@then("the response set session and agora_refresh with the Secure attribute")
def refresh_secure(world: World) -> None:
    _assert_secure(world, ("session", "agora_refresh"), True)


@then("the response set session and agora_refresh without the Secure attribute")
def refresh_not_secure(world: World) -> None:
    _assert_secure(world, ("session", "agora_refresh"), False)
