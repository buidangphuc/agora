"""Generic, reusable steps (navigate / page-displayed by name / API login).

Reuse-first: check here before adding a feature-specific step. Domain steps live
in auth_steps / buyer_steps / seller_steps / etc.
"""

from __future__ import annotations

from pytest_bdd import given, parsers, then, when

from src.constants import PageName
from src.utils import get_test_data_manager
from tests.e2e.flows import login_via_api
from tests.e2e.support.world import World


@given(parsers.parse('the "{page_name}" page is open'))
def open_named_page(world: World, page_name: str) -> None:
    world.navigate_to(PageName(page_name))


@when(parsers.parse('I navigate to the "{page_name}" page'))
def navigate_named_page(world: World, page_name: str) -> None:
    world.navigate_to(PageName(page_name))


@then(parsers.parse('the "{page_name}" page is displayed'))
def named_page_displayed(world: World, page_name: str) -> None:
    page = world.get_page(PageName(page_name))
    assert page.is_displayed(), f"'{page_name}' page not displayed (url={world.page.url})"


@given("I am logged in as a buyer via API")
def logged_in_as_buyer_api(world: World) -> None:
    buyer = world.state.extra.get("seeded_buyer") or get_test_data_manager().get_user_by_role(
        "buyer"
    )
    login_via_api(world, buyer)
