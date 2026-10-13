"""Caller-safe error mapping for servicer handlers: nothing internal reaches the caller.

Messages are built from field NAMES only (validation) or are fixed strings.
``str(exc)`` is never forwarded: for a pydantic ``ValidationError`` it includes
``input_value`` (user text), and for arbitrary failures it can carry paths or secrets.
The original exception is logged server-side with the request id.
"""

import functools
import re
from collections.abc import Awaitable, Callable
from typing import Any, TypeVar

import grpc
from loguru import logger
from pydantic import ValidationError

from app.core.errors import ServiceUnavailableError
from app.core.request_context import get_request_id
from app.modules.business.chat.errors import (
    ChatDeadlineExceeded,
    ChatInterrupted,
    ChatModelRateLimited,
    ChatModelUnavailable,
    ChatQuotaExhausted,
)

INTERNAL_MESSAGE = "internal error"
UNAVAILABLE_MESSAGE = "service unavailable"
# StreamChat: fixed strings, never provider or exception text.
MODEL_UNAVAILABLE_MESSAGE = "model unavailable"
MODEL_RATE_LIMITED_MESSAGE = "model rate limited"
DEADLINE_EXCEEDED_MESSAGE = "deadline exceeded"
QUOTA_EXCEEDED_MESSAGE = "quota exceeded"

# A pydantic ``loc`` part normally comes from our own schema, but a dict-typed field
# puts client-chosen keys there, so only a conservative charset is echoed.
_UNSAFE_LOC = re.compile(r"[^A-Za-z0-9_.\-\[\]]")


def _field_names(exc: ValidationError) -> list[str]:
    names: list[str] = []
    for err in exc.errors(
        include_input=False, include_url=False, include_context=False
    ):
        name = ".".join(_UNSAFE_LOC.sub("", str(part)) for part in err.get("loc", ()))
        if name and name not in names:
            names.append(name)
    return names


def map_servicer_error(exc: BaseException) -> tuple[grpc.StatusCode, str]:
    """Map an exception to a gRPC status and a caller-safe message."""
    if isinstance(exc, ValidationError):
        detail = ", ".join(_field_names(exc)) or "request"
        return grpc.StatusCode.INVALID_ARGUMENT, f"invalid request: {detail}"
    if isinstance(exc, ServiceUnavailableError):
        return grpc.StatusCode.UNAVAILABLE, UNAVAILABLE_MESSAGE
    if isinstance(exc, ChatDeadlineExceeded):
        return grpc.StatusCode.DEADLINE_EXCEEDED, DEADLINE_EXCEEDED_MESSAGE
    if isinstance(exc, ChatQuotaExhausted):
        return grpc.StatusCode.RESOURCE_EXHAUSTED, QUOTA_EXCEEDED_MESSAGE
    if isinstance(exc, ChatModelRateLimited):
        return grpc.StatusCode.RESOURCE_EXHAUSTED, MODEL_RATE_LIMITED_MESSAGE
    if isinstance(exc, ChatModelUnavailable | ChatInterrupted):
        return grpc.StatusCode.UNAVAILABLE, MODEL_UNAVAILABLE_MESSAGE
    return grpc.StatusCode.INTERNAL, INTERNAL_MESSAGE


F = TypeVar("F", bound=Callable[..., Awaitable[Any]])


def map_errors(rpc: str) -> Callable[[F], F]:
    """Decorator for servicer handler bodies (``self, request, context``).

    ``grpc.aio.AbortError`` (raised by ``context.abort``, e.g. a scope check) is
    re-raised untouched, so an abort is never re-mapped.
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
                request_id = get_request_id()
                if isinstance(exc, ValidationError):
                    # Client error: log the safe message only (the input may be user text).
                    logger.warning(
                        "grpc.{}.invalid_request request_id={} message={}",
                        rpc,
                        request_id,
                        message,
                    )
                else:
                    logger.opt(exception=exc).error(
                        "grpc.{}.error request_id={} type={}",
                        rpc,
                        request_id,
                        type(exc).__name__,
                    )
                await context.abort(code, message)
                raise AssertionError("unreachable") from exc  # abort raises

        return wrapper  # type: ignore[return-value]

    return decorate
