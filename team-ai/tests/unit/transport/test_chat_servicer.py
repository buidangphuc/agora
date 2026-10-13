"""ChatService servicer: scopes, session bound, streaming and error mapping over a real channel."""

from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import grpc
import pytest

from app.modules.business.chat.echo import EchoChatService
from app.modules.business.chat.errors import (
    ChatDeadlineExceeded,
    ChatError,
    ChatInterrupted,
    ChatModelRateLimited,
    ChatModelUnavailable,
    ChatQuotaExhausted,
)
from app.modules.business.chat.ports import ChatService
from app.modules.platform.rate_limit.service import RateLimitResult
from app.transport.grpc._pb.platform.chat.v1 import chat_pb2, chat_pb2_grpc
from app.transport.grpc.server import build_grpc_server
from app.transport.grpc.servicers.chat import (
    MAX_SESSION_ID_LEN,
    chat_service_registration,
)
from tests.factories import allow_all_limiter


def _md(principal_id: str = "u1", *scopes: str, ptype: str = "user"):
    return (
        ("x-principal-id", principal_id),
        ("x-principal-type", ptype),
        ("x-principal-scopes", ",".join(scopes)),
    )


class _FailingChat:
    def __init__(self, error: Exception, deltas: tuple[str, ...] = ()) -> None:
        self._error = error
        self._deltas = deltas

    async def stream(self, message: str, **kwargs) -> AsyncIterator[str]:
        for delta in self._deltas:
            yield delta
        raise self._error


class _RecordingChat:
    def __init__(self) -> None:
        self.kwargs: dict = {}

    async def stream(self, message: str, **kwargs) -> AsyncIterator[str]:
        self.kwargs = kwargs
        yield "ok "


@asynccontextmanager
async def _serving(
    chat: ChatService, *, require_ai_use: bool = False, rate_limiter=None
) -> AsyncIterator[chat_pb2_grpc.ChatServiceStub]:
    server = build_grpc_server(
        registrations=(chat_service_registration(chat, require_ai_use=require_ai_use),),
        rate_limiter=rate_limiter or allow_all_limiter(),
    )
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        async with grpc.aio.insecure_channel(f"127.0.0.1:{port}") as channel:
            yield chat_pb2_grpc.ChatServiceStub(channel)
    finally:
        await server.stop(None)


async def _collect(stub, message: str = "hi", *, session_id: str = "s1", md=None):
    call = stub.StreamChat(
        chat_pb2.StreamChatRequest(session_id=session_id, message=message),
        metadata=md or _md(),
    )
    return [(r.delta, r.done) async for r in call], call


async def _fails(stub, **kwargs) -> grpc.aio.AioRpcError:
    with pytest.raises(grpc.aio.AioRpcError) as exc:
        await _collect(stub, **kwargs)
    return exc.value


def test_stream_chat_is_rate_limited():
    registration = chat_service_registration(EchoChatService(), require_ai_use=False)

    assert registration.service_name == "platform.chat.v1.ChatService"
    assert registration.rate_limited_methods == frozenset(
        {"/platform.chat.v1.ChatService/StreamChat"}
    )


class _BurstLimiter:
    """Allows ``limit`` calls, then rejects with a 7s retry-after."""

    def __init__(self, limit: int) -> None:
        self._remaining = limit

    async def check(self, key: str) -> RateLimitResult:
        if self._remaining > 0:
            self._remaining -= 1
            return RateLimitResult(allowed=True, remaining=self._remaining)
        return RateLimitResult(allowed=False, remaining=0, retry_after_seconds=7)


async def test_stream_chat_burst_over_the_limit_is_resource_exhausted():
    async with _serving(EchoChatService(), rate_limiter=_BurstLimiter(2)) as stub:
        for _ in range(2):
            responses, _ = await _collect(stub, "hi")
            assert responses[-1] == ("", True)

        error = await _fails(stub, message="hi")

    assert error.code() == grpc.StatusCode.RESOURCE_EXHAUSTED
    assert dict(error.trailing_metadata())["retry-after"] == "7"


async def test_mock_backend_streams_echo_then_done():
    async with _serving(EchoChatService()) as stub:
        responses, _ = await _collect(stub, "hi")

    assert responses == [("echo: ", False), ("hi ", False), ("", True)]


