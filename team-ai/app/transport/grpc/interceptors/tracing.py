"""Tracing interceptor: bridge the ``x-request-id`` correlation id across the hop.

Binds ``x-request-id`` from incoming metadata (or a minted ``req_<uuid4 hex>``) into the
shared request context, the same convention as the HTTP ``RequestIdMiddleware``, and
counts every RPC (health checks included) for the autoscaling metric.
"""

import uuid
from typing import Any

import grpc

from app.core import grpc_metrics
from app.core.request_context import reset_request_id, set_request_id
from app.transport.grpc.interceptors._wrap import Cleanup, wrap_handler


class TracingInterceptor(grpc.aio.ServerInterceptor):
    async def intercept_service(
        self, continuation: Any, handler_call_details: Any
    ) -> Any:
        grpc_metrics.hit()
        handler = await continuation(handler_call_details)
        if handler is None:
            return handler

        async def before(context: grpc.aio.ServicerContext) -> Cleanup:
            raw = dict(context.invocation_metadata() or ()).get("x-request-id")
            incoming = raw.decode() if isinstance(raw, bytes) else raw
            token = set_request_id(incoming or f"req_{uuid.uuid4().hex}")
            return lambda: reset_request_id(token)

        return wrap_handler(handler, before)
