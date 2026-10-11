"""Per-attempt Langfuse trace config on the chat path."""

from __future__ import annotations

from langchain_core.callbacks import BaseCallbackHandler

from app.modules.ai.llm.langfuse import LangfuseLLMTracker
from app.modules.ai.llm.router import ModelRouter
from app.modules.ai.llm.testing import Script, ScriptedProvider, StatusError
from app.transport.grpc.chat_stream import LLMRouterChatStreamer
from tests.factories import build_test_settings


class _Handler(BaseCallbackHandler):
    pass


def _tracker(*, enabled: bool, handler: _Handler | None = None) -> LangfuseLLMTracker:
    return LangfuseLLMTracker(
        instance_id="grpc-chat",
        service_name="team-ai.chat",
        enabled=enabled,
        callback_handler_factory=(lambda: handler) if handler else None,
    )


def _streamer(provider: ScriptedProvider, tracker, *, fallbacks: str = ""):
    router = ModelRouter(
        build_test_settings(CHAT_MODEL="a", CHAT_FALLBACK_MODELS=fallbacks),
        model_builder=provider.builder,
    )
    return LLMRouterChatStreamer(router, tracker=tracker, max_attempts=3)


async def _chat(streamer) -> list[str]:
    return [
        d
        async for d in streamer.astream(
            "hello",
            session_id="sess-1",
            principal_id="user-7",
            request_id="req-42",
        )
    ]


async def test_enabled_tracker_passes_session_user_and_request_ids():
    handler = _Handler()
    provider = ScriptedProvider().queue("a", Script(chunks=("ok",)))

    await _chat(_streamer(provider, _tracker(enabled=True, handler=handler)))

    config = provider.calls[0].config
    assert config is not None
    assert config["callbacks"] == [handler]
    metadata = config["metadata"]
    assert metadata["langfuse_session_id"] == "sess-1"
    assert metadata["langfuse_user_id"] == "user-7"
    assert metadata["request_id"] == "req-42"
    assert metadata["target"] == "a"
    assert metadata["attempt"] == 1
    assert {"grpc", "chat"} <= set(config["tags"])


async def test_each_attempt_carries_its_own_target_and_attempt_number():
    handler = _Handler()
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0, error=StatusError(500)))
    provider.queue("b", Script(chunks=("ok",)))

    await _chat(
        _streamer(provider, _tracker(enabled=True, handler=handler), fallbacks="b")
    )

    seen = [(c.target, c.config["metadata"]["target"]) for c in provider.calls]  # type: ignore[index]
    assert seen == [("a", "a"), ("b", "b")]


async def test_disabled_tracker_attaches_no_callbacks_and_output_is_identical():
    untraced = ScriptedProvider().queue("a", Script(chunks=("x", "y")))
    disabled = ScriptedProvider().queue("a", Script(chunks=("x", "y")))

    out_none = await _chat(_streamer(untraced, None))
    out_disabled = await _chat(_streamer(disabled, _tracker(enabled=False)))

    assert out_none == out_disabled == ["x", "y"]
    assert untraced.calls[0].config is None
    config = disabled.calls[0].config
    assert config is not None and "callbacks" not in config
