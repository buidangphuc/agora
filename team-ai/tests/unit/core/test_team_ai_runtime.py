"""team-ai runtime: gRPC, Redis and recommendations are always on; unused template
capabilities are gone (openspec change ``team-ai-runtime``)."""

from __future__ import annotations

import re
from pathlib import Path

import grpc
import pytest
from grpc_reflection.v1alpha import reflection_pb2, reflection_pb2_grpc
from pydantic import ValidationError

from app.bootstrap.addons.base import default_resource_addons
from app.bootstrap.application import create_app
from app.core.config import Settings
from app.modules.business.recommendation.service import RecommendationService
from tests.factories import api_client_for, api_key_entry, build_test_settings

REPO = Path(__file__).resolve().parents[3]

# Production values of platform-gitops/argocd/apps/team-ai.yaml, as OS env vars.
GITOPS_ENV = {
    "GRPC_ENABLED": "true",
    "GRPC_HOST": "0.0.0.0",
    "GRPC_PORT": "50060",
    "CHAT_BACKEND": "mock",
    "RAG_ENABLED": "false",
    "KAFKA_ENABLED": "false",
    "SEARCH_ENABLED": "false",
    "DATABASE_ENABLED": "false",
    "QUEUE_ENABLED": "false",
    "REDIS_ENABLED": "true",
    "REDIS_HOST": "valkey",
    "REDIS_PORT": "6379",
    "RECS_ENABLED": "true",
    "RECS_BACKEND": "qdrant",
    "RECS_QDRANT_URL": "http://qdrant:6333",
    "RECS_QDRANT_COLLECTION": "item_als_vectors",
    "RECS_CACHE_PREFIX": "recs",
    "RECS_CACHE_SCHEMA_VERSION": "v1",
    "RECS_CACHE_TTL_SECONDS": "86400",
    "RECS_FEATURESTORE_REDIS_URL": "redis://valkey:6379/2",
    "RECS_NEARLINE_REDIS_URL": "redis://valkey:6379/0",
    "RECS_RETRIEVE_TIMEOUT_MS": "15",
    "OBJECTS_ENABLED": "false",
}

REMOVED_FLAGS = ("GRPC_ENABLED", "REDIS_ENABLED", "RECS_ENABLED")


@pytest.mark.parametrize("flag", REMOVED_FLAGS)
def test_removed_flags_are_rejected_in_env_files(tmp_path: Path, flag: str):
    env_file = tmp_path / ".env"
    env_file.write_text(f"REDIS_HOST=valkey\n{flag}=true\n")

    with pytest.raises(ValidationError) as excinfo:
        Settings(_env_file=env_file)  # pyright: ignore[reportCallIssue]

    assert [(e["type"], e["loc"]) for e in excinfo.value.errors()] == [
        ("extra_forbidden", (flag.lower(),))
    ]


def test_gitops_environment_validates(monkeypatch: pytest.MonkeyPatch):
    for key, value in GITOPS_ENV.items():
        monkeypatch.setenv(key, value)
    monkeypatch.setenv("AUTH_API_KEYS", api_key_entry("gateway", "s3cret-gw", "admin"))

    settings = Settings(_env_file=None)  # pyright: ignore[reportCallIssue]

    assert settings.GRPC_PORT == 50060
    assert settings.REDIS_HOST == "valkey"
    assert settings.RECS_QDRANT_URL == "http://qdrant:6333"


def test_redis_host_has_no_default(monkeypatch: pytest.MonkeyPatch):
    monkeypatch.delenv("REDIS_HOST", raising=False)

    with pytest.raises(ValidationError, match="REDIS_HOST"):
        Settings(_env_file=None)  # pyright: ignore[reportCallIssue]


async def _served_services(port: int) -> list[str]:
    async with grpc.aio.insecure_channel(f"127.0.0.1:{port}") as channel:
        stub = reflection_pb2_grpc.ServerReflectionStub(channel)
        call = stub.ServerReflectionInfo(
            iter([reflection_pb2.ServerReflectionRequest(list_services="")])
        )
        async for reply in call:
            return [s.name for s in reply.list_services_response.service]
    return []


async def test_recommendation_always_composed():
    app = create_app(settings=build_test_settings(GRPC_HOST="127.0.0.1"))

    async with api_client_for(app):
        assert isinstance(app.state.resources.recommendation, RecommendationService)
        services = await _served_services(app.state.grpc_port)

    assert "platform.recommendation.v1.RecommendationService" in services


def test_remaining_capability_set():
    addons = {addon.name: addon for addon in default_resource_addons()}
    defaults = build_test_settings()

    assert set(addons) == {
        "telemetry",
        "rate_limit",
        "quota",
        "rag",
        "taxonomy",
        "recommendation",
        "listing_indexer",
    }
    assert [n for n, a in addons.items() if a.is_enabled(defaults)] == [
        "rate_limit",
        "quota",
        "taxonomy",
        "recommendation",
    ]


_REMOVED_IMPORT = re.compile(
    r"^\s*(?:from|import)\s+"
    r"(?:sqlalchemy|alembic|asyncpg|aio_pika|aioboto3|motor|pymongo"
    r"|app\.core\.(?:database|mongo|aws_errors)"
    r"|app\.modules\.messaging\.(?:queue|tasks|outbox|webhooks)"
    r"|app\.modules\.platform\.(?:cache|idempotency|mongo|objects|audit))\b",
    re.MULTILINE,
)


def test_no_imports_of_removed_packages():
    offenders = [
        str(path.relative_to(REPO))
        for root in ("app", "tests", "scripts")
        for path in (REPO / root).rglob("*.py")
        if path != Path(__file__).resolve()
        and "_pb" not in path.parts
        and _REMOVED_IMPORT.search(path.read_text())
    ]

    assert offenders == []
