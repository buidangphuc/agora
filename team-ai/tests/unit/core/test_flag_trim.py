"""flag-trim: four switches remain; limits, taxonomy and quota are always on."""

from __future__ import annotations

import re
from pathlib import Path

import fakeredis
import grpc
import pytest
from fakeredis import FakeServer
from pydantic import ValidationError

from app.bootstrap.addons import taxonomy
from app.bootstrap.addons.telemetry import TelemetryAddon
from app.bootstrap.application import create_app
from app.core.config import Settings
from app.modules.ai.llm import langfuse as langfuse_module
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2, ai_pb2_grpc
from tests.factories import api_client_for, api_key_entry, build_test_settings

REMOVED_FLAGS = (
    "GRPC_REFLECTION_ENABLED",
    "GRPC_BEARER_FALLBACK_ENABLED",
    "GRPC_RATE_LIMIT_ENABLED",
    "GRACEFUL_SHUTDOWN_ENABLED",
    "REQUEST_TIMEOUT_ENABLED",
    "SECURITY_HEADERS_ENABLED",
    "SECURITY_HSTS_ENABLED",
    "GZIP_ENABLED",
    "RATE_LIMIT_ENABLED",
    "RATE_LIMIT_IP_ENABLED",
    "RATE_LIMIT_PRINCIPAL_ENABLED",
    "QUOTA_ENABLED",
    "TAXONOMY_PERSISTENCE_ENABLED",
    "LISTING_INDEXER_ENABLED",
    "OTEL_ENABLED",
    "LANGFUSE_ENABLED",
)
REMOVED_SELECTORS = (
    "RATE_LIMIT_BACKEND",
    "QUOTA_BACKEND",
    "RAG_BACKEND",
    "RAG_EMBED_BACKEND",
    "RECS_BACKEND",
)
PRINCIPAL_MD = (
    ("x-principal-id", "u1"),
    ("x-principal-type", "user"),
    ("x-principal-scopes", "listing.read"),
)


@pytest.mark.parametrize("key", (*REMOVED_FLAGS, *REMOVED_SELECTORS))
def test_removed_key_in_env_file_is_rejected(tmp_path: Path, key: str):
    env_file = tmp_path / ".env"
    env_file.write_text(f"REDIS_HOST=valkey\n{key}=true\n")

    with pytest.raises(ValidationError) as excinfo:
        Settings(_env_file=env_file)  # pyright: ignore[reportCallIssue]

    assert [e["type"] for e in excinfo.value.errors()] == ["extra_forbidden"]


def test_switches_are_the_four_that_remain():
    switches = {
        name
        for name in Settings.model_fields
        if re.search(r"_ENABLED$|_BACKEND$", name)
    }

    assert switches == {
        "RAG_ENABLED",
        "DOCS_ENABLED",
        "CHAT_BACKEND",
        "SCREEN_BACKEND",  # ai-listing-screen: mock|llm_router selector
    }
    assert "AI_USE_SCOPE_REQUIRED" in Settings.model_fields


def _prod_settings(**overrides: object) -> Settings:
    return build_test_settings(
        ENVIRONMENT="production",
        CORS_ALLOW_ORIGINS="https://app.example.com",
        TRUSTED_HOSTS="test",
        AUTH_API_KEYS=api_key_entry("gateway", "s3cret-gw", "admin"),
        **overrides,
    )


async def _status(settings: Settings, path: str) -> int:
    async with api_client_for(create_app(settings=settings, init_resources=False)) as c:
        return (await c.get(path)).status_code


async def test_docs_are_off_in_production_by_default():
    assert await _status(_prod_settings(), "/docs") == 404
    assert await _status(_prod_settings(), "/openapi.json") == 404


async def test_docs_are_on_locally_by_default():
    assert await _status(build_test_settings(ENVIRONMENT="local"), "/docs") == 200


