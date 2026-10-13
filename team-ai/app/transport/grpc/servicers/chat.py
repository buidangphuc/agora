"""ChatService servicer: StreamChat maps business deltas and errors onto the wire.

The business ``ChatService`` owns the stream (history, quota, model); this servicer
only gates ``ai:use``, bounds the session id, forwards the caller's principal,
request id and remaining deadline, and maps each delta and ``ChatError`` to proto.
StreamChat is rate-limited by the interceptor; quota stays the chat service's job.
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from typing import Final

import grpc
from loguru import logger

from app.core.request_context import get_request_id
from app.modules.business.chat.errors import ChatQuotaExhausted
from app.modules.business.chat.ports import ChatService
from app.transport.grpc._pb.platform.chat.v1 import chat_pb2, chat_pb2_grpc
from app.transport.grpc.context import current_principal, ensure_scopes
from app.transport.grpc.errors import map_servicer_error
from app.transport.grpc.registry import GrpcServiceRegistration
from app.transport.grpc.scopes import ai_use_scopes

_SERVICE_NAME = "platform.chat.v1.ChatService"
_RATE_LIMITED_METHODS: Final = frozenset({f"/{_SERVICE_NAME}/StreamChat"})
MAX_SESSION_ID_LEN: Final = 128


class ChatServicer(chat_pb2_grpc.ChatServiceServicer):
    def __init__(self, chat: ChatService, *, require_ai_use: bool) -> None:
        self._chat = chat
        self._scopes = ai_use_scopes(require_ai_use)

    async def StreamChat(
        self,
        request: chat_pb2.StreamChatRequest,
        context: grpc.aio.ServicerContext,
    ) -> AsyncIterator[chat_pb2.StreamChatResponse]:
        await ensure_scopes(context, *self._scopes)
        if len(request.session_id) > MAX_SESSION_ID_LEN:
            await context.abort(
                grpc.StatusCode.INVALID_ARGUMENT,
                f"session_id must be at most {MAX_SESSION_ID_LEN} characters",
            )
        request_id = get_request_id()
        principal = current_principal()
        try:
            async for delta in self._chat.stream(
                request.message,
                session_id=request.session_id,
                principal_id=(
                    principal.id if principal and principal.type != "anonymous" else ""
                ),
                request_id=request_id,
                deadline_seconds=context.time_remaining(),
            ):
                yield chat_pb2.StreamChatResponse(delta=delta, done=False)
        except grpc.aio.AbortError:
            raise
        except Exception as exc:
            code, message = map_servicer_error(exc)
            if isinstance(exc, ChatQuotaExhausted) and exc.retry_after_seconds:
                context.set_trailing_metadata(
                    (("retry-after", str(exc.retry_after_seconds)),)
                )
            logger.opt(exception=exc).error(
                "grpc.StreamChat.error request_id={} type={} code={}",
                request_id,
                type(exc).__name__,
                code.name,
            )
            await context.abort(code, message)
            raise AssertionError("unreachable") from exc  # abort raises
        yield chat_pb2.StreamChatResponse(delta="", done=True)


def chat_service_registration(
    chat: ChatService, *, require_ai_use: bool
) -> GrpcServiceRegistration:
    servicer = ChatServicer(chat, require_ai_use=require_ai_use)
    return GrpcServiceRegistration(
        _SERVICE_NAME,
        lambda server: chat_pb2_grpc.add_ChatServiceServicer_to_server(
            servicer, server
        ),
        rate_limited_methods=_RATE_LIMITED_METHODS,
    )
