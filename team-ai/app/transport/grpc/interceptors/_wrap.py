"""Rebuild a gRPC method handler so a preamble runs before the original behavior.

grpc.aio ServerInterceptors receive an ``RpcMethodHandler`` and must return one of
the same shape, preserving the (de)serializers. ``before`` is a coroutine that may
abort and may return a cleanup callable, run in ``finally`` after the call (used to
reset contextvars bound by the preamble). Unary-unary and unary-stream are wrapped;
other shapes pass through.
"""

from collections.abc import AsyncIterator, Awaitable, Callable
from typing import Any, cast

import grpc

Cleanup = Callable[[], None]
Before = Callable[[grpc.aio.ServicerContext], Awaitable[Cleanup | None]]
# grpc-stubs types handler callables with the sync ServicerContext; grpc.aio hands
# them the aio one, so the wrapped behaviors are re-typed to what they really get.
_UnaryUnary = Callable[[Any, grpc.aio.ServicerContext], Awaitable[Any]]
_UnaryStream = Callable[[Any, grpc.aio.ServicerContext], AsyncIterator[Any]]


def wrap_handler(
    handler: grpc.RpcMethodHandler, before: Before
) -> grpc.RpcMethodHandler:
    if handler.unary_unary is not None:
        inner = cast("_UnaryUnary", handler.unary_unary)

        async def unary_unary(request: Any, context: grpc.aio.ServicerContext) -> Any:
            cleanup = await before(context)
            try:
                return await inner(request, context)
            finally:
                if cleanup is not None:
                    cleanup()

        return grpc.unary_unary_rpc_method_handler(
            unary_unary,
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )

    if handler.unary_stream is not None:
        inner_stream = cast("_UnaryStream", handler.unary_stream)

        async def unary_stream(
            request: Any, context: grpc.aio.ServicerContext
        ) -> AsyncIterator[Any]:
            cleanup = await before(context)
            try:
                async for response in inner_stream(request, context):
                    yield response
            finally:
                if cleanup is not None:
                    cleanup()

        return grpc.unary_stream_rpc_method_handler(
            unary_stream,
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )

    return handler