async def test_requires_ai_use_when_flag_on():
    async with _serving(EchoChatService(), require_ai_use=True) as stub:
        error = await _fails(stub)
        assert error.code() == grpc.StatusCode.PERMISSION_DENIED
        assert error.details() == "insufficient_scope: missing ['ai:use']"

        responses, _ = await _collect(stub, md=_md("u1", "ai:use"))
        assert responses[-1] == ("", True)


async def test_session_id_too_long_is_invalid_argument():
    async with _serving(EchoChatService()) as stub:
        error = await _fails(stub, session_id="x" * (MAX_SESSION_ID_LEN + 1))

    assert error.code() == grpc.StatusCode.INVALID_ARGUMENT
    assert error.details() == "session_id must be at most 128 characters"


async def test_session_id_at_the_limit_is_accepted():
    async with _serving(EchoChatService()) as stub:
        responses, _ = await _collect(stub, session_id="x" * MAX_SESSION_ID_LEN)

    assert responses[-1] == ("", True)


async def test_forwards_principal_request_id_and_deadline():
    chat = _RecordingChat()
    async with _serving(chat) as stub:
        call = stub.StreamChat(
            chat_pb2.StreamChatRequest(session_id="s9", message="hi"),
            metadata=_md("u42"),
            timeout=30,
        )
        _ = [r async for r in call]

    assert chat.kwargs["principal_id"] == "u42"
    assert chat.kwargs["session_id"] == "s9"
    assert chat.kwargs["request_id"]
    assert 0 < chat.kwargs["deadline_seconds"] <= 31


async def test_anonymous_principal_maps_to_empty_id():
    chat = _RecordingChat()
    async with _serving(chat) as stub:
        await _collect(stub, md=_md("anonymous", ptype="anonymous"))

    assert chat.kwargs["principal_id"] == ""
    assert chat.kwargs["deadline_seconds"] is None


@pytest.mark.parametrize(
    ("error", "code", "message"),
    [
        (
            ChatDeadlineExceeded(),
            grpc.StatusCode.DEADLINE_EXCEEDED,
            "deadline exceeded",
        ),
        (ChatQuotaExhausted(), grpc.StatusCode.RESOURCE_EXHAUSTED, "quota exceeded"),
        (
            ChatModelRateLimited(),
            grpc.StatusCode.RESOURCE_EXHAUSTED,
            "model rate limited",
        ),
        (ChatModelUnavailable(), grpc.StatusCode.UNAVAILABLE, "model unavailable"),
        (ChatInterrupted(), grpc.StatusCode.UNAVAILABLE, "model unavailable"),
        (ChatError("secret detail"), grpc.StatusCode.INTERNAL, "internal error"),
        (RuntimeError("secret detail"), grpc.StatusCode.INTERNAL, "internal error"),
    ],
)
async def test_errors_map_to_fixed_status_and_message(error, code, message):
    async with _serving(_FailingChat(error)) as stub:
        failure = await _fails(stub)

    assert failure.code() == code
    assert failure.details() == message
    assert "secret" not in (failure.debug_error_string() or "")


async def test_failure_after_deltas_has_no_done_chunk():
    async with _serving(_FailingChat(ChatInterrupted(), ("a ", "b "))) as stub:
        call = stub.StreamChat(
            chat_pb2.StreamChatRequest(session_id="s", message="hi"),
            metadata=_md(),
        )
        seen: list[tuple[str, bool]] = []
        with pytest.raises(grpc.aio.AioRpcError) as exc:
            async for r in call:
                seen.append((r.delta, r.done))

    assert seen == [("a ", False), ("b ", False)]
    assert exc.value.code() == grpc.StatusCode.UNAVAILABLE


async def test_quota_exhausted_sets_retry_after_trailer():
    async with _serving(_FailingChat(ChatQuotaExhausted(30))) as stub:
        failure = await _fails(stub)

    assert dict(failure.trailing_metadata() or ())["retry-after"] == "30"


async def test_retry_after_trailer_absent_without_a_value():
    async with _serving(_FailingChat(ChatQuotaExhausted(None))) as stub:
        failure = await _fails(stub)

    assert "retry-after" not in dict(failure.trailing_metadata() or ())


async def test_other_errors_set_no_retry_after_trailer():
    async with _serving(_FailingChat(ChatModelRateLimited())) as stub:
        failure = await _fails(stub)

    assert "retry-after" not in dict(failure.trailing_metadata() or ())