def test_llm_router_in_production_has_no_boot_guard():
    assert _prod_settings(CHAT_BACKEND="llm_router").CHAT_BACKEND == "llm_router"


def test_rag_on_needs_the_embed_server():
    with pytest.raises(ValidationError, match="RAG_EMBED_SERVER_URL"):
        build_test_settings(RAG_ENABLED=True, RAG_EMBED_SERVER_URL="")


async def test_grpc_burst_over_the_limit_is_resource_exhausted():
    app = create_app(
        settings=build_test_settings(
            GRPC_HOST="127.0.0.1", RATE_LIMIT_PRINCIPAL_PER_MINUTE=2
        )
    )
    request = ai_pb2.ShoppingAssistantRequest(message="tai nghe")

    async with api_client_for(app):
        async with grpc.aio.insecure_channel(
            f"127.0.0.1:{app.state.grpc_port}"
        ) as channel:
            stub = ai_pb2_grpc.AIServiceStub(channel)
            for _ in range(2):
                await stub.ShoppingAssistant(request, metadata=PRINCIPAL_MD)
            with pytest.raises(grpc.aio.AioRpcError) as exc:
                await stub.ShoppingAssistant(request, metadata=PRINCIPAL_MD)

    assert exc.value.code() == grpc.StatusCode.RESOURCE_EXHAUSTED
    assert dict(exc.value.trailing_metadata() or ()).get("retry-after")


async def test_taxonomy_is_persisted_by_default(monkeypatch: pytest.MonkeyPatch):
    fake = FakeServer()
    monkeypatch.setattr(
        taxonomy,
        "build_taxonomy_redis_client",
        lambda settings: fakeredis.FakeAsyncRedis(server=fake, decode_responses=True),
    )
    admin = {"Authorization": "Bearer admin-token"}
    settings = build_test_settings(
        AUTH_API_KEYS=api_key_entry("admin", "admin-token", "admin", "ai.classify")
    )
    explore = {
        "batch_listings": [
            {"listing_id": "a", "title": "Sạc GaN công suất 77W"},
            {"listing_id": "b", "title": "Củ sạc laptop 77W"},
        ],
        "min_frequency": 2,
        "min_confidence": 0.8,
    }
    base = "/api/v1/ai"

    async with api_client_for(create_app(settings=settings, init_resources=False)) as c:
        await c.post(f"{base}/tags/explore", json=explore, headers=admin)
        promoted = await c.post(
            f"{base}/tags/promote",
            json={"tag_slugs": ["cong-suat-77w"]},
            headers=admin,
        )
        assert promoted.json()["total_promoted"] == 1

    async with api_client_for(create_app(settings=settings, init_resources=False)) as c:
        response = await c.get(
            f"{base}/tags", params={"status": "promoted"}, headers=admin
        )
        assert "cong-suat-77w" in {t["slug"] for t in response.json()["tags"]}


def test_otel_starts_exactly_when_the_endpoint_is_set():
    assert not TelemetryAddon().is_enabled(build_test_settings())
    assert TelemetryAddon().is_enabled(
        build_test_settings(OTEL_EXPORTER_OTLP_ENDPOINT="http://127.0.0.1:4318")
    )


def test_langfuse_starts_exactly_when_both_keys_are_set(
    monkeypatch: pytest.MonkeyPatch,
):
    monkeypatch.setattr(
        langfuse_module, "_build_langfuse_client", lambda settings: object()
    )

    def tracker(**overrides: object):
        return langfuse_module.build_langfuse_tracker(
            build_test_settings(**overrides), instance_id="t", service_name="t"
        )

    assert tracker().client is None
    assert tracker(LANGFUSE_PUBLIC_KEY="pk").client is None
    keys = {
        "LANGFUSE_PUBLIC_KEY": "pk",
        "LANGFUSE_SECRET_KEY": "sk",  # pragma: allowlist secret
    }
    assert tracker(**keys).client
