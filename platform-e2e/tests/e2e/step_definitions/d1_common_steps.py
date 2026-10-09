"""Steps shared by the d1 UI step modules."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then

from src.constants import timeouts
from tests.e2e.support.world import World


def _slow_actions(world: World, delay_ms: int = 900) -> None:
    def delay(route) -> None:  # noqa: ANN001
        if route.request.method == "POST" and route.request.headers.get("next-action"):
            world.page.wait_for_timeout(delay_ms)
        route.continue_()

    world.page.route("**/*", delay)
    world.add_cleanup(lambda: world.page.unroute_all(behavior="ignoreErrors"))


@given("the form submission is slowed down")
def form_submission_slowed(world: World) -> None:
    _slow_actions(world)


@then(parsers.parse("the cart counter shows {count:d}"))
def cart_counter_shows(world: World, count: int) -> None:
    cart = world.page.locator('header a[href="/cart"]').first
    expect(cart).to_contain_text(str(count), timeout=timeouts.DEFAULT)
