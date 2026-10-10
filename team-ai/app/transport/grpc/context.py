"""Per-call gRPC context: the authenticated Principal and scope enforcement.

The auth interceptor resolves a ``Principal`` from request metadata and binds it
here for the duration of the call; servicers read it via ``current_principal`` and
gate RPCs with ``ensure_scopes``.
"""

from __future__ import annotations

from contextvars import ContextVar, Token

import grpc

from app.modules.platform.identity.schemas import Principal

_principal: ContextVar[Principal | None] = ContextVar("grpc_principal", default=None)
# The client IP team-gateway observed (``x-client-ip``), only ever set from gateway
# metadata. It keys anonymous callers so one visitor cannot exhaust everyone's bucket.
_client_ip: ContextVar[str] = ContextVar("grpc_client_ip", default="")

_MAX_CLIENT_IP_LEN = 64


def current_principal() -> Principal | None:
    return _principal.get()


def bind_principal(principal: Principal) -> Token[Principal | None]:
    return _principal.set(principal)


def reset_principal(token: Token[Principal | None]) -> None:
    _principal.reset(token)


def bind_client_ip(value: str) -> Token[str]:
    ip = value.strip()
    if len(ip) > _MAX_CLIENT_IP_LEN or any(c.isspace() for c in ip):
        ip = ""
    return _client_ip.set(ip)


def current_client_ip() -> str:
    return _client_ip.get()


def caller_key(principal: Principal) -> str:
    """Rate-limit key of a call: the principal, or the client IP for anonymous callers.

    Anonymous callers without a forwarded IP share one bucket (the previous behaviour).
    """
    if principal.type == "anonymous":
        ip = current_client_ip()
        return f"anonymous:ip:{ip}" if ip else "anonymous:anonymous"
    return f"{principal.type}:{principal.id}"


async def ensure_scopes(context: grpc.aio.ServicerContext, *required: str) -> Principal:
    """Abort PERMISSION_DENIED unless the call's Principal holds every scope."""
    principal = current_principal()
    if principal is None:
        await context.abort(grpc.StatusCode.UNAUTHENTICATED, "no principal on call")
        raise AssertionError("unreachable")  # abort raises; satisfies type checkers
    missing = sorted(frozenset(required) - set(principal.scopes))
    if missing:
        await context.abort(
            grpc.StatusCode.PERMISSION_DENIED,
            f"insufficient_scope: missing {missing}",
        )
        raise AssertionError("unreachable")
    return principal
