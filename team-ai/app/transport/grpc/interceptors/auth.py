"""Auth interceptor: resolve a Principal from gRPC metadata.

Primary path: the gateway verifies the JWT once at the edge and forwards a trusted
principal as ``x-principal-{id,type,scopes}`` metadata; when present it is trusted.

A call without that metadata is UNAUTHENTICATED: there is no bearer fallback.
"""

from typing import Any

import grpc

from app.modules.platform.identity.schemas import Principal
from app.transport.grpc.context import (
    bind_client_ip,
    bind_principal,
    reset_client_ip,
    reset_principal,
)
from app.transport.grpc.interceptors._wrap import Cleanup, wrap_handler

_PRINCIPAL_TYPES = {"user", "service", "anonymous"}

# Infrastructure services that must answer unauthenticated: the gRPC health check
# (kubelet probes carry no token) and server reflection.
_AUTH_EXEMPT_PREFIXES = ("/grpc.health.v1.Health/", "/grpc.reflection.")


def _decode(value: Any) -> str:
    return value.decode() if isinstance(value, bytes) else (value or "")


def _principal_from_metadata(metadata: dict[str, Any]) -> Principal | None:
    """Build a Principal from ``x-principal-*`` metadata; None when absent."""
    if "x-principal-id" not in metadata:
        return None
    ptype = _decode(metadata.get("x-principal-type")) or "anonymous"
    pid = _decode(metadata.get("x-principal-id")).strip()
    if ptype not in _PRINCIPAL_TYPES or not pid:
        # fail closed: an unrecognised type or an empty id gets no identity
        ptype = "anonymous"
        pid = pid or "anonymous"
    scopes_raw = _decode(metadata.get("x-principal-scopes"))
    scopes = tuple(s.strip() for s in scopes_raw.split(",") if s.strip())
    return Principal(id=pid, type=ptype, scopes=scopes)  # type: ignore[arg-type]


class AuthInterceptor(grpc.aio.ServerInterceptor):
    async def intercept_service(
        self, continuation: Any, handler_call_details: Any
    ) -> Any:
        handler = await continuation(handler_call_details)
        if handler is None:
            return handler
        if (handler_call_details.method or "").startswith(_AUTH_EXEMPT_PREFIXES):
            return handler

        async def before(context: grpc.aio.ServicerContext) -> Cleanup:
            principal = await self._authenticate(context)
            metadata = dict(context.invocation_metadata() or ())
            principal_token = bind_principal(principal)
            ip_token = bind_client_ip(_decode(metadata.get("x-client-ip")))

            def cleanup() -> None:
                reset_client_ip(ip_token)
                reset_principal(principal_token)

            return cleanup

        return wrap_handler(handler, before)

    async def _authenticate(self, context: grpc.aio.ServicerContext) -> Principal:
        forwarded = _principal_from_metadata(dict(context.invocation_metadata() or ()))
        if forwarded is None:
            await context.abort(
                grpc.StatusCode.UNAUTHENTICATED, "Missing gateway principal"
            )
            raise AssertionError("unreachable")  # abort raises
        return forwarded
