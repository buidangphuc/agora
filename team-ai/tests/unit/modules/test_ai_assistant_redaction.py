"""PII is masked in AIAssistantService log lines (raw-text logs)."""

from __future__ import annotations

from collections.abc import Iterator
from contextlib import contextmanager

from loguru import logger

from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    MagicListingRequest,
    ShoppingAssistantRequest,
)
from app.modules.business.ai_assistant.service import AIAssistantService

PHONE = "0912345678"
EMAIL = "buyer@example.com"
CCCD = "001203004567"


@contextmanager
def _captured_logs() -> Iterator[list[str]]:
    lines: list[str] = []
    handler_id = logger.add(lambda message: lines.append(str(message)), level="DEBUG")
    try:
        yield lines
    finally:
        logger.remove(handler_id)


def _assert_no_raw_pii(lines: list[str]) -> None:
    blob = "\n".join(lines)
    assert blob, "expected the service to log"
    for raw in (PHONE, EMAIL, CCCD):
        assert raw not in blob


async def test_shopping_assistant_log_masks_phone_email_and_id():
    service = AIAssistantService()

    with _captured_logs() as lines:
        await service.shopping_assistant(
            ShoppingAssistantRequest(
                message=f"ao thun, lien he {PHONE} {EMAIL} cccd {CCCD}", user_id="u1"
            )
        )

    _assert_no_raw_pii(lines)
    assert any("[phone]" in line and "[email]" in line for line in lines)


async def test_chat_copilot_log_masks_buyer_message():
    service = AIAssistantService()

    with _captured_logs() as lines:
        await service.chat_copilot(
            ChatCopilotRequest(buyer_message=f"goi em {PHONE} hoac {EMAIL}")
        )

    _assert_no_raw_pii(lines)


async def test_magic_listing_log_masks_hint():
    service = AIAssistantService()

    with _captured_logs() as lines:
        await service.magic_listing(MagicListingRequest(title_hint=f"ao {PHONE}"))

    _assert_no_raw_pii(lines)


async def test_full_trace_mode_keeps_the_configured_policy():
    service = AIAssistantService(redaction_policy=RedactionPolicy(mode="full"))

    with _captured_logs() as lines:
        await service.chat_copilot(ChatCopilotRequest(buyer_message=f"sdt {PHONE}"))

    assert PHONE in "\n".join(lines)


async def test_off_mode_logs_a_placeholder_not_the_message():
    service = AIAssistantService(redaction_policy=RedactionPolicy(mode="off"))

    with _captured_logs() as lines:
        await service.shopping_assistant(
            ShoppingAssistantRequest(message=f"ao thun, lien he {PHONE}", user_id="u1")
        )
        await service.chat_copilot(ChatCopilotRequest(buyer_message=f"sdt {PHONE}"))
        await service.magic_listing(MagicListingRequest(title_hint=f"ao {PHONE}"))

    blob = "\n".join(lines)
    assert PHONE not in blob
    assert "ao thun" not in blob, "off must not log any user text, only a placeholder"
    query_line = next(line for line in lines if "shopping_assistant.query" in line)
    assert "message=[redacted]" in query_line
