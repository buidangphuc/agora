"""ChatServicer: deadline plumbing and error -> gRPC status mapping."""

from __future__ import annotations

import grpc
import pytest

from app.core.resilience import CircuitBreakerPolicy, FailureKind
from app.modules.ai.llm.router import ModelRouter
from app.modules.ai.llm.testing import Script, ScriptedProvider, StatusError
from app.transport.grpc._pb.platform.chat.v1 import chat_pb2
from app.transport.grpc.chat_stream import (
    ChainExhausted,
    ChatDeadlineExceeded,
    LLMRouterChatStreamer,
    StreamInterrupted,
)
from app.transport.grpc.servicers.chat import ChatServicer
from tests.factories import build_test_settings


@pytest.fixture(autouse=True)
def _caller_with_ai_use():
    """StreamChat requires ``ai:use``; bind a signed-in caller that holds it."""
    from app.modules.platform.identity.schemas import Principal
    from app.transport.grpc.context import bind_principal, reset_principal

    token = bind_principal(Principal(id="caller", type="user", scopes=("ai:use",)))
    yield
    reset_principal(token)


class _Aborted(Exception):
    def __init__(self, code: grpc.StatusCode, details: str) -> None:
        super().__init__(details)
        self.code = code
        self.details = details


class _FakeContext:
    def __init__(self, time_remaining: float | None = None) -> None:
        self._remaining = time_remaining

    def time_remaining(self) -> float | None:
        return self._remaining

    async def abort(self, code: grpc.StatusCode, details: str = ""):
        raise _Aborted(code, details)


class _ScriptedStreamer:
    """Yields deltas then raises ``error`` (or finishes); records kwargs."""

    def __init__(self, deltas=(), error: Exception | None = None) -> None:
        self._deltas = deltas
        self._error = error
        self.kwargs: dict = {}

    async def astream(self, message: str, **kwargs):
        self.kwargs = {"message": message, **kwargs}
        for delta in self._deltas:
            yield delta
        if self._error is not None:
            raise self._error


async def _drain(servicer: ChatServicer, context: _FakeContext):
    request = chat_pb2.StreamChatRequest(session_id="s1", message="hello")
    return [r async for r in servicer.StreamChat(request, context)]  # type: ignore[arg-type]


async def test_servicer_passes_remaining_deadline_and_ids_to_the_streamer():
    streamer = _ScriptedStreamer(("a", "b"))

    responses = await _drain(ChatServicer(streamer), _FakeContext(time_remaining=12.5))

    assert [r.delta for r in responses] == ["a", "b", ""]
    assert responses[-1].done is True
    assert streamer.kwargs["deadline_seconds"] == 12.5
    assert streamer.kwargs["session_id"] == "s1"
    assert "request_id" in streamer.kwargs


async def test_servicer_passes_no_deadline_when_the_caller_has_none():
    streamer = _ScriptedStreamer(("a",))

    await _drain(ChatServicer(streamer), _FakeContext(time_remaining=None))

    assert streamer.kwargs["deadline_seconds"] is None


@pytest.mark.parametrize(
    ("error", "code"),
    [
        (ChainExhausted(FailureKind.SERVER, 3), grpc.StatusCode.UNAVAILABLE),
        (ChainExhausted(FailureKind.TIMEOUT, 2), grpc.StatusCode.UNAVAILABLE),
        (ChainExhausted(None, 0), grpc.StatusCode.UNAVAILABLE),
        (
            ChainExhausted(FailureKind.RATE_LIMITED, 2),
            grpc.StatusCode.RESOURCE_EXHAUSTED,
        ),
        (StreamInterrupted(FailureKind.SERVER), grpc.StatusCode.UNAVAILABLE),
        (ChatDeadlineExceeded("late"), grpc.StatusCode.DEADLINE_EXCEEDED),
    ],
)
async def test_errors_map_to_grpc_status(error, code):
    servicer = ChatServicer(_ScriptedStreamer((), error))

    with pytest.raises(_Aborted) as info:
        await _drain(servicer, _FakeContext())

    assert info.value.code is code
    assert info.value.details in {
        "model unavailable",
        "model rate limited",
        "deadline exceeded",
    }


async def test_stream_interrupted_aborts_after_the_partial_deltas():
    servicer = ChatServicer(
        _ScriptedStreamer(("x", "y"), StreamInterrupted(FailureKind.TIMEOUT))
    )
    seen: list[str] = []
    request = chat_pb2.StreamChatRequest(session_id="s", message="hello")

    with pytest.raises(_Aborted) as info:
        async for response in servicer.StreamChat(request, _FakeContext()):  # type: ignore[arg-type]
            seen.append(response.delta)

    assert seen == ["x", "y"]  # no done=True after a failure
    assert info.value.code is grpc.StatusCode.UNAVAILABLE


async def test_upstream_is_cancelled_at_the_grpc_deadline_with_real_streamer():
    provider = ScriptedProvider().queue("a", Script(hang=True))
    router = ModelRouter(
        build_test_settings(CHAT_MODEL="a", CHAT_FALLBACK_MODELS="b"),
        model_builder=provider.builder,
        breaker_policy=CircuitBreakerPolicy(failure_threshold=3),
    )
    streamer = LLMRouterChatStreamer(router, first_token_timeout_seconds=30)

    with pytest.raises(_Aborted) as info:
        await _drain(ChatServicer(streamer), _FakeContext(time_remaining=0.1))

    assert info.value.code is grpc.StatusCode.DEADLINE_EXCEEDED
    assert provider.calls_to("a")[0].cancelled is True
    assert provider.calls_to("b") == []


async def test_chain_failure_with_real_streamer_maps_429_to_resource_exhausted():
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0, error=StatusError(429)))
    router = ModelRouter(
        build_test_settings(CHAT_MODEL="a", CHAT_FALLBACK_MODELS=""),
        model_builder=provider.builder,
    )
    streamer = LLMRouterChatStreamer(router)

    with pytest.raises(_Aborted) as info:
        await _drain(ChatServicer(streamer), _FakeContext(time_remaining=5))

    assert info.value.code is grpc.StatusCode.RESOURCE_EXHAUSTED


async def test_exception_text_never_reaches_the_client_and_is_logged(caplog_loguru):
    secret = "sk-live-TOPSECRET provider said: user phone 0912345678"
    servicer = ChatServicer(_ScriptedStreamer(("x",), RuntimeError(secret)))

    with pytest.raises(_Aborted) as info:
        await _drain(servicer, _FakeContext())

    assert info.value.code is grpc.StatusCode.INTERNAL
    assert info.value.details == "internal error"
    assert "TOPSECRET" not in info.value.details
    # The original exception is kept server-side, tagged with the request id.
    assert any(
        "grpc.StreamChat.error" in line and "RuntimeError" in line
        for line in caplog_loguru
    )


async def test_chain_failure_message_carries_no_provider_text():
    provider = ScriptedProvider()
    provider.queue(
        "a", Script(fail_at=0, error=StatusError(500, "upstream said: sk-abc leaked"))
    )
    router = ModelRouter(
        build_test_settings(CHAT_MODEL="a", CHAT_FALLBACK_MODELS=""),
        model_builder=provider.builder,
    )
    streamer = LLMRouterChatStreamer(router, max_attempts=1)

    with pytest.raises(_Aborted) as info:
        await _drain(ChatServicer(streamer), _FakeContext(time_remaining=5))

    assert info.value.code is grpc.StatusCode.UNAVAILABLE
    assert info.value.details == "model unavailable"
