"""Anonymous callers are keyed by the gateway-forwarded client IP; empty ids are anonymous."""

from __future__ import annotations

import pytest

from app.modules.platform.identity.schemas import Principal
from app.transport.grpc.context import bind_client_ip, caller_key
from app.transport.grpc.interceptors.auth import _principal_from_metadata
from tests.factories import build_test_settings


def _anon() -> Principal:
    return Principal(id="anonymous", type="anonymous", scopes=())


def test_anonymous_callers_are_keyed_by_client_ip() -> None:
    bind_client_ip("203.0.113.7")
    assert caller_key(_anon()) == "anonymous:ip:203.0.113.7"
    bind_client_ip("198.51.100.9")
    assert caller_key(_anon()) == "anonymous:ip:198.51.100.9"


def test_anonymous_without_ip_shares_one_bucket_and_bad_ips_are_ignored() -> None:
    bind_client_ip("")
    assert caller_key(_anon()) == "anonymous:anonymous"
    bind_client_ip("1.2.3.4 evil")
    assert caller_key(_anon()) == "anonymous:anonymous"
    bind_client_ip("x" * 65)
    assert caller_key(_anon()) == "anonymous:anonymous"


def test_users_keep_their_principal_key_whatever_the_ip() -> None:
    bind_client_ip("203.0.113.7")
    user = Principal(id="u-1", type="user", scopes=())
    assert caller_key(user) == "user:u-1"


@pytest.mark.parametrize("ptype", ["user", "service"])
def test_an_empty_forwarded_id_is_anonymous(ptype: str) -> None:
    p = _principal_from_metadata({"x-principal-id": "", "x-principal-type": ptype})
    assert p is not None and p.type == "anonymous" and p.id == "anonymous"


def test_llm_router_outside_local_needs_rate_limit_and_quota() -> None:
    base: dict[str, object] = {
        "CHAT_BACKEND": "llm_router",
        "ENVIRONMENT": "production",
        "AUTH_BEARER_TOKEN": "x" * 32,
        "DOCS_ENABLED": False,
        "CORS_ALLOW_ORIGINS": "https://example.invalid",
        "TRUSTED_HOSTS": "example.invalid",
    }
    with pytest.raises(ValueError, match="QUOTA_ENABLED"):
        build_test_settings(**base, GRPC_RATE_LIMIT_ENABLED=False, QUOTA_ENABLED=False)
    with pytest.raises(ValueError, match="QUOTA_ENABLED"):
        build_test_settings(**base, GRPC_RATE_LIMIT_ENABLED=False, QUOTA_ENABLED=True)
    build_test_settings(CHAT_BACKEND="llm_router", ENVIRONMENT="local")
