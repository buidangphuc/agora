import asyncio
from collections.abc import AsyncIterator
from typing import Any

import pytest
from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import AIMessageChunk, HumanMessage
from pydantic import Field

from app.core.errors import ServiceUnavailableError
from app.core.resilience import BreakerState
from app.modules.ai.llm.runtime import (
    LLMDeadlineExceeded,
    LLMStreamInterrupted,
    build_llm_instance,
)


class _StatusError(Exception):
    def __init__(self, status_code: int) -> None:
        super().__init__(f"status {status_code}")
        self.status_code = status_code


class _ScriptedModel(BaseChatModel):
    """Streams its script: str = text chunk, chunk = as is, Exception = raise,
    float = sleep. ``events`` records start/close so tests can see who ran."""

    steps: list[Any]
    events: list[str] = Field(default_factory=list)

    @property
    def _llm_type(self) -> str:
        return "scripted"

    def _generate(self, *args: Any, **kwargs: Any) -> Any:
        raise NotImplementedError

    async def astream(  # type: ignore[override]
        self, input: Any, config: Any = None, **kwargs: Any
    ) -> AsyncIterator[AIMessageChunk]:
        self.events.append("start")
        try:
            for step in self.steps:
                if isinstance(step, Exception):
                    raise step
                if isinstance(step, float):
                    await asyncio.sleep(step)
                    continue
                yield (
                    step
                    if isinstance(step, AIMessageChunk)
                    else AIMessageChunk(content=step)
                )
        finally:
            self.events.append("closed")


def _instance(test_settings, scripts: dict[str, list[Any]], **overrides):
    models = {t: _ScriptedModel(steps=s) for t, s in scripts.items()}
    settings = test_settings.model_copy(
        update={
            "CHAT_MODEL": "primary",
            "CHAT_FALLBACK_MODELS": ",".join(list(scripts)[1:]),
            "LLM_BREAKER_FAILURE_THRESHOLD": 2,
            **overrides,
        }
    )
    instance = build_llm_instance(
        settings,
        instance_id="t",
        service_name="test",
        model_builder=lambda target: models[target],
    )
    return instance, models


def _spy(instance) -> list[tuple[str, str]]:
    calls: list[tuple[str, str]] = []
    router = instance.router
    for name in ("record_success", "record_error", "release"):
        original = getattr(router, name)

        def wrapper(target, *args, _name=name, _original=original, **kwargs):
            calls.append((_name, target))
            return _original(target, *args, **kwargs)

        setattr(router, name, wrapper)
    return calls


async def _texts(instance, **kwargs: Any) -> list[str]:
    kwargs.setdefault("first_token_timeout", 5.0)
    return [
        str(chunk.content)
        async for chunk in instance.astream([HumanMessage(content="hi")], **kwargs)
        if chunk.content
    ]


async def test_streams_the_primary_and_records_success_once(test_settings):
    instance, models = _instance(
        test_settings, {"primary": ["a", "b"], "secondary": ["x"]}
    )
    calls = _spy(instance)

    assert await _texts(instance) == ["a", "b"]

    assert calls == [("record_success", "primary")]
    assert models["secondary"].events == []


async def test_falls_back_on_a_transient_error_before_the_first_text(test_settings):
    instance, models = _instance(
        test_settings, {"primary": [_StatusError(503)], "secondary": ["x", "y"]}
    )
    calls = _spy(instance)

    assert await _texts(instance) == ["x", "y"]

    assert calls == [("record_error", "primary"), ("record_success", "secondary")]
    assert models["primary"].events == ["start", "closed"]


async def test_falls_back_when_no_text_arrives_within_the_first_token_timeout(
    test_settings,
):
    instance, models = _instance(
        test_settings, {"primary": [5.0, "late"], "secondary": ["fast"]}
    )
    calls = _spy(instance)

    assert await _texts(instance, first_token_timeout=0.05) == ["fast"]

    assert calls == [("record_error", "primary"), ("record_success", "secondary")]
    assert models["primary"].events == ["start", "closed"]


async def test_non_text_chunks_do_not_count_as_the_first_chunk(test_settings):
    usage_only = AIMessageChunk(
        content="",
        usage_metadata={"input_tokens": 1, "output_tokens": 0, "total_tokens": 1},
    )
    instance, _ = _instance(
        test_settings,
        {"primary": ["", usage_only, _StatusError(503)], "secondary": ["", "ok"]},
    )

    chunks = [
        chunk
        async for chunk in instance.astream(
            [HumanMessage(content="hi")], first_token_timeout=5.0
        )
    ]

    # the failed target's chunks are dropped; the serving target's all pass through
    assert [str(c.content) for c in chunks] == ["", "ok"]


