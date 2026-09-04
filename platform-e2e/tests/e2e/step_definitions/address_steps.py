"""Delivery address steps (add via the account/addresses modal)."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.constants import PageName, timeouts
from src.pages import AddressesPage
from src.utils import data as fake
from tests.e2e.support.world import World


@when("the buyer adds a delivery address")
def buyer_adds_address(world: World) -> None:
    recipient = f"E2E {fake.vietnamese_name()}"
    world.state.extra["address_recipient"] = recipient
    page: AddressesPage = world.get_page(PageName.ADDRESSES)  # type: ignore[assignment]
    page.add_address(recipient, "0912345678")


@then("the new address appears in the address list")
def address_appears(world: World) -> None:
    recipient = world.state.extra["address_recipient"]
    page: AddressesPage = world.get_page(PageName.ADDRESSES)  # type: ignore[assignment]
    expect(page.address_by_recipient(recipient).first).to_be_visible(timeout=timeouts.NAVIGATION)
