"""Token usage capture and the structured usage log line."""

from __future__ import annotations

import json

import pytest
from loguru import logger

from app.modules.ai.llm.router import ModelRouter
from app.modules.ai.llm.testing import Script, ScriptedProvider
from app.transport.grpc.chat_stream import ChainExhausted, LLMRouterChatStreamer
from tests.factories import build_test_settings


def _streamer(provider: ScriptedProvider) -> LLMRouterChatStreamer:
    router = ModelRouter(
        build_test_settings(CHAT_MODEL="a", CHAT_FALLBACK_MODELS="b"),
        model_builder=provider.builder,
    )
    return LLMRouterChatStreamer(router)


async def _chat(streamer, message="hello world", request_id="req-usage-1"):
    return [
        d
        async for d in streamer.astream(
            message, session_id="", principal_id="u", request_id=request_id
        )
    ]


def _usage_records(records):
    return [r for r in records if r["message"].startswith("chat.llm.usage")]


async def test_provider_reported_usage_is_logged_with_the_serving_target(
    loguru_records,
):
    provider = ScriptedProvider().queue(
        "a",
        Script(
            chunks=("ok",),
            usage={"input_tokens": 11, "output_tokens": 7, "total_tokens": 18},
        ),
    )

    await _chat(_streamer(provider))

    (record,) = _usage_records(loguru_records)
    extra = record["extra"]
    assert extra["request_id"] == "req-usage-1"
    assert extra["target"] == "a"
    assert extra["input_tokens"] == 11
    assert extra["output_tokens"] == 7
    assert extra["estimated"] is False


async def test_usage_is_attributed_to_the_fallback_that_answered(loguru_records):
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0))
    provider.queue(
        "b",
        Script(
            chunks=("ok",),
            usage={"input_tokens": 3, "output_tokens": 2, "total_tokens": 5},
        ),
    )

    await _chat(_streamer(provider))

    (record,) = _usage_records(loguru_records)
    assert record["extra"]["target"] == "b"
    assert (record["extra"]["input_tokens"], record["extra"]["output_tokens"]) == (3, 2)


async def test_without_provider_usage_a_flagged_estimate_is_logged(loguru_records):
    provider = ScriptedProvider().queue("a", Script(chunks=("abcd", "efgh")))

    await _chat(_streamer(provider), message="x" * 40)

    (record,) = _usage_records(loguru_records)
    assert record["extra"]["estimated"] is True
    # system prompt + the 40-char message, estimated at 4 chars per token
    assert record["extra"]["input_tokens"] > 10
    assert record["extra"]["output_tokens"] == 2  # 8 chars / 4


async def test_no_usage_line_when_the_reply_fails_before_a_chunk(loguru_records):
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0))
    provider.queue("b", Script(fail_at=0))

    with pytest.raises(ChainExhausted):
        await _chat(_streamer(provider))

    assert _usage_records(loguru_records) == []


async def test_json_log_line_carries_the_required_keys():
    lines: list[str] = []
    handler_id = logger.add(lines.append, serialize=True, level="INFO")
    try:
        provider = ScriptedProvider().queue(
            "a",
            Script(
                chunks=("ok",),
                usage={"input_tokens": 11, "output_tokens": 7, "total_tokens": 18},
            ),
        )
        await _chat(_streamer(provider), request_id="e2e-123")
    finally:
        logger.remove(handler_id)

    usage = [
        json.loads(line)["record"]["extra"]
        for line in lines
        if "chat.llm.usage" in line
    ]
    assert len(usage) == 1
    assert usage[0]["request_id"] == "e2e-123"
    assert usage[0]["target"] == "a"
    assert usage[0]["input_tokens"] == 11
    assert usage[0]["output_tokens"] == 7
