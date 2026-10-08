"""System prompt: Langfuse prompt when available, static fallback otherwise."""

from __future__ import annotations

from app.modules.ai.llm.langfuse import LangfuseLLMTracker
from app.modules.ai.llm.prompt import STATIC_SYSTEM_PROMPT, PromptProvider
from app.modules.ai.llm.router import ModelRouter
from app.modules.ai.llm.testing import Script, ScriptedProvider
from app.transport.grpc.chat_stream import LLMRouterChatStreamer
from tests.factories import build_test_settings


class _Prompt:
    def __init__(self, text: str) -> None:
        self._text = text

    def compile(self) -> str:
        return self._text


class _Client:
    def __init__(self, prompt=None, error: Exception | None = None) -> None:
        self.prompt = prompt
        self.error = error
        self.requested: list[str] = []

    def get_prompt(self, name: str, **kwargs):
        self.requested.append(name)
        if self.error is not None:
            raise self.error
        return self.prompt


def _tracker(*, enabled: bool, client=None) -> LangfuseLLMTracker:
    return LangfuseLLMTracker(
        instance_id="t", service_name="s", enabled=enabled, client=client
    )


async def _first_messages(provider_under_test: PromptProvider):
    fake = ScriptedProvider().queue("a", Script(chunks=("ok",)))
    router = ModelRouter(
        build_test_settings(CHAT_MODEL="a", CHAT_FALLBACK_MODELS=""),
        model_builder=fake.builder,
    )
    streamer = LLMRouterChatStreamer(router, prompt_provider=provider_under_test)
    _ = [d async for d in streamer.astream("hi", session_id="")]
    return fake.calls[0].messages


async def test_langfuse_disabled_uses_static_system_prompt_first():
    messages = await _first_messages(PromptProvider(_tracker(enabled=False)))

    assert messages[0].type == "system"
    assert messages[0].content == STATIC_SYSTEM_PROMPT
    assert messages[-1].type == "human"


async def test_no_tracker_uses_static_system_prompt_first():
    messages = await _first_messages(PromptProvider())

    assert messages[0].content == STATIC_SYSTEM_PROMPT


async def test_langfuse_raising_falls_back_to_static_system_prompt():
    client = _Client(error=RuntimeError("langfuse down"))

    messages = await _first_messages(
        PromptProvider(_tracker(enabled=True, client=client))
    )

    assert client.requested == ["team-ai/chat-system"]
    assert messages[0].type == "system"
    assert messages[0].content == STATIC_SYSTEM_PROMPT


async def test_langfuse_prompt_is_used_when_available():
    client = _Client(prompt=_Prompt("  Be brief.  "))

    messages = await _first_messages(
        PromptProvider(_tracker(enabled=True, client=client))
    )

    assert messages[0].content == "Be brief."


async def test_empty_or_unusable_langfuse_prompt_falls_back():
    for prompt in (_Prompt("   "), _Prompt(None), object()):  # type: ignore[arg-type]
        client = _Client(prompt=prompt)
        text = await PromptProvider(
            _tracker(enabled=True, client=client)
        ).system_prompt()
        assert text == STATIC_SYSTEM_PROMPT


async def test_configured_system_prompt_replaces_the_static_one():
    from app.transport.grpc.chat_stream import build_chat_streamer

    settings = build_test_settings(
        CHAT_BACKEND="llm_router",
        CHAT_MODEL="a",
        CHAT_SYSTEM_PROMPT="  You are a terse bot.  ",
    )
    streamer = build_chat_streamer(settings)
    fake = ScriptedProvider().queue("a", Script(chunks=("ok",)))
    streamer._router.model_builder = fake.builder  # type: ignore[attr-defined]

    _ = [d async for d in streamer.astream("hi", session_id="")]  # type: ignore[attr-defined]

    assert fake.calls[0].messages[0].content == "You are a terse bot."
