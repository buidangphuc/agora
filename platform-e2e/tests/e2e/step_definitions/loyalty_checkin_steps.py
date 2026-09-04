"""Loyalty daily check-in steps (home-page "Điểm danh" widget → EngagementService.CheckIn).

Reuses the login Given from `common_steps`. A fresh buyer is seeded per scenario
via the `@needsBuyer` tag, so the streak deterministically advances 0 → 1.
"""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.constants import timeouts
from src.pages.loyalty_checkin_widget import LoyaltyCheckinWidget
from tests.e2e.support.world import World


def _widget(world: World) -> LoyaltyCheckinWidget:
    page = LoyaltyCheckinWidget(world.page)
    world.set_current_page(page)
    return page


@when("the buyer opens the home page")
def buyer_opens_home(world: World) -> None:
    _widget(world).navigate()


@then("the loyalty check-in widget shows the streak and coin balance")
def widget_shows_streak_and_coins(world: World) -> None:
    widget = _widget(world)
    expect(widget.heading).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(widget.widget).to_contain_text("Chuỗi ngày")
    expect(widget.widget).to_contain_text("xu")
    expect(widget.check_in_button).to_be_visible(timeout=timeouts.DEFAULT)
    world.logger.info("Loyalty check-in widget rendered with streak + coin balance")


@when("the buyer checks in for the day")
def buyer_checks_in(world: World) -> None:
    widget = _widget(world)
    expect(widget.check_in_button).to_be_visible(timeout=timeouts.NAVIGATION)
    widget.check_in()
    # The success toast fires only after the CheckIn RPC commits server-side.
    expect(world.page.get_by_text("Điểm danh thành công", exact=False)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then("the streak advances and coins are earned")
def streak_advances(world: World) -> None:
    widget = _widget(world)
    # The "(+N)" badge only renders after a successful check-in.
    expect(widget.earned_marker).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(widget.streak_value).to_have_text("1", timeout=timeouts.DEFAULT)
    expect(widget.widget).to_contain_text("xu")
    world.logger.info("Check-in advanced streak to 1 and earned coins")
