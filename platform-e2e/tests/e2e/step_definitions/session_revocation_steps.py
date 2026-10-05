"""Session revocation + trusted client context (change: session-revocation).

API-driven except the device/IP scenario, which logs in through the real form so the
browser -> frontend -> gateway -> identity forwarding path is what gets asserted.
The e2e runner is not in the gateway's TRUSTED_PROXIES, so direct gateway calls here
look like an untrusted client.
"""

from __future__ import annotations

import re

import httpx
from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService, SessionService
from src.constants import timeouts
from src.pages.account_security_page import AccountSecurityPage
from src.utils import data as fake
from tests.e2e.flows import (
    login_via_ui,
    login_with_headers,
    public_status_of,
    restart_container,
    session_id_of,
    status_of,
    wait_for_gateway,
    wait_until_rejected,
)
from tests.e2e.support.world import World

SETTINGS = get_settings()

REVOKE_DEADLINE_S = 5.0
RESTART_DEADLINE_S = 20.0


def _register(world: World) -> tuple[str, str]:
    username = fake.unique_username("revoke")
    password = SETTINGS.seed_password
    token = AuthService().register(username, password, "buyer")
    assert token, "registration returned no token"
    world.state.extra["username"] = username
    world.state.extra["password"] = password
    return username, password


@given("a buyer who is signed in on two devices")
def buyer_on_two_devices(world: World) -> None:
    username, password = _register(world)
    # Two independent logins = two sessions (two `sid`s), like two devices.
    other = AuthService().login(username, password)
    current = AuthService().login(username, password)
    assert session_id_of(other) and session_id_of(current), "tokens must carry a sid"
    assert session_id_of(other) != session_id_of(current)
    # Both work before any revocation.
    assert status_of(other) == httpx.codes.OK
    assert status_of(current) == httpx.codes.OK
    world.state.extra.update(other_token=other, current_token=current)


@given("a registered buyer")
def registered_buyer(world: World) -> None:
    _register(world)


@when("the buyer revokes the other device's session from the current device")
def revoke_other_device(world: World) -> None:
    extra = world.state.extra
    svc = SessionService(token=extra["current_token"])
    try:
        svc.revoke_session(session_id_of(extra["other_token"]))
    finally:
        svc.close()


@then(parsers.parse("the other device's token is rejected with 401 within {seconds:d} seconds"))
@when(parsers.parse("the other device's token is rejected with 401 within {seconds:d} seconds"))
def other_token_rejected(world: World, seconds: int) -> None:
    elapsed = wait_until_rejected(world.state.extra["other_token"], float(seconds))
    world.logger.info(f"revoked token rejected after {elapsed:.2f}s")
    assert elapsed <= seconds


@then("the other device's token is also rejected on a public route")
def other_token_rejected_public(world: World) -> None:
    assert public_status_of(world.state.extra["other_token"]) == httpx.codes.UNAUTHORIZED


@then("the current device's token still works")
def current_token_works(world: World) -> None:
    assert status_of(world.state.extra["current_token"]) == httpx.codes.OK


@when("the gateway is restarted")
def restart_gateway(world: World) -> None:
    """Restart the real gateway container: its in-memory denylist starts empty and must
    be rebuilt by replaying identity.events. Scenario is `@destructive` (serial only)."""
    world.add_cleanup(lambda: wait_for_gateway(SETTINGS.gateway_url))
    restart_container(SETTINGS.gateway_container)
    wait_for_gateway(SETTINGS.gateway_url)


@when("the buyer logs in through the login form")
def login_through_form(world: World) -> None:
    extra = world.state.extra
    login_via_ui(world, extra["username"], extra["password"])
    world.page.wait_for_url(f"{world.settings.base_url}/", timeout=timeouts.NAVIGATION)
    world.state.extra["browser_ua"] = world.page.evaluate("() => navigator.userAgent")


@when("the buyer opens the account security page directly")
def open_security(world: World) -> None:
    page = AccountSecurityPage(world.page)
    page.navigate()
    expect(page.heading).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the newest session lists this browser's user agent and a non-empty IP")
def session_shows_device_and_ip(world: World) -> None:
    ua = world.state.extra["browser_ua"]
    # The UI shows `device` as the row's primary text and `IP: <ip> · ...` beneath it.
    expect(world.page.get_by_text(ua, exact=False).first).to_be_visible(timeout=timeouts.DEFAULT)
    body = world.page.locator("body").inner_text()
    match = re.search(r"IP:\s*([0-9a-fA-F:.]+)\s*·", body)
    assert match, f"no 'IP: <address> ·' found on the security page: {body[:600]!r}"

    # Cross-check the stored record through the API (what identity actually saved).
    cookie = next(c for c in world.context.cookies() if c["name"] == "session")
    svc = SessionService(token=cookie["value"])
    try:
        sessions = svc.list_sessions()
    finally:
        svc.close()
    assert sessions, "the login must have created a session"
    newest = sessions[0]
    assert newest.get("device", "").startswith(ua[:200]), newest
    assert newest.get("ip"), f"session ip must not be empty: {newest}"


@when(parsers.parse("the buyer logs in directly at the gateway claiming to be {ip}"))
def login_claiming_ip(world: World, ip: str) -> None:
    extra = world.state.extra
    token = login_with_headers(
        extra["username"],
        extra["password"],
        {
            "X-Forwarded-For": ip,
            "X-Real-IP": ip,
            "X-Client-IP": ip,
            "X-Client-User-Agent": "spoofed-agent",
        },
    )
    assert token, "login returned no token"
    extra["login_token"] = token


@then(parsers.parse("the recorded session IP is not {ip} and is not empty"))
def recorded_ip_not_spoofed(world: World, ip: str) -> None:
    token = world.state.extra["login_token"]
    svc = SessionService(token=token)
    try:
        sessions = svc.list_sessions()
    finally:
        svc.close()
    mine = next(s for s in sessions if s["id"] == session_id_of(token))
    assert mine.get("ip"), f"session ip must not be empty: {mine}"
    assert mine["ip"] != ip, f"the spoofed X-Forwarded-For must be ignored: {mine}"
    assert (
        mine.get("device") != "spoofed-agent"
    ), f"x-client-user-agent must not pass through: {mine}"
