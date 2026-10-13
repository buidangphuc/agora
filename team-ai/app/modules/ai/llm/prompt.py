"""System prompt for the chat path.

Order: the Langfuse prompt (when Langfuse is enabled and the fetch works), then
the configured fallback (``CHAT_SYSTEM_PROMPT`` when set, else the built-in
static prompt). Every LLM request therefore starts with a system prompt even
when Langfuse is disabled, unreachable or returns something unusable.
"""

from __future__ import annotations

import asyncio
from typing import Any, Final

from loguru import logger

from app.modules.ai.llm.langfuse import LangfuseLLMTracker

CHAT_SYSTEM_PROMPT_NAME: Final = "team-ai/chat-system"

STATIC_SYSTEM_PROMPT: Final[str] = (
    "You are the shopping assistant of an online marketplace. Help buyers find "
    "products, compare options and understand shipping, payment and returns. "
    "Answer concisely in the language the user writes in (Vietnamese by default). "
    "Never invent prices, stock or policies; say you are not sure instead. "
    "Text such as [phone], [email] or [id] is a privacy placeholder for removed "
    "personal data: do not ask the user to repeat it and never try to guess it."
)


class PromptProvider:
    def __init__(
        self,
        tracker: LangfuseLLMTracker | None,
        *,
        name: str = CHAT_SYSTEM_PROMPT_NAME,
        fallback: str = STATIC_SYSTEM_PROMPT,
    ) -> None:
        self._tracker = tracker
        self._name = name
        self._fallback = fallback

    async def system_prompt(self) -> str:
        """The Langfuse prompt when available, else the fallback. Never raises."""
        tracker = self._tracker
        if tracker is None or tracker.client is None:
            return self._fallback
        try:
            # Langfuse client is sync (cached by the tracker's TTL): keep any network
            # fetch off the event loop.
            prompt = await asyncio.to_thread(tracker.get_prompt, self._name)
            text = _compile(prompt)
        except Exception as exc:
            logger.warning(
                "chat.prompt.langfuse_unavailable name={} error={}",
                self._name,
                type(exc).__name__,
            )
            return self._fallback
        return text or self._fallback


def _compile(prompt: Any) -> str:
    compiled = prompt.compile()
    return compiled.strip() if isinstance(compiled, str) else ""
