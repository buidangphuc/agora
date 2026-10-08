"""Error mapping for the AIService servicer: nothing internal reaches the caller.

``map_servicer_error`` turns an exception into a (status, message) pair whose
message is either built from field NAMES only (validation), or a fixed string.
``str(exc)`` is never forwarded: for a pydantic ``ValidationError`` it includes
``input_value`` (user text), and for arbitrary failures it can carry stack
details, paths or secrets. The original exception is logged server-side with
the request id.
"""

from __future__ import annotations

import functools
import re
from collections.abc import Awaitable, Callable
from typing import Any, TypeVar

import grpc
from loguru import logger
from pydantic import ValidationError

from app.core.errors import ServiceUnavailableError
from app.core.request_context import get_request_id
from app.core.resilience import FailureKind
from app.transport.grpc.chat_stream import (
    ChainExhausted,
    ChatDeadlineExceeded,
    ChatStreamError,
    StreamInterrupted,
)

INTERNAL_MESSAGE = "internal error"
UNAVAILABLE_MESSAGE = "service unavailable"
# StreamChat (LLM path): fixed strings, never provider or exception text.
MODEL_UNAVAILABLE_MESSAGE = "model unavailable"
MODEL_RATE_LIMITED_MESSAGE = "model rate limited"
DEADLINE_EXCEEDED_MESSAGE = "deadline exceeded"
QUOTA_EXCEEDED_MESSAGE = "quota exceeded"

# A pydantic ``loc`` part normally comes from our own schema; a dict-typed field
# would put client-chosen keys there, so only a conservative charset is echoed.
_SAFE_LOC = re.compile(r"[^A-Za-z0-9_.\-\[\]]")


def _field_names(exc: ValidationError) -> list[str]:
    names: list[str] = []
    for err in exc.errors(
        include_input=False, include_url=False, include_context=False
    ):
        name = ".".join(_SAFE_LOC.sub("", str(part)) for part in err.get("loc", ()))
        if name and name not in names:
            names.append(name)
    return names


def map_servicer_error(exc: BaseException) -> tuple[grpc.StatusCode, str]:
    """Map an exception to a gRPC status and a caller-safe message."""
    if isinstance(exc, ValidationError):
        names = _field_names(exc)
        detail = ", ".join(names) if names else "request"
        return grpc.StatusCode.INVALID_ARGUMENT, f"invalid request: {detail}"
    if isinstance(exc, ServiceUnavailableError):
        return grpc.StatusCode.UNAVAILABLE, UNAVAILABLE_MESSAGE
    if isinstance(exc, ChatStreamError):
        return map_chat_error(exc)
    return grpc.StatusCode.INTERNAL, INTERNAL_MESSAGE


def map_chat_error(exc: ChatStreamError) -> tuple[grpc.StatusCode, str]:
    """StreamChat failures onto existing gRPC codes with fixed messages."""
    if isinstance(exc, ChatDeadlineExceeded):
        return grpc.StatusCode.DEADLINE_EXCEEDED, DEADLINE_EXCEEDED_MESSAGE
    if isinstance(exc, ChainExhausted) and exc.last_kind is FailureKind.RATE_LIMITED:
        return grpc.StatusCode.RESOURCE_EXHAUSTED, MODEL_RATE_LIMITED_MESSAGE
    if isinstance(exc, ChainExhausted | StreamInterrupted):
        return grpc.StatusCode.UNAVAILABLE, MODEL_UNAVAILABLE_MESSAGE
    return grpc.StatusCode.INTERNAL, INTERNAL_MESSAGE


F = TypeVar("F", bound=Callable[..., Awaitable[Any]])


def map_errors(rpc: str) -> Callable[[F], F]:
    """Decorator for ``AIService`` handler bodies (``self, request, context``).

    ``grpc.aio.AbortError`` (raised by ``context.abort``, for instance by a scope
    check) is re-raised untouched. Scope checks run BEFORE this wrapper, outside
    it, so an abort is never re-mapped.
    """

    def decorate(fn: F) -> F:
        @functools.wraps(fn)
        async def wrapper(self: Any, request: Any, context: grpc.aio.ServicerContext):
            try:
                return await fn(self, request, context)
            except grpc.aio.AbortError:
                raise
            except Exception as exc:
                code, message = map_servicer_error(exc)
                rid = get_request_id()
                if isinstance(exc, ValidationError):
                    # A client error: log field names only (the input may be user text).
                    logger.warning(
                        "grpc.{}.invalid_request request_id={} message={}",
                        rpc,
                        rid,
                        message,
                    )
                else:
                    logger.opt(exception=exc).error(
                        "grpc.{}.error request_id={} type={}",
                        rpc,
                        rid,
                        type(exc).__name__,
                    )
                await context.abort(code, message)
                raise AssertionError("unreachable") from exc  # abort raises

        return wrapper  # type: ignore[return-value]

    return decorate
