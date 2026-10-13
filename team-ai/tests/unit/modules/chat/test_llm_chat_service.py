import asyncio
from collections.abc import AsyncIterator
from typing import Any, cast

import pytest
from langchain_core.messages import AIMessageChunk, HumanMessage, SystemMessage
from loguru import logger

from app.core.errors import ServiceUnavailableError
from app.modules.ai.llm.prompt import STATIC_SYSTEM_PROMPT, PromptProvider
from app.modules.ai.llm.runtime import (
    LLMDeadlineExceeded,
    LLMInstance,
    LLMStreamInterrupted,
)
from app.modules.ai.llm.session import InMemorySessionStore, Turn
from app.modules.business.chat.errors import (
    ChatDeadlineExceeded,
    ChatInterrupted,
    ChatModelRateLimited,
    ChatModelUnavailable,
    ChatQuotaExhausted,
)
from app.modules.business.chat.factory import CHAT_QUOTA_RESOURCE
from app.modules.business.chat.service import LLMChatService
from app.modules.platform.quota.factory import (
    build_memory_quota_store,
    build_quota_service,
)
from app.modules.platform.quota.models import QuotaPolicy

POLICY = QuotaPolicy(resource=CHAT_QUOTA_RESOURCE, limit=2, window_seconds=3600)


class _Status(Exception):
    def __init__(self, status_code: int) -> None:
        super().__init__("provider secret detail")
        self.status_code = status_code


class _FakeLLM:
    """One script per call: str = text chunk, chunk = as is, Exception = raise."""

    def __init__(self, *scripts: list[Any]) -> None:
        self.scripts = list(scripts)
        self.calls: list[dict[str, Any]] = []

    async def astream(self, input: Any, **kwargs: Any) -> AsyncIterator[Any]:
        self.calls.append({"input": input, **kwargs})
        for step in self.scripts.pop(0):
            if isinstance(step, Exception):
                raise step
            if isinstance(step, AIMessageChunk):
                yield step
            else:
                yield AIMessageChunk(content=step)


def _sessions() -> InMemorySessionStore:
    return InMemorySessionStore(ttl_seconds=100, max_turns=5, max_chars=1000)


def _service(llm: Any, *, quota=None, sessions=None, prompts=None, **kw):
    return LLMChatService(
        cast(LLMInstance, llm),
        first_token_timeout=kw.get("first_token_timeout", 3.0),
        quota=quota,
        quota_policy=POLICY,
        sessions=sessions or _sessions(),
        prompts=prompts or PromptProvider(None),
    )


async def _collect(service, message="hi", **kw) -> list[str]:
    return [
        d
        async for d in service.stream(
            message,
            session_id=kw.get("session_id", "s1"),
            principal_id=kw.get("principal_id", "u1"),
            request_id=kw.get("request_id", "r1"),
            deadline_seconds=kw.get("deadline_seconds"),
        )
    ]


async def _used(quota, principal="u1") -> int:
    usage = await quota.get_usage(
        subject_id=principal, resource=CHAT_QUOTA_RESOURCE, policy=POLICY
    )
    return usage.used


def _quota():
    return build_quota_service(build_memory_quota_store())


class _BrokenQuota:
    async def reserve(self, **kwargs: Any) -> Any:
        raise RuntimeError("quota backend down")

    async def finalize(self, reservation: Any) -> Any:
        raise RuntimeError("quota backend down")

    async def refund(self, reservation: Any) -> Any:
        raise RuntimeError("quota backend down")


class _BrokenSessions:
    async def load(self, principal_id: str, session_id: str) -> list[Turn]:
        raise RuntimeError("redis down")

    async def append(self, principal_id: str, session_id: str, turn: Turn) -> None:
        raise RuntimeError("redis down")


@pytest.fixture
def logs():
    records: list[Any] = []
    sink = logger.add(lambda m: records.append(m.record), level="DEBUG")
    yield records
    logger.remove(sink)


async def test_streams_text_deltas_and_passes_trace_context_and_deadline():
    llm = _FakeLLM(["a", "", "b"])

    deltas = await _collect(
        _service(llm, first_token_timeout=2.5), deadline_seconds=9.0, request_id="rq"
    )

    assert deltas == ["a", "b"]
    call = llm.calls[0]
    assert call["first_token_timeout"] == 2.5
    assert call["deadline_seconds"] == 9.0
    ctx = call["context"]
    assert (ctx.session_id, ctx.user_id, ctx.request_id) == ("s1", "u1", "rq")
    assert ctx.tags == ("grpc", "chat")


async def test_message_is_redacted_and_history_is_replayed():
    sessions = _sessions()
    await sessions.append("u1", "s1", Turn("old q", "old a"))
    llm = _FakeLLM(["ok"])

    await _collect(
        _service(llm, sessions=sessions), "mail me at a@b.com, CCCD 012345678901"
    )

    system, h1, a1, human = llm.calls[0]["input"]
    assert isinstance(system, SystemMessage)
    assert system.content == STATIC_SYSTEM_PROMPT
    assert (h1.content, a1.content) == ("old q", "old a")
    assert isinstance(human, HumanMessage)
    assert "a@b.com" not in human.content
    assert "012345678901" not in human.content
    assert "[id]" in human.content


async def test_complete_reply_is_remembered_redacted():
    sessions = _sessions()

    await _collect(
        _service(_FakeLLM(["he", "llo"]), sessions=sessions), "write a@b.com"
    )

    [turn] = await sessions.load("u1", "s1")
    assert turn.assistant == "hello"
    assert "a@b.com" not in turn.user


