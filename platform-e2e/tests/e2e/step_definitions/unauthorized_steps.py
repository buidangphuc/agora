"""Step definitions for unauthorized access checks."""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import then

from src.constants import timeouts
from tests.e2e.support.world import World


@then("the user is redirected to the login page")
def user_redirected_to_login(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/login.*"), timeout=timeouts.DEFAULT)
