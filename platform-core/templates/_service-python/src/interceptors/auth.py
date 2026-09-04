"""Authentication interceptor + Principal propagation.

Mirrors ``platform.common.v1.Principal`` (id / type / scopes) as a plain
dataclass so business code never imports generated protobuf types just to read
who is calling. The interceptor resolves a Principal from the ``authorization``
metadata and stashes it in a :class:`~contextvars.ContextVar`; handlers read it
via :func:`principal_from_context` and gate access with :func:`require_scopes`.

Auth today is a static bearer token (``settings.AUTH_BEARER_TOKEN``). TODO:
replace with JWT / cookie verification at the edge per ADR-0003 — services then
only ever receive an already-resolved Principal.
"""

from __future__ import annotations

import enum
import hmac
from collections.abc import Awaitable, Callable
from contextvars import ContextVar
from dataclasses import dataclass, field

import grpc

from config import get_settings


class PrincipalType(enum.IntEnum):
    """Language-local mirror of ``platform.common.v1.PrincipalType``."""

    UNSPECIFIED = 0
    ANONYMOUS = 1
    USER = 2
    SERVICE = 3


@dataclass(frozen=True, slots=True)
class Principal:
    """Resolved caller identity (mirror of ``platform.common.v1.Principal``)."""

    id: str
    type: PrincipalType = PrincipalType.UNSPECIFIED
    scopes: tuple[str, ...] = field(default_factory=tuple)


ANONYMOUS = Principal(id="anonymous", type=PrincipalType.ANONYMOUS, scopes=())

_principal: ContextVar[Principal] = ContextVar("principal", default=ANONYMOUS)


def principal_from_context() -> Principal:
    """Return the Principal resolved for the in-flight RPC (or ANONYMOUS)."""
    return _principal.get()


class InsufficientScopeError(grpc.RpcError):
    """Raised by :func:`require_scopes`; carries the gRPC status + details.

    A ``grpc.RpcError`` subclass so it satisfies ``code()`` / ``details()`` like
    any transport error. A servicer may let it propagate (the server maps it to
    a status) or convert it via ``await context.abort(err.code(), err.details())``.
    """

    def __init__(self, missing: list[str]) -> None:
        self.missing = missing
        self._details = f"insufficient_scope: missing {missing}"
        super().__init__(self._details)

    def code(self) -> grpc.StatusCode:
        return grpc.StatusCode.PERMISSION_DENIED

    def details(self) -> str:
        return self._details


def require_scopes(*scopes: str) -> None:
    """Abort the current RPC unless the caller holds every listed scope.

    Raises :class:`InsufficientScopeError` (``PERMISSION_DENIED`` / details
    ``"insufficient_scope"`` — the stable domain code from ``platform.common.v1.Error``).
    Call from inside a servicer method::

        async def SearchListings(self, request, context):
            require_scopes("search:read")
            ...
    """
    principal = principal_from_context()
    missing = sorted(set(scopes) - set(principal.scopes))
    if missing:
        raise InsufficientScopeError(missing)


def _resolve_principal(metadata: grpc.aio.Metadata | None) -> Principal:
    """Build a Principal from ``authorization: bearer <token>`` metadata.

    Returns ANONYMOUS when no valid credential is present — per-RPC scope checks
    (``require_scopes``) do the actual gating, so unauthenticated calls to open
    endpoints (e.g. health) still work.
    """
    settings = get_settings()
    if not settings.AUTH_BEARER_TOKEN or metadata is None:
        return ANONYMOUS

    raw = metadata.get("authorization")
    if not raw:
        return ANONYMOUS

    scheme, _, token = raw.partition(" ")
    if scheme.lower() != "bearer" or not token:
        return ANONYMOUS

    # Constant-time compare — avoids leaking the token via timing.
    if not hmac.compare_digest(token, settings.AUTH_BEARER_TOKEN):
        return ANONYMOUS

    # TODO(ADR-0003): derive id / scopes from verified JWT claims. Seed grants a
    # single service identity with a broad scope so require_scopes() is testable.
    return Principal(
        id="service",
        type=PrincipalType.SERVICE,
        scopes=("search:read", "chat:read"),
    )


class AuthInterceptor(grpc.aio.ServerInterceptor):
    """Resolve a Principal per RPC and expose it through the contextvar."""

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
        principal = _resolve_principal(metadata)

        # Wrap the actual RPC behaviour so the contextvar is set inside the same
        # task that runs the handler (setting it here in intercept_service would
        # not survive to the deferred handler invocation).
        return _wrap_handler(handler, principal)


def _wrap_handler(
    handler: grpc.RpcMethodHandler, principal: Principal
) -> grpc.RpcMethodHandler:
    """Return a handler that binds ``principal`` for the duration of the call."""

    def _bind(behavior: Callable[..., object]) -> Callable[..., object]:
        async def unary_response(request: object, context: object) -> object:
            token = _principal.set(principal)
            try:
                return await behavior(request, context)  # type: ignore[misc]
            finally:
                _principal.reset(token)

        async def stream_response(request: object, context: object):
            token = _principal.set(principal)
            try:
                async for response in behavior(request, context):  # type: ignore[misc]
                    yield response
            finally:
                _principal.reset(token)

        return stream_response if handler.response_streaming else unary_response

    if handler.unary_unary is not None:
        return grpc.unary_unary_rpc_method_handler(
            _bind(handler.unary_unary),
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )
    if handler.unary_stream is not None:
        return grpc.unary_stream_rpc_method_handler(
            _bind(handler.unary_stream),
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )
    if handler.stream_unary is not None:
        return grpc.stream_unary_rpc_method_handler(
            _bind(handler.stream_unary),
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )
    if handler.stream_stream is not None:
        return grpc.stream_stream_rpc_method_handler(
            _bind(handler.stream_stream),
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )
    return handler
