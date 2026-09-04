"""Account security (F: account_security) — `/account/security` (nav "Bảo Mật")
shows the logged-in buyer's sessions + login history, and redirects anonymous
visitors to `/login`. Mirrors the search_facets step style: API login (fast) +
real UI assertions selected by visible Vietnamese text/role.
"""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import given, scenarios, then, when

from config.settings import get_settings
from src.constants import timeouts
from src.models import User
from src.pages.account_security_page import AccountSecurityPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.support.world import World

SETTINGS = get_settings()

scenarios("../features/frontend/account_security.feature")


def _page(world: World) -> AccountSecurityPage:
    return AccountSecurityPage(world.page)


@given("a logged-in buyer")
def logged_in_buyer(world: World) -> None:
    buyer = User(
        username=fake.unique_username("sec_buyer"),
        password=SETTINGS.seed_password,
        role="buyer",
    )
    login_via_api(world, buyer)


@when('the buyer opens Account Security from the "Bảo Mật" nav link')
def open_security_via_nav(world: World) -> None:
    # Load the shell so the authenticated header (with the "Bảo Mật" link) renders.
    world.page.goto(f"{SETTINGS.base_url.rstrip('/')}/", wait_until="domcontentloaded")
    page = _page(world)
    expect(page.nav_link).to_be_visible(timeout=timeouts.NAVIGATION)
    page.nav_link.click()
    world.page.wait_for_url("**/account/security**", timeout=timeouts.NAVIGATION)


@then("the security page shows the sessions and login history sections")
def security_sections_render(world: World) -> None:
    page = _page(world)
    # Section headers must render without error; the sessions list itself may be
    # empty in a fresh environment, so we assert the section scaffolding only.
    expect(page.heading).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.sessions_section).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.login_history_section).to_be_visible(timeout=timeouts.DEFAULT)
    world.logger.info("Account security sections rendered (sessions + login history)")


@when("an anonymous visitor opens the account security page")
def anon_opens_security(world: World) -> None:
    _page(world).navigate()


@then("they are redirected to the login page")
def redirected_to_login(world: World) -> None:
    world.page.wait_for_url("**/login**", timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("button", name="Đăng nhập")).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    world.logger.info("Anonymous visitor was redirected to /login")
