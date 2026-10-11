import hmac
from collections.abc import Callable, Coroutine
from typing import Any

from fastapi import Depends, Request
from fastapi.security.utils import get_authorization_scheme_param

from app.bootstrap.state import get_app_resources, get_app_settings
from app.core.config import Settings
from app.core.errors import ForbiddenError, RateLimitError, UnauthorizedError
from app.modules.platform.identity.schemas import Principal

ADMIN_SCOPE = "admin"
AI_CLASSIFY_SCOPE = "ai.classify"


async def authenticate_bearer_token(
    authorization: str | None,
    *,
    settings: Settings,
    allow_admin_token: bool = False,
) -> Principal:
    """Resolve a static bearer token to a service principal.

    ``allow_admin_token`` additionally accepts ``AUTH_ADMIN_BEARER_TOKEN`` (the tag
    taxonomy routes only), which resolves to ``admin`` + ``ai.classify``.
    """
    admin_token = settings.AUTH_ADMIN_BEARER_TOKEN if allow_admin_token else ""
    if not settings.AUTH_BEARER_TOKEN and not admin_token:
        raise ForbiddenError(
            "Bearer token authentication is not configured",
            code="auth_not_configured",
        )

    scheme, token = get_authorization_scheme_param(authorization)
    if not authorization or scheme.lower() != "bearer":
        raise UnauthorizedError("Missing bearer token")

    if admin_token and hmac.compare_digest(token, admin_token):
        return Principal(
            id=settings.AUTH_ADMIN_SUBJECT,
            type="service",
            scopes=(ADMIN_SCOPE, AI_CLASSIFY_SCOPE),
        )
    if not settings.AUTH_BEARER_TOKEN or not hmac.compare_digest(
        token, settings.AUTH_BEARER_TOKEN
    ):
        raise UnauthorizedError("Invalid bearer token")

    return Principal(
        id=settings.AUTH_SUBJECT,
        type="service",
        scopes=tuple(settings.auth_roles),
    )


async def _enforce_rate_limit(request: Request, principal: Principal) -> None:
    rate_limiter = get_app_resources(request.app).principal_rate_limiter
    if rate_limiter is None:
        return
    result = await rate_limiter.check(principal.id)
    if not result.allowed:
        raise RateLimitError(retry_after_seconds=result.retry_after_seconds)


async def require_principal(request: Request) -> Principal:
    principal = await authenticate_bearer_token(
        request.headers.get("Authorization"),
        settings=get_app_settings(request.app),
    )
    request.state.principal = principal
    await _enforce_rate_limit(request, principal)
    return principal


def require_scopes(
    *required: str,
) -> Callable[[Principal], Coroutine[Any, Any, Principal]]:
    """Build a dependency that enforces ``principal.scopes`` ⊇ ``required``.

    Usage::

        @router.post(
            "/admin/users",
            dependencies=[Depends(require_scopes("admin"))],
        )
        async def create_user(...): ...
    """
    required_set = frozenset(required)

    async def dependency(
        principal: Principal = Depends(require_principal),
    ) -> Principal:
        missing = sorted(required_set - set(principal.scopes))
        if missing:
            raise ForbiddenError(
                f"Missing required scopes: {missing}",
                code="insufficient_scope",
                data={"required": sorted(required_set), "missing": missing},
            )
        return principal

    return dependency


def require_tag_access(
    *accepted: str,
) -> Callable[[Request], Coroutine[Any, Any, Principal]]:
    """Dependency for the tag taxonomy routes: a valid service or admin token whose
    principal holds at least one of ``accepted`` scopes (401 no/invalid token, 403 scope)."""
    accepted_set = frozenset(accepted)

    async def dependency(request: Request) -> Principal:
        principal = await authenticate_bearer_token(
            request.headers.get("Authorization"),
            settings=get_app_settings(request.app),
            allow_admin_token=True,
        )
        if not accepted_set & set(principal.scopes):
            raise ForbiddenError(
                f"Missing required scope: one of {sorted(accepted_set)}",
                code="insufficient_scope",
                data={"required_any": sorted(accepted_set)},
            )
        request.state.principal = principal
        await _enforce_rate_limit(request, principal)
        return principal

    return dependency
