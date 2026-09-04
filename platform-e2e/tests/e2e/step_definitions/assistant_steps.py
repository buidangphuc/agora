"""AI shopping-assistant steps (drives the /assistant chat, needs team-ai up)."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.constants import PageName, timeouts
from src.pages import AssistantPage
from tests.e2e.support.world import World


@when("the buyer asks the assistant a question")
def ask_assistant(world: World) -> None:
    page: AssistantPage = world.get_page(PageName.ASSISTANT)  # type: ignore[assignment]
    page.chat_input.fill("Tìm laptop dưới 20 triệu")
    page.send_button.click()


@then("the assistant replies")
def assistant_replies(world: World) -> None:
    page: AssistantPage = world.get_page(PageName.ASSISTANT)  # type: ignore[assignment]
    expect(page.ai_replies.first).to_be_visible(timeout=timeouts.LONG)
