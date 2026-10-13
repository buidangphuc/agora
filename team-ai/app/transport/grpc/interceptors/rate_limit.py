"""Rate-limit interceptor: per-caller throttle on a registered set of methods.

Runs after ``AuthInterceptor`` so the principal is already bound. Over the limit =>
``RESOURCE_EXHAUSTED`` with a ``retry-after`` trailer. A limiter backend failure fails
open (logged): a Redis blip must not take the service down.
"""

from typing import Any, Protocol

import grpc
from loguru import logger

from app.modules.platform.rate_limit.service import RateLimitResult
from app.transport.grpc.context import caller_key, current_principal
from app.transport.grpc.interceptors._wrap import Cleanup, wrap_handler


class RateLimiter(Protocol):
    async def check(self, key: str) -> RateLimitResult: ...


class RateLimitInterceptor(grpc.aio.ServerInterceptor):
    def __init__(self, limiter: RateLimiter, *, methods: frozenset[str]) -> None:
        self._limiter = limiter
        self._methods = methods

    async def intercept_service(
        self, continuation: Any, handler_call_details: Any
    ) -> Any:
        handler = await continuation(handler_call_details)
        if handler is None or (handler_call_details.method or "") not in self._methods:
            return handler

        async def before(context: grpc.aio.ServicerContext) -> Cleanup | None:
            principal = current_principal()
            if principal is None:  # auth runs first and always binds one
                return None
            try:
                result = await self._limiter.check(caller_key(principal))
            except Exception as exc:
                logger.warning("grpc.rate_limit.error error={}", type(exc).__name__)
                return None
            if not result.allowed:
                retry_after = str(result.retry_after_seconds or 1)
                context.set_trailing_metadata((("retry-after", retry_after),))
                await context.abort(
                    grpc.StatusCode.RESOURCE_EXHAUSTED, "rate limit exceeded"
                )
            return None

        return wrap_handler(handler, before)
