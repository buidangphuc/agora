from typing import Any

from app.modules.ai.llm.langfuse import LangfuseLLMTracker
from app.modules.ai.llm.prompt import (
    CHAT_SYSTEM_PROMPT_NAME,
    STATIC_SYSTEM_PROMPT,
    PromptProvider,
)


class _Prompt:
    def __init__(self, text: Any) -> None:
        self._text = text

    def compile(self) -> Any:
        return self._text


class _Client:
    def __init__(self, prompt: Any = None, error: Exception | None = None) -> None:
        self.prompt = prompt
        self.error = error
        self.requested: list[str] = []

    def get_prompt(self, name: str, **kwargs: Any) -> Any:
        self.requested.append(name)
        if self.error is not None:
            raise self.error
        return self.prompt


def _tracker(client: Any = None) -> LangfuseLLMTracker:
    return LangfuseLLMTracker(instance_id="t", service_name="s", client=client)


async def test_langfuse_prompt_named_team_ai_chat_system_is_used():
    client = _Client(prompt=_Prompt("  Be brief.  "))

    text = await PromptProvider(_tracker(client)).system_prompt()

    assert CHAT_SYSTEM_PROMPT_NAME == "team-ai/chat-system"
    assert client.requested == ["team-ai/chat-system"]
    assert text == "Be brief."


async def test_langfuse_disabled_or_no_tracker_uses_the_fallback():
    assert await PromptProvider(_tracker()).system_prompt() == STATIC_SYSTEM_PROMPT
    assert await PromptProvider(None).system_prompt() == STATIC_SYSTEM_PROMPT


async def test_langfuse_unreachable_uses_the_fallback():
    client = _Client(error=RuntimeError("langfuse down"))

    text = await PromptProvider(_tracker(client)).system_prompt()

    assert text == STATIC_SYSTEM_PROMPT


async def test_empty_or_unusable_langfuse_prompt_uses_the_fallback():
    for prompt in (_Prompt("   "), _Prompt(None), object()):
        text = await PromptProvider(_tracker(_Client(prompt=prompt))).system_prompt()
        assert text == STATIC_SYSTEM_PROMPT


async def test_a_configured_fallback_replaces_the_static_prompt():
    provider = PromptProvider(None, fallback="You are a terse bot.")

    assert await provider.system_prompt() == "You are a terse bot."
    failing = PromptProvider(
        _tracker(_Client(error=RuntimeError("down"))), fallback="custom"
    )
    assert await failing.system_prompt() == "custom"


def test_static_prompt_is_team_ais_text_verbatim():
    assert STATIC_SYSTEM_PROMPT.startswith(
        "You are the shopping assistant of an online marketplace."
    )
    assert STATIC_SYSTEM_PROMPT.endswith("never try to guess it.")
