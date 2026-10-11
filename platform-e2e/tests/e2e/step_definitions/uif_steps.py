"""Shared steps for the ui-* failure-path scenarios (area uif): stop / pause / restore a real
stack container (see support/uif_support.py)."""

from __future__ import annotations

from pytest_bdd import parsers, then, when

from tests.e2e.support import uif_support as uif
from tests.e2e.support.world import World


@when(parsers.parse("{service} is stopped to force a real failure"))
def service_stopped(world: World, service: str) -> None:
    uif.stop(world, service)


@when(parsers.parse("{service} is paused so that its callers hang"))
def service_paused(world: World, service: str) -> None:
    uif.pause(world, service)


@when(parsers.parse("{service} is running again and answers through the gateway"))
@then(parsers.parse("{service} is running again and answers through the gateway"))
def service_restored(world: World, service: str) -> None:
    uif.restore_now(world, service)


@when("the buyer presses the retry button of the error")
def press_retry(world: World) -> None:
    world.page.get_by_role("button", name="Thử lại").click()
