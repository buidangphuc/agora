"""Tag taxonomy REST routes require a verified bearer principal (change tag-routes-authz)."""

from __future__ import annotations

import pytest
from httpx import ASGITransport, AsyncClient

from app.bootstrap.application import create_app
from app.core.errors import ForbiddenError, UnauthorizedError
from app.modules.platform.identity.auth import authenticate_bearer_token
from tests.factories import build_test_settings

SERVICE = "svc-token"  # pragma: allowlist secret
ADMIN = "admin-token"  # pragma: allowlist secret
TAGS = "/api/v1/ai/tags"
CLASSIFY = {"title": "Sac nhanh GaN 65W", "category_id": "cat-electronics"}
EXPLORE = {
    "batch_listings": [
        {"listing_id": "a", "title": "Sac nhanh cong suat 77W", "category_id": "c"},
        {"listing_id": "b", "title": "Cu sac cong suat 77W", "category_id": "c"},
    ],
    "min_frequency": 2,
    "min_confidence": 0.8,
}
PROMOTE = {"tag_slugs": ["cong-suat-77w"], "target_category_id": "c"}

ROUTES = [
    ("post", "/classify", CLASSIFY, "read"),
    (
        "post",
        "/classify-sku-hierarchy",
        {
            "spu_title": "iPhone 15",
            "category_id": "c",
            "variants": [
                {
                    "variant_id": "v1",
                    "name": "Xanh / 256GB",
                    "sku_code": "S1",
                    "price": 1,
                    "stock": 1,
                    "options": {"color": "Xanh"},
                }
            ],
        },
        "read",
    ),
    ("get", "", None, "read"),
    ("post", "/explore", EXPLORE, "admin"),
    ("post", "/promote", PROMOTE, "admin"),
]


def _client(app) -> AsyncClient:
    return AsyncClient(transport=ASGITransport(app=app), base_url="http://test")


@pytest.fixture
def app():
    settings = build_test_settings(
        AUTH_BEARER_TOKEN=SERVICE,
        AUTH_ROLES="ai.classify",
        AUTH_ADMIN_BEARER_TOKEN=ADMIN,
    )
    application = create_app(settings=settings, init_resources=False)
    return application


def _send(client: AsyncClient, method: str, path: str, body, token: str | None):
    headers = {"Authorization": f"Bearer {token}"} if token else {}
    return client.request(method, TAGS + path, json=body, headers=headers)


@pytest.mark.parametrize(("method", "path", "body", "_need"), ROUTES)
async def test_anonymous_and_wrong_tokens_get_401(app, method, path, body, _need):
    async with _client(app) as client:
        for token in (
            None,
            "wrong",
            "eyJhbGciOiJSUzI1NiJ9.e30.sig",
        ):  # a JWT is just wrong
            resp = await _send(client, method, path, body, token)
            assert resp.status_code == 401, (path, token, resp.text)


@pytest.mark.parametrize(("method", "path", "body", "need"), ROUTES)
async def test_service_token_reads_but_cannot_mutate(app, method, path, body, need):
    async with _client(app) as client:
        resp = await _send(client, method, path, body, SERVICE)
    assert resp.status_code == (200 if need == "read" else 403), resp.text


@pytest.mark.parametrize(("method", "path", "body", "_need"), ROUTES)
async def test_admin_token_may_call_every_route(app, method, path, body, _need):
    async with _client(app) as client:
        resp = await _send(client, method, path, body, ADMIN)
    assert resp.status_code == 200, resp.text


async def test_refused_calls_leave_the_taxonomy_unchanged(app):
    async with _client(app) as client:
        before = (await _send(client, "get", "", None, SERVICE)).json()["tags"]
        for token in (None, SERVICE):
            await _send(client, "post", "/explore", EXPLORE, token)
            await _send(client, "post", "/promote", PROMOTE, token)
        after = (await _send(client, "get", "", None, SERVICE)).json()["tags"]
    assert before == after


async def test_service_token_without_the_scope_is_403():
    settings = build_test_settings(AUTH_BEARER_TOKEN=SERVICE, AUTH_ROLES="")
    async with _client(create_app(settings=settings, init_resources=False)) as client:
        resp = await _send(client, "post", "/classify", CLASSIFY, SERVICE)
    assert resp.status_code == 403


async def test_admin_token_unset_grants_no_admin():
    settings = build_test_settings(AUTH_BEARER_TOKEN=SERVICE, AUTH_ROLES="ai.classify")
    async with _client(create_app(settings=settings, init_resources=False)) as client:
        for token in (SERVICE, ADMIN, ""):
            resp = await _send(client, "post", "/promote", PROMOTE, token or None)
            assert resp.status_code in (401, 403), resp.text
    with pytest.raises(UnauthorizedError):
        await authenticate_bearer_token(
            f"Bearer {ADMIN}", settings=settings, allow_admin_token=True
        )


async def test_admin_token_is_not_a_general_credential():
    settings = build_test_settings(
        AUTH_BEARER_TOKEN=SERVICE, AUTH_ADMIN_BEARER_TOKEN=ADMIN
    )
    # require_principal (completions, gRPC bearer fallback) does not honour it
    with pytest.raises(UnauthorizedError):
        await authenticate_bearer_token(f"Bearer {ADMIN}", settings=settings)
    principal = await authenticate_bearer_token(
        f"Bearer {ADMIN}", settings=settings, allow_admin_token=True
    )
    assert set(principal.scopes) == {"admin", "ai.classify"}


async def test_nothing_configured_is_refused():
    settings = build_test_settings(AUTH_BEARER_TOKEN="")
    with pytest.raises(ForbiddenError):
        await authenticate_bearer_token(
            "Bearer x", settings=settings, allow_admin_token=True
        )


def test_weak_or_shared_admin_token_is_refused_outside_local():
    strong = "a-strong-admin-token-of-24-plus-chars"  # pragma: allowlist secret
    service = "another-strong-service-token-24-chars"  # pragma: allowlist secret
    base = {
        "ENVIRONMENT": "prod",
        "DOCS_ENABLED": False,
        "CORS_ALLOW_ORIGINS": "https://a.example.com",
        "TRUSTED_HOSTS": "api.example.com",
        "AUTH_BEARER_TOKEN": service,
    }
    for admin in ("short", "test-token", service):
        with pytest.raises(ValueError, match="AUTH_ADMIN_BEARER_TOKEN"):
            build_test_settings(**base, AUTH_ADMIN_BEARER_TOKEN=admin)
    build_test_settings(**base, AUTH_ADMIN_BEARER_TOKEN=strong)
