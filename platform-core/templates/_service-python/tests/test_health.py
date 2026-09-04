"""Seed tests — must pass WITHOUT generated protobuf code present.

Covers the pure helpers only: Principal scope checking and config loading. The
gRPC surface (health/reflection/search) is exercised by integration tests once
`make proto` has produced the stubs.
"""

from __future__ import annotations

import pytest

from config import Settings, get_settings
from interceptors.auth import (
    ANONYMOUS,
    InsufficientScopeError,
    Principal,
    PrincipalType,
    _principal,
    principal_from_context,
    require_scopes,
)


def test_config_defaults() -> None:
    # Construct without reading a developer's local .env so defaults are stable.
    settings = Settings(_env_file=None)  # type: ignore[call-arg]
    assert settings.GRPC_PORT == 50052
    assert settings.QDRANT_URL == "http://localhost:6333"
    assert settings.OTEL_SERVICE_NAME == "team-service"
    assert settings.DATABASE_URL is None


def test_get_settings_is_cached() -> None:
    assert get_settings() is get_settings()


def test_principal_defaults_to_anonymous() -> None:
    assert principal_from_context() is ANONYMOUS
    assert ANONYMOUS.type is PrincipalType.ANONYMOUS


def test_require_scopes_passes_when_scopes_present() -> None:
    principal = Principal(
        id="svc", type=PrincipalType.SERVICE, scopes=("search:read", "chat:read")
    )
    token = _principal.set(principal)
    try:
        # Does not raise when the caller holds every required scope.
        require_scopes("search:read")
        require_scopes("search:read", "chat:read")
    finally:
        _principal.reset(token)


def test_require_scopes_denies_when_scope_missing() -> None:
    principal = Principal(id="svc", type=PrincipalType.SERVICE, scopes=("search:read",))
    token = _principal.set(principal)
    try:
        with pytest.raises(InsufficientScopeError) as excinfo:
            require_scopes("search:read", "admin")
    finally:
        _principal.reset(token)

    err = excinfo.value
    assert err.missing == ["admin"]
    assert "insufficient_scope" in err.details()
    assert err.code().name == "PERMISSION_DENIED"


def test_require_scopes_denies_anonymous() -> None:
    with pytest.raises(InsufficientScopeError):
        require_scopes("search:read")
