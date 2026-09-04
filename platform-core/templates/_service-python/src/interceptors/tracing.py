"""OpenTelemetry tracing setup + request-id propagation.

Two concerns, both per ADR-0004:

1. **W3C trace context** — the standard ``traceparent`` metadata key is handled
   for us by ``opentelemetry-instrumentation-grpc`` (the aio server interceptor
   extracts/injects it). We just wire up a TracerProvider + OTLP exporter.
2. **x-request-id** — a human-facing correlation id bridged from the product
   repo's HTTP convention. Read from inbound metadata or generated, then stashed
   in a contextvar so logs/spans can attach it.
"""

from __future__ import annotations

import uuid
from collections.abc import Awaitable, Callable
from contextvars import ContextVar

import grpc
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.grpc import aio_server_interceptor
from opentelemetry.sdk.resources import SERVICE_NAME, Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

from config import get_settings

REQUEST_ID_KEY = "x-request-id"

_request_id: ContextVar[str] = ContextVar("request_id", default="")


def request_id_from_context() -> str:
    """Return the correlation id for the in-flight RPC (empty if unset)."""
    return _request_id.get()


def setup_tracing() -> TracerProvider:
    """Install a global TracerProvider exporting OTLP to the configured endpoint.

    Idempotent-ish: call once at startup. Returns the provider so the server can
    hold a reference and shut it down gracefully.
    """
    settings = get_settings()
    resource = Resource.create({SERVICE_NAME: settings.OTEL_SERVICE_NAME})
    provider = TracerProvider(resource=resource)
    exporter = OTLPSpanExporter(endpoint=settings.OTEL_EXPORTER_OTLP_ENDPOINT)
    provider.add_span_processor(BatchSpanProcessor(exporter))
    trace.set_tracer_provider(provider)
    return provider


def otel_server_interceptor() -> grpc.aio.ServerInterceptor:
    """OTel aio server interceptor (handles W3C ``traceparent`` extraction)."""
    return aio_server_interceptor()


class RequestIdInterceptor(grpc.aio.ServerInterceptor):
    """Bridge ``x-request-id``: read inbound or mint one, expose via contextvar."""

    async def intercept_service(
        self,
        continuation: Callable[
            [grpc.HandlerCallDetails], Awaitable[grpc.RpcMethodHandler]
        ],
        handler_call_details: grpc.HandlerCallDetails,
    ) -> grpc.RpcMethodHandler:
        handler = await continuation(handler_call_details)
        if handler is None:
            return handler

        metadata = grpc.aio.Metadata.from_tuple(
            handler_call_details.invocation_metadata or ()
        )
        request_id = metadata.get(REQUEST_ID_KEY) or uuid.uuid4().hex

        return _bind_request_id(handler, request_id)


def _bind_request_id(
    handler: grpc.RpcMethodHandler, request_id: str
) -> grpc.RpcMethodHandler:
    """Wrap the handler so ``request_id`` is set on the contextvar during the call."""

    def _wrap(behavior: Callable[..., object]) -> Callable[..., object]:
        async def unary_response(request: object, context: object) -> object:
            token = _request_id.set(request_id)
            try:
                return await behavior(request, context)  # type: ignore[misc]
            finally:
                _request_id.reset(token)

        async def stream_response(request: object, context: object):
            token = _request_id.set(request_id)
            try:
                async for response in behavior(request, context):  # type: ignore[misc]
                    yield response
            finally:
                _request_id.reset(token)

        return stream_response if handler.response_streaming else unary_response

    if handler.unary_unary is not None:
        return grpc.unary_unary_rpc_method_handler(
            _wrap(handler.unary_unary),
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )
    if handler.unary_stream is not None:
        return grpc.unary_stream_rpc_method_handler(
            _wrap(handler.unary_stream),
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )
    if handler.stream_unary is not None:
        return grpc.stream_unary_rpc_method_handler(
            _wrap(handler.stream_unary),
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )
    if handler.stream_stream is not None:
        return grpc.stream_stream_rpc_method_handler(
            _wrap(handler.stream_stream),
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )
    return handler
