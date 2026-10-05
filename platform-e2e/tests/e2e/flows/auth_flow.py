"""Auth flows: hybrid API login (fast) + UI login (real path).

`login_via_api` mirrors bds `loginViaApi`: authenticate through the gateway, then
inject the resulting JWT as the frontend `session` cookie so the browser is logged
in without driving the form. The frontend stores exactly this token in `session`,
so the cookie value is the raw JWT.
"""

from __future__ import annotations

from src.api.services import AuthService
from src.constants import PageName
from src.models import User

SESSION_COOKIE = "session"


def login_via_api(world, user: User) -> User:
    """Log in through the gateway API and inject the session cookie."""
    auth: AuthService = world.service_factory.auth
    try:
        token = auth.login(user.username, user.password)
    except Exception:  # noqa: BLE001 - account may not exist yet in a fresh env
        token = auth.register(user.username, user.password, user.role)
    user.token = token
    world.service_factory.set_token(token)
    world.context.add_cookies(
        [{"name": SESSION_COOKIE, "value": token, "url": world.settings.base_url}]
    )
    world.state.current_user = user
    world.logger.info(f"Logged in via API as {user.username} ({user.role})")
    return user


def scenario_buyer(world) -> User:
    """The buyer this scenario owns: the @needsBuyer account, else one registered now.

    Never the shared test-data buyer: scenarios run in parallel, and a shared account
    lets one scenario's cart or orders leak into another's assertions.
    """
    existing = world.state.extra.get("seeded_buyer")
    if existing:
        return existing
    from src.utils import data as fake

    username = fake.unique_username("buyer")
    password = world.settings.seed_password
    token = world.service_factory.auth.register(username, password, "buyer")
    user = User(username=username, password=password, role="buyer", token=token)
    world.state.extra["seeded_buyer"] = user
    world.logger.info(f"Registered scenario buyer {username}")
    return user


def login_via_ui(world, username: str, password: str) -> None:
    """Drive the real login form (used to test the login flow itself)."""
    login_page = world.navigate_to(PageName.LOGIN)
    login_page.login(username, password)
    world.logger.info(f"Submitted UI login for {username}")
