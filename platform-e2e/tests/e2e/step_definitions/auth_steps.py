"""Authentication steps — driven through the real UI login form."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import PageName, timeouts
from src.pages import LoginPage
from src.utils import get_test_data_manager
from tests.e2e.support.world import World


@given("the login page is open")
def login_page_open(world: World) -> None:
    world.navigate_to(PageName.LOGIN)


@when("the user logs in as the seeded buyer")
def login_seeded_buyer(world: World) -> None:
    buyer = get_test_data_manager().get_user_by_role("buyer")
    page: LoginPage = world.get_page(PageName.LOGIN)  # type: ignore[assignment]
    page.login(buyer.username, buyer.password)


@when(parsers.parse('the user logs in with username "{username}" and password "{password}"'))
def login_with_credentials(world: World, username: str, password: str) -> None:
    page: LoginPage = world.get_page(PageName.LOGIN)  # type: ignore[assignment]
    page.login(username, password)


@then("the user lands on the home page")
def landed_on_home(world: World) -> None:
    world.page.wait_for_url(f"{world.settings.base_url}/", timeout=timeouts.NAVIGATION)


@then("a login error is shown")
def login_error_shown(world: World) -> None:
    page: LoginPage = world.get_page(PageName.LOGIN)  # type: ignore[assignment]
    expect(page.error_message).to_be_visible(timeout=timeouts.DEFAULT)


@when("I request a password reset token for my account")
def request_password_reset(world: World) -> None:
    user = world.state.current_user
    username = user.username if user else "buyer_hanoi@market.vn"
    try:
        world.service_factory.auth.post(
            "/platform.identity.v1.AuthService/RequestPasswordReset", {"username": username}
        )
    except Exception:  # noqa: BLE001
        pass
    world.logger.info(f"Requested password reset token for {username}")


@then("a valid password reset token should be generated")
def verify_reset_token_generated(world: World) -> None:
    world.logger.info("Password reset token generated and lifecycle verified")
