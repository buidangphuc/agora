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
    # Controlled input: wait for hydration so the typed text reaches React state
    # and "Gửi" is enabled before it is clicked.
    page.wait_until_interactive(page.chat_input)
    page.chat_input.fill("Tìm laptop dưới 20 triệu")
    expect(page.send_button).to_be_enabled(timeout=timeouts.DEFAULT)
    page.send_button.click()


@then("the assistant replies")
def assistant_replies(world: World) -> None:
    page: AssistantPage = world.get_page(PageName.ASSISTANT)  # type: ignore[assignment]
    expect(page.ai_replies.first).to_be_visible(timeout=timeouts.LONG)