async def test_non_text_chunks_are_passed_through_in_order(test_settings):
    usage = AIMessageChunk(
        content="",
        usage_metadata={"input_tokens": 1, "output_tokens": 2, "total_tokens": 3},
    )
    instance, _ = _instance(test_settings, {"primary": ["", "a", usage]})

    chunks = [
        c
        async for c in instance.astream(
            [HumanMessage(content="hi")], first_token_timeout=5.0
        )
    ]

    assert [str(c.content) for c in chunks] == ["", "a", ""]
    assert chunks[-1].usage_metadata == usage.usage_metadata


async def test_deterministic_error_propagates_without_trying_the_next_target(
    test_settings,
):
    instance, models = _instance(
        test_settings, {"primary": [_StatusError(400)], "secondary": ["x"]}
    )

    with pytest.raises(_StatusError):
        await _texts(instance)

    assert models["secondary"].events == []


async def test_failure_after_text_raises_stream_interrupted_and_never_switches(
    test_settings,
):
    boom = _StatusError(503)
    instance, models = _instance(
        test_settings, {"primary": ["par", boom], "secondary": ["x"]}
    )
    calls = _spy(instance)
    seen: list[str] = []

    with pytest.raises(LLMStreamInterrupted) as info:
        async for chunk in instance.astream(
            [HumanMessage(content="hi")], first_token_timeout=5.0
        ):
            seen.append(str(chunk.content))

    assert seen == ["par"]
    assert info.value.__cause__ is boom
    assert info.value.status_code == 503
    assert info.value.code == "llm_stream_interrupted"
    assert calls == [("record_error", "primary")]
    assert models["secondary"].events == []


async def test_every_target_failing_raises_the_last_error(test_settings):
    last = _StatusError(502)
    instance, _ = _instance(
        test_settings,
        {"primary": [_StatusError(503)], "secondary": [last]},
    )

    with pytest.raises(_StatusError) as info:
        await _texts(instance)

    assert info.value is last


async def test_every_breaker_open_raises_service_unavailable(test_settings):
    instance, models = _instance(
        test_settings,
        {"primary": [_StatusError(503)], "secondary": [_StatusError(503)]},
    )
    for _ in range(2):
        with pytest.raises(_StatusError):
            await _texts(instance)
    breakers = instance.router._breakers
    assert breakers[("default", "primary")].state is BreakerState.OPEN
    started = len(models["primary"].events)

    with pytest.raises(ServiceUnavailableError):
        await _texts(instance)

    assert len(models["primary"].events) == started


async def test_chain_deadline_bounds_the_pre_first_chunk_walk(test_settings):
    instance, models = _instance(
        test_settings,
        {"primary": [5.0, "late"], "secondary": ["x"]},
        LLM_CHAIN_DEADLINE_SECONDS=0.05,
    )
    calls = _spy(instance)

    with pytest.raises(LLMDeadlineExceeded):
        await _texts(instance, first_token_timeout=5.0)

    assert models["primary"].events == ["start", "closed"]
    assert models["secondary"].events == []
    assert calls == [("release", "primary")]
    assert instance.router._breakers[("default", "primary")].failure_count == 0


async def test_caller_deadline_cuts_the_stream_mid_reply(test_settings):
    instance, models = _instance(test_settings, {"primary": ["a", 5.0, "b"]})
    calls = _spy(instance)
    seen: list[str] = []

    with pytest.raises(LLMDeadlineExceeded):
        async for chunk in instance.astream(
            [HumanMessage(content="hi")],
            first_token_timeout=5.0,
            deadline_seconds=0.1,
        ):
            seen.append(str(chunk.content))

    assert seen == ["a"]
    assert models["primary"].events == ["start", "closed"]
    assert calls == [("release", "primary")]


async def test_caller_deadline_already_spent_raises_before_any_call(test_settings):
    instance, models = _instance(test_settings, {"primary": ["a"]})

    with pytest.raises(LLMDeadlineExceeded):
        await _texts(instance, deadline_seconds=0)

    assert models["primary"].events == []


async def test_closing_the_stream_early_closes_upstream_without_a_failure(
    test_settings,
):
    instance, models = _instance(test_settings, {"primary": ["a", "b", "c"]})
    calls = _spy(instance)

    stream = instance.astream([HumanMessage(content="hi")], first_token_timeout=5.0)
    await anext(stream)
    await stream.aclose()  # type: ignore[attr-defined]

    assert models["primary"].events == ["start", "closed"]
    assert calls == [("release", "primary")]
