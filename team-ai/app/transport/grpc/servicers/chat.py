"""ChatService gRPC servicer — server-streaming reply tokens.

Requires ``ai:use`` when ``AI_USE_SCOPE_REQUIRED`` is on (checked before the
streamer is touched), then streams deltas from the ``ChatStreamer`` seam (external LLM
via the router, or mock). A final ``done=True`` chunk closes the stream. The
Gateway adapts this to SSE for browsers at the edge (out of scope here).
"""

from __future__ import annotations

from collections.abc import AsyncIterator

import grpc

from app.transport.grpc._pb.platform.chat.v1 import chat_pb2, chat_pb2_grpc
from app.transport.grpc.chat_stream import ChatStreamer
from app.transport.grpc.context import ensure_scopes
from app.transport.grpc.scopes import ai_use_scopes


class ChatServicer(chat_pb2_grpc.ChatServiceServicer):
    def __init__(self, streamer: ChatStreamer, *, require_ai_use: bool = False) -> None:
        self._streamer = streamer
        self._scopes = ai_use_scopes(require_ai_use)

    async def StreamChat(
        self,
        request: chat_pb2.StreamChatRequest,
        context: grpc.aio.ServicerContext,
    ) -> AsyncIterator[chat_pb2.StreamChatResponse]:
        # StreamChat is the only real-LLM path (cost): same ``ai:use`` gate as
        # ShoppingAssistant, checked BEFORE the streamer runs. It is empty until
        # ``AI_USE_SCOPE_REQUIRED`` because team-identity grants no ``ai:use`` yet
        # (and "chat:read" is not granted to any role either).
        await ensure_scopes(context, *self._scopes)
        async for delta in self._streamer.astream(
            request.message, session_id=request.session_id
        ):
            yield chat_pb2.StreamChatResponse(delta=delta, done=False)
        yield chat_pb2.StreamChatResponse(delta="", done=True)
