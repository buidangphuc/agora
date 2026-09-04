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


def login_via_ui(world, username: str, password: str) -> None:
    """Drive the real login form (used to test the login flow itself)."""
    login_page = world.navigate_to(PageName.LOGIN)
    login_page.login(username, password)
    world.logger.info(f"Submitted UI login for {username}")
