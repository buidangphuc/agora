import asyncio

import fakeredis.aioredis as fake_aioredis
import pytest
from httpx import AsyncClient

from app.bootstrap import resources as resources_module, services as services_module
from app.bootstrap.addons import (
    listing_indexer as listing_indexer_module,
    rag as rag_module,
    taxonomy as taxonomy_module,
)
from app.bootstrap.application import create_app
from app.core.config import Settings
from app.modules.business.recommendation.adapters.memory import MemoryCandidateSource
from tests.factories import (
    api_client_for,
    build_memory_rag_service,
    build_test_settings,
)


@pytest.fixture(autouse=True)
def fake_redis(monkeypatch: pytest.MonkeyPatch) -> None:
    """Every app built in a test gets an in-process Redis (it is never optional).

    ``FakeRedis`` is looked up at call time so a plugin that patches it (the
    real-Redis one) wins; a test that patches ``build_redis_client`` itself wins too.
    """
    monkeypatch.setattr(
        resources_module,
        "build_redis_client",
        lambda settings: fake_aioredis.FakeRedis(decode_responses=True),
    )


@pytest.fixture(autouse=True)
def memory_backends(monkeypatch: pytest.MonkeyPatch) -> None:
    """The offline seams: production builders are Qdrant, model_server, Kafka and a
    taxonomy Redis database; tests swap in memory and fake adapters."""
    monkeypatch.setattr(
        services_module, "build_candidate_source", lambda s, c: MemoryCandidateSource()
    )
    monkeypatch.setattr(rag_module, "build_rag_service", build_memory_rag_service)
    monkeypatch.setattr(
        taxonomy_module,
        "build_taxonomy_redis_client",
        lambda settings: fake_aioredis.FakeRedis(decode_responses=True),
    )

    async def idle_consumer(build) -> None:
        await asyncio.Event().wait()

    monkeypatch.setattr(listing_indexer_module, "supervise", idle_consumer)

    # Tests that call the real RAG builder get an in-memory Qdrant, never a server
    # on RAG_QDRANT_URL (a local dev stack may be listening there).
    def memory_qdrant_backend(*, url: str, collection: str):
        from qdrant_client import AsyncQdrantClient, QdrantClient

        from app.modules.ai.rag.qdrant import build_qdrant_backend

        return build_qdrant_backend(
            client=QdrantClient(location=":memory:"),
            aclient=AsyncQdrantClient(location=":memory:"),
            collection=collection,
        )

    monkeypatch.setattr(
        "app.modules.ai.rag.qdrant.connect_qdrant_backend", memory_qdrant_backend
    )


@pytest.fixture()
def test_settings() -> Settings:
    return build_test_settings()


@pytest.fixture()
async def client(test_settings: Settings):
    app = create_app(settings=test_settings, init_resources=False)
    async with api_client_for(app) as test_client:
        yield test_client


@pytest.fixture()
async def auth_headers(client: AsyncClient) -> dict[str, str]:
    return {"Authorization": "Bearer test-token"}
