"""Step definitions for voucher hub exploration."""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.constants import PageName, timeouts
from tests.e2e.support.world import World


@when("the buyer navigates to the voucher hub")
def buyer_navigates_to_vouchers(world: World) -> None:
    world.navigate_to(PageName.VOUCHERS)


@then("the vouchers page is displayed")
def vouchers_page_displayed(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/vouchers.*"), timeout=timeouts.DEFAULT)


@then("available promotional vouchers are rendered")
def promotional_vouchers_rendered(world: World) -> None:
    voucher_btn = world.page.get_by_role("button", name="Lưu mã").first
    expect(voucher_btn).to_be_visible(timeout=timeouts.DEFAULT)
