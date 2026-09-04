"""Registration steps (real UI form)."""

from __future__ import annotations

from pytest_bdd import when

from src.constants import PageName
from src.pages import RegisterPage
from src.utils import data as fake
from tests.e2e.support.world import World


@when("the visitor registers a new buyer account")
def register_new_buyer(world: World) -> None:
    username = fake.unique_username("buyer")
    world.state.extra["registered_username"] = username
    page: RegisterPage = world.get_page(PageName.REGISTER)  # type: ignore[assignment]
    page.register(username, "pass123", "buyer")
