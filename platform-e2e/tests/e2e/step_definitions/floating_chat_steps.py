from __future__ import annotations

import re
from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import timeouts
from tests.e2e.support.world import World


@then("the floating chat bubble is visible on the bottom right")
def chat_bubble_visible(world: World) -> None:
    bubble = world.page.get_by_role("link", name="Mở hộp thư tin nhắn và hỗ trợ")
    expect(bubble).to_be_visible(timeout=timeouts.DEFAULT)


@when("the user clicks the floating chat button")
def user_clicks_chat_bubble(world: World) -> None:
    bubble = world.page.get_by_role("link", name="Mở hộp thư tin nhắn và hỗ trợ")
    bubble.click()
    world.page.wait_for_load_state("networkidle")


@then("the chat messenger page is displayed")
def chat_messenger_displayed(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/chat.*"), timeout=timeouts.DEFAULT)
