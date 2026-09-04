"""Step definitions for empty and error UI states."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import then

from src.constants import timeouts
from tests.e2e.support.world import World


@then("the search results show an empty state notice")
def search_results_show_empty_state(world: World) -> None:
    empty_notice = world.page.get_by_text("Không tìm thấy", exact=False)
    expect(empty_notice).to_be_visible(timeout=timeouts.DEFAULT)


@then("suggested search tips are displayed")
def suggested_search_tips_displayed(world: World) -> None:
    tips = world.page.get_by_text("Hãy thử tìm kiếm với từ khóa khác", exact=False)
    expect(tips).to_be_visible(timeout=timeouts.DEFAULT)


@then("the empty cart state is displayed")
def empty_cart_state_displayed(world: World) -> None:
    empty_msg = world.page.get_by_text("Giỏ hàng của bạn đang trống", exact=False)
    expect(empty_msg).to_be_visible(timeout=timeouts.DEFAULT)
