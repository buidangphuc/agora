from __future__ import annotations

import re
from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import timeouts
from tests.e2e.support.world import World


@when("the user opens the home page")
def user_opens_home_page(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/")
    world.page.wait_for_load_state("networkidle")


@when("the user clicks the logout button")
def user_clicks_logout(world: World) -> None:
    logout_btn = world.page.get_by_role("button", name="Đăng xuất")
    expect(logout_btn).to_be_visible(timeout=timeouts.DEFAULT)
    logout_btn.click()
    world.page.wait_for_load_state("networkidle")


@then("the header displays the login and register links")
def header_shows_auth_links(world: World) -> None:
    expect(world.page.get_by_role("link", name="Đăng Nhập")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(world.page.get_by_role("link", name="Đăng Ký")).to_be_visible(timeout=timeouts.DEFAULT)
