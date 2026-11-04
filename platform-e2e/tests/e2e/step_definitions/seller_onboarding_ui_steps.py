"""Steps for frontend/seller_onboarding_ui.feature (seller-onboarding-ui, initiative p10-backend-gaps).

Black box in the browser on the seller app plus gateway read-back. Accounts are registered by
user_enforcement_support (imported only); the account-creating and admin-status steps are reused from
user_enforcement_steps through the shared `ueui` state bucket.
"""

from __future__ import annotations

import re
from urllib.parse import quote, urlsplit

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from src.core.base_page import wait_streamed
from src.pages.login_page import LoginPage
from tests.e2e.support import seller_onboarding_support as so
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

ACCEPT = "Đồng ý và trở thành Người Bán"
SESSION_COOKIE = "seller_session"
REFRESH_COOKIE = "seller_refresh"


def _x(world: World) -> dict:
    return world.state.extra.setdefault("ueui", {})


def _seller(world: World, path: str = "") -> str:
    return f"{world.settings.seller_base_url.rstrip('/')}{path}"


def _button(world: World):
    return world.page.get_by_role("button", name=ACCEPT)


def _cookie(world: World, suffix: str) -> str:
    for c in world.context.cookies(world.settings.seller_base_url):
        if c["name"].endswith(suffix):
            return c["value"]
    return ""


def _sign_in(world: World, user: dict) -> None:
    world.context.clear_cookies()
    page = LoginPage(world.page)
    page.goto(_seller(world, "/login"))
    page.login(user["username"], user["password"])


def _track_navigation(world: World) -> None:
    visited: list[str] = []
    _x(world)["visited"] = visited
    world.page.on(
        "framenavigated", lambda f: visited.append(f.url) if f == world.page.main_frame else None
    )


@given("a second freshly registered buyer for the users page")
def second_buyer(world: World) -> None:
    _x(world)["buyer"] = ue.register_user("buyer", "so_ui_buyer2")


@given("the buyer is signed in on the seller app")
def signed_in_seller_app(world: World) -> None:
    _sign_in(world, _x(world)["buyer"])
    world.page.wait_for_url(lambda u: "/login" not in u, timeout=timeouts.NAVIGATION)
    refresh = _cookie(world, REFRESH_COOKIE)
    assert refresh, "seller login set no seller_refresh cookie"
    _x(world)["refresh_before"] = refresh
    _track_navigation(world)


@when(parsers.parse('the buyer opens "{path}" on the seller app'))
@when(parsers.parse('the visitor opens "{path}" on the seller app'))
def open_on_seller_app(world: World, path: str) -> None:
    world.page.goto(_seller(world, path), wait_until="domcontentloaded")


@when("the buyer clicks the onboarding button")
def click_onboarding(world: World) -> None:
    button = _button(world)
    expect(button).to_be_visible(timeout=timeouts.NAVIGATION)
    LoginPage(world.page).wait_until_interactive(button)
    _x(world)["click_index"] = len(_x(world).get("visited", []))
    button.click()


@when(parsers.parse('an admin sets that buyer to "{status}" after the page loaded'))
def admin_sets_after_load(world: World, status: str) -> None:
    user = _x(world)["buyer"]
    resp = ue.set_user_status(world, user["id"], status, reason="abuse")
    assert resp.status_code == 200, f"SetUserStatus failed: {resp.status_code} {resp.text[:200]}"
    assert ue.stored_user(world, user["id"]).get("status") == f"USER_STATUS_{status.upper()}"


@when("the buyer signs in on the seller login that opened")
def sign_in_on_opened_login(world: World) -> None:
    user = _x(world)["buyer"]
    wait_streamed(world.page)
    LoginPage(world.page).login(user["username"], user["password"])


@then("the onboarding button is shown")
def button_shown(world: World) -> None:
    expect(_button(world)).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the onboarding button is not shown")
def button_not_shown(world: World) -> None:
    expect(_button(world)).to_have_count(0)


@then(parsers.parse('the browser ends on "{path}" with the new-listing form visible'))
def ends_on_new_listing(world: World, path: str) -> None:
    world.page.wait_for_url(_seller(world, path), timeout=timeouts.NAVIGATION)
    expect(world.page.locator("#title")).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the browser ends on "{path}" with the onboarding button shown'))
def ends_with_button(world: World, path: str) -> None:
    world.page.wait_for_url(_seller(world, path), timeout=timeouts.NAVIGATION)
    expect(_button(world)).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the seller_session cookie carries listing.write")
def cookie_has_scope(world: World) -> None:
    token = _cookie(world, SESSION_COOKIE)
    assert token, "no seller_session cookie"
    assert "listing.write" in so.scopes_of(token), so.scopes_of(token)


@then("the seller_session cookie does not carry listing.write")
def cookie_lacks_scope(world: World) -> None:
    token = _cookie(world, SESSION_COOKIE)
    assert "listing.write" not in (
        so.scopes_of(token) if token else []
    ), "cookie carries listing.write"


@then("the seller_refresh cookie value is unchanged")
def refresh_unchanged(world: World) -> None:
    assert _cookie(world, REFRESH_COOKIE) == _x(world)["refresh_before"]


@then(parsers.parse('the seller app shows no "{text}"'))
def shows_no_text(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_have_count(0)


@then("the gateway lists the buyer with the seller role")
def gateway_seller_role(world: World) -> None:
    roles = so.roles_of(world, _x(world)["buyer"]["id"])
    assert "seller" in roles, roles


@then("the gateway lists the buyer without the seller role")
def gateway_no_seller_role(world: World) -> None:
    roles = so.roles_of(world, _x(world)["buyer"]["id"])
    assert "seller" not in roles, roles


@then("the seller centre renders")
def seller_centre(world: World) -> None:
    expect(world.page.get_by_text("Tổng quan gian hàng").first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then("the browser lands on the seller login")
def lands_on_login(world: World) -> None:
    base = urlsplit(_seller(world)).netloc
    world.page.wait_for_url(
        lambda u: urlsplit(u).netloc == base and urlsplit(u).path == "/login",
        timeout=timeouts.NAVIGATION,
    )


@then(parsers.parse('the browser lands on the seller login with returnUrl "{path}"'))
def lands_on_login_with_return(world: World, path: str) -> None:
    pattern = re.compile(r"/login\?.*returnUrl=" + re.escape(quote(path, safe="")) + r"(&|$)")
    world.page.wait_for_url(
        lambda u: urlsplit(u).netloc == urlsplit(_seller(world)).netloc and bool(pattern.search(u)),
        timeout=timeouts.NAVIGATION,
    )


@then(parsers.parse('the browser never reached "{path}" after the click'))
def never_reached(world: World, path: str) -> None:
    after = _x(world)["visited"][_x(world)["click_index"] :]
    hits = [u for u in after if urlsplit(u).path == path]
    assert not hits, f"navigated to {path} after the click: {hits}"


@then(parsers.parse('the seller page shows "{text}"'))
def seller_page_shows(world: World, text: str) -> None:
    expect(world.page.get_by_text(text).first).to_be_visible(timeout=timeouts.NAVIGATION)