async def test_quota_exhausted_makes_no_model_call():
    quota = _quota()
    llm = _FakeLLM(["1"], ["2"])
    service = _service(llm, quota=quota)
    await _collect(service)
    await _collect(service)

    with pytest.raises(ChatQuotaExhausted) as info:
        await _collect(service)

    assert info.value.retry_after_seconds is not None
    assert info.value.retry_after_seconds >= 1
    assert len(llm.calls) == 2


async def test_successful_reply_is_charged():
    quota = _quota()

    await _collect(_service(_FakeLLM(["x"]), quota=quota))

    assert await _used(quota) == 1


async def test_quota_backend_failure_fails_open(logs):
    deltas = await _collect(_service(_FakeLLM(["x"]), quota=_BrokenQuota()))

    assert deltas == ["x"]
    warnings = [r for r in logs if r["level"].name == "WARNING"]
    assert warnings
    assert all("quota backend down" not in r["message"] for r in warnings)
    assert any("RuntimeError" in r["message"] for r in warnings)


async def test_partial_reply_is_charged_and_not_remembered():
    quota = _quota()
    sessions = _sessions()
    service = _service(
        _FakeLLM(["par", LLMStreamInterrupted()]), quota=quota, sessions=sessions
    )
    received: list[str] = []

    with pytest.raises(ChatInterrupted):
        async for d in service.stream(
            "hi",
            session_id="s1",
            principal_id="u1",
            request_id="r",
            deadline_seconds=None,
        ):
            received.append(d)

    assert received == ["par"]
    assert await _used(quota) == 1
    assert await sessions.load("u1", "s1") == []


@pytest.mark.parametrize(
    ("error", "expected"),
    [
        (_Status(429), ChatModelRateLimited),
        (_Status(503), ChatModelUnavailable),
        (
            ServiceUnavailableError(message="down", code="llm_unavailable"),
            ChatModelUnavailable,
        ),
        (LLMDeadlineExceeded(5), ChatDeadlineExceeded),
    ],
)
async def test_failure_before_text_is_refunded_and_classified(error, expected):
    quota = _quota()

    with pytest.raises(expected):
        await _collect(_service(_FakeLLM([error]), quota=quota))

    assert await _used(quota) == 0


async def test_rate_limit_is_found_on_the_cause():
    error = LLMStreamInterrupted()
    error.__cause__ = _Status(429)

    with pytest.raises(ChatModelRateLimited):
        await _collect(_service(_FakeLLM([error])))


async def test_empty_reply_is_refunded_and_not_remembered():
    quota = _quota()
    sessions = _sessions()

    deltas = await _collect(_service(_FakeLLM([""]), quota=quota, sessions=sessions))

    assert deltas == []
    assert await _used(quota) == 0
    assert await sessions.load("u1", "s1") == []


async def test_client_disconnect_mid_stream_charges_the_partial_reply():
    quota = _quota()
    service = _service(_FakeLLM(["a", "b", "c"]), quota=quota)
    stream = service.stream(
        "hi", session_id="s1", principal_id="u1", request_id="r", deadline_seconds=None
    )

    assert await anext(stream) == "a"
    await stream.aclose()  # type: ignore[attr-defined]

    assert await _used(quota) == 1


async def test_cancel_before_text_refunds():
    quota = _quota()

    class _Hang(_FakeLLM):
        async def astream(self, input: Any, **kwargs: Any) -> AsyncIterator[Any]:
            await asyncio.sleep(30)
            yield AIMessageChunk(content="never")

    task = asyncio.create_task(_collect(_service(_Hang(), quota=quota)))
    await asyncio.sleep(0.05)
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task

    assert await _used(quota) == 0


async def test_history_failure_degrades_to_stateless(logs):
    deltas = await _collect(_service(_FakeLLM(["ok"]), sessions=_BrokenSessions()))

    assert deltas == ["ok"]
    warnings = [r for r in logs if r["level"].name == "WARNING"]
    assert len(warnings) >= 2
    assert all("redis down" not in r["message"] for r in warnings)


async def test_anonymous_caller_is_stateless_and_unmetered():
    quota = _quota()
    sessions = _sessions()
    llm = _FakeLLM(["ok"])

    await _collect(_service(llm, quota=quota, sessions=sessions), principal_id="")

    assert len(llm.calls[0]["input"]) == 2  # system + human, no history
    assert llm.calls[0]["context"].user_id is None
    assert await sessions.load("", "s1") == []
    assert await _used(quota, "") == 0


async def test_usage_is_logged_without_user_text(logs):
    chunk = AIMessageChunk(
        content="done",
        usage_metadata={"input_tokens": 7, "output_tokens": 3, "total_tokens": 10},
    )

    await _collect(_service(_FakeLLM([chunk])), "very secret question", request_id="rq")

    [record] = [r for r in logs if "chat.llm.usage" in r["message"]]
    extra = record["extra"]
    assert extra["request_id"] == "rq"
    assert (extra["input_tokens"], extra["output_tokens"], extra["total_tokens"]) == (
        7,
        3,
        10,
    )
    assert extra["estimated"] is False
    assert "target" in extra
    assert all("very secret question" not in r["message"] for r in logs)


async def test_usage_is_estimated_when_provider_reports_none(logs):
    await _collect(_service(_FakeLLM(["abcdefgh"])))

    [record] = [r for r in logs if "chat.llm.usage" in r["message"]]
    assert record["extra"]["estimated"] is True
    assert record["extra"]["output_tokens"] == 2


async def test_prompt_fallback_is_used_as_system_prompt():
    llm = _FakeLLM(["ok"])

    await _collect(_service(llm, prompts=PromptProvider(None, fallback="custom")))

    assert llm.calls[0]["input"][0].content == "custom"
