"""Auth interceptor: resolve a Principal from gRPC metadata.

Primary path (platform ADR-0003): team-gateway verifies the JWT once at the edge
and forwards a trusted principal as ``x-principal-{id,type,scopes}`` metadata,
rebuilt on every hop. When those headers are present we trust them — this is how
every other platform service authenticates gateway traffic.

Fallback: a direct caller (tests, local tooling) may still present an
``authorization: bearer`` token, resolved via the transport-neutral
``authenticate_bearer_token`` so both surfaces issue the same Principal.
"""

from __future__ import annotations

from contextvars import Token
from typing import Any

import grpc

from app.core.config import Settings
from app.core.errors import ForbiddenError, UnauthorizedError
from app.modules.platform.identity.auth import authenticate_bearer_token
from app.modules.platform.identity.schemas import Principal
from app.transport.grpc.context import _principal
from app.transport.grpc.interceptors._wrap import wrap_handler


_PRINCIPAL_TYPES = {"user", "service", "anonymous"}

# Infrastructure services that must answer unauthenticated: the gRPC health check
# (kubelet liveness/readiness probes carry no token) and server reflection.
_AUTH_EXEMPT_PREFIXES = ("/grpc.health.v1.Health/", "/grpc.reflection.")


def _decode(value: Any) -> str:
    return value.decode() if isinstance(value, bytes) else (value or "")


def _principal_from_metadata(metadata: dict[str, Any]) -> Principal | None:
    """Build a Principal from gateway-forwarded ``x-principal-*`` metadata.

    Returns None when the gateway headers are absent so the bearer fallback runs.
    """
    if "x-principal-id" not in metadata:
        return None
    ptype = _decode(metadata.get("x-principal-type")) or "anonymous"
    if ptype not in _PRINCIPAL_TYPES:
        ptype = "user"
    scopes_raw = _decode(metadata.get("x-principal-scopes"))
    scopes = tuple(s.strip() for s in scopes_raw.split(",") if s.strip())
    return Principal(
        id=_decode(metadata.get("x-principal-id")),
        type=ptype,  # type: ignore[arg-type]
        scopes=scopes,
    )


class AuthInterceptor(grpc.aio.ServerInterceptor):
    def __init__(self, settings: Settings) -> None:
        self._settings = settings

    async def intercept_service(
        self,
        continuation: Any,
        handler_call_details: Any,
    ) -> Any:
        handler = await continuation(handler_call_details)
        if handler is None:
            return handler

        method = handler_call_details.method or ""
        if method.startswith(_AUTH_EXEMPT_PREFIXES):
            return handler

        async def before(context: grpc.aio.ServicerContext) -> Token[Principal | None]:
            principal = await self._authenticate(context)
            return _principal.set(principal)

        return wrap_handler(handler, before)

    async def _authenticate(self, context: grpc.aio.ServicerContext) -> Principal:
        metadata = dict(context.invocation_metadata() or ())

        # ADR-0003: trust the principal team-gateway forwards after verifying the
        # JWT once at the edge.
        forwarded = _principal_from_metadata(metadata)
        if forwarded is not None:
            return forwarded

        raw = metadata.get("authorization")
        authorization = raw.decode() if isinstance(raw, bytes) else raw
        try:
            return await authenticate_bearer_token(
                authorization, settings=self._settings
            )
        except UnauthorizedError as exc:
            await context.abort(grpc.StatusCode.UNAUTHENTICATED, exc.message)
        except ForbiddenError as exc:
            await context.abort(grpc.StatusCode.PERMISSION_DENIED, exc.message)
        raise AssertionError("unreachable")  # abort raises
