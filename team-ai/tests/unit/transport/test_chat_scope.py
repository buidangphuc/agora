"""StreamChat's ``ai:use`` gate runs before the streamer is touched."""

from __future__ import annotations

from collections.abc import AsyncIterator

import grpc
import pytest

from app.modules.platform.identity.schemas import Principal
from app.transport.grpc._pb.platform.chat.v1 import chat_pb2
from app.transport.grpc.context import bind_principal, reset_principal
from app.transport.grpc.scopes import AI_USE_SCOPE
from app.transport.grpc.servicers.chat import ChatServicer

ANONYMOUS = Principal(
    id="anonymous", type="anonymous", scopes=("listing.read", "search:read")
)
BUYER = Principal(id="buyer-1", type="user", scopes=("listing.read", AI_USE_SCOPE))
BUYER_OLD_TOKEN = Principal(id="buyer-2", type="user", scopes=("listing.read",))


class _Aborted(Exception):
    def __init__(self, code: grpc.StatusCode, details: str) -> None:
        super().__init__(details)
        self.code = code


class _FakeContext:
    async def abort(self, code: grpc.StatusCode, details: str = ""):
        raise _Aborted(code, details)

    def time_remaining(self) -> float | None:
        return None


class _SpyStreamer:
    def __init__(self) -> None:
        self.calls = 0

    async def astream(
        self, message: str, *, session_id: str, **_: object
    ) -> AsyncIterator[str]:
        self.calls += 1
        yield "hello"


async def _stream(servicer: ChatServicer, principal: Principal):
    token = bind_principal(principal)
    try:
        request = chat_pb2.StreamChatRequest(session_id="s1", message="xin chao")
        return [r async for r in servicer.StreamChat(request, _FakeContext())]  # type: ignore[arg-type]
    finally:
        reset_principal(token)


@pytest.mark.parametrize("principal", [ANONYMOUS, BUYER_OLD_TOKEN])
async def test_enforced_gate_denies_without_ai_use_before_the_streamer(principal):
    streamer = _SpyStreamer()
    servicer = ChatServicer(streamer, require_ai_use=True)  # type: ignore[arg-type]

    with pytest.raises(_Aborted) as info:
        await _stream(servicer, principal)

    assert info.value.code is grpc.StatusCode.PERMISSION_DENIED
    assert streamer.calls == 0


async def test_enforced_gate_lets_a_buyer_with_ai_use_stream_and_receive_done():
    streamer = _SpyStreamer()
    servicer = ChatServicer(streamer, require_ai_use=True)  # type: ignore[arg-type]

    responses = await _stream(servicer, BUYER)

    assert [r.delta for r in responses][:-1] == ["hello"]
    assert responses[-1].done is True
    assert streamer.calls == 1


@pytest.mark.parametrize("principal", [ANONYMOUS, BUYER_OLD_TOKEN, BUYER])
async def test_default_keeps_chat_open_until_identity_grants_ai_use(principal):
    streamer = _SpyStreamer()
    servicer = ChatServicer(streamer)  # type: ignore[arg-type]

    responses = await _stream(servicer, principal)

    assert responses[-1].done is True
    assert streamer.calls == 1
