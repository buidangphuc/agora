"""Qdrant RAG backend over qdrant-client's local mode (no server)."""

import uuid

import pytest
from llama_index.core import Document
from llama_index.core.embeddings import MockEmbedding

from app.core.redaction import RedactionPolicy
from app.modules.ai.rag.qdrant import QdrantBackend, build_qdrant_backend
from app.modules.ai.rag.service import KnowledgeRetrievalService, build_rag_node_parser

qdrant_client = pytest.importorskip("qdrant_client")

COLLECTION = "rag_test"


def _backend() -> QdrantBackend:
    client = qdrant_client.QdrantClient(location=":memory:")
    aclient = qdrant_client.AsyncQdrantClient(location=":memory:")
    # Local mode keeps data per client object; the service writes through the
    # sync client and reads through the async one, so make them share storage.
    aclient._client.collections = client._client.collections  # pyright: ignore[reportPrivateUsage,reportAttributeAccessIssue]
    return build_qdrant_backend(client=client, aclient=aclient, collection=COLLECTION)


def _service(backend: QdrantBackend) -> KnowledgeRetrievalService:
    return KnowledgeRetrievalService(
        embed_model=MockEmbedding(embed_dim=16),
        node_parser=build_rag_node_parser(chunk_size=64, chunk_overlap=8),
        redaction_policy=RedactionPolicy(mode="redacted"),
        storage_context=backend.storage_context,
        collection_exists=backend.collection_exists,
        on_close=backend.aclose,
    )


async def test_absent_collection_searches_empty_and_deletes_nothing() -> None:
    service = _service(_backend())

    assert await service.search("anything") == []
    await service.delete("L1")


async def test_index_search_delete_roundtrip_with_uuid5_point_ids() -> None:
    backend = _backend()
    service = _service(backend)
    await service.index(
        [Document(id_="L1", text="Product: Tai nghe", metadata={"category": "audio"})]
    )

    hits = await service.search("Tai nghe", filters={"category": "audio"})

    assert [h.node.metadata["document_id"] for h in hits] == ["L1"]
    points, _ = await backend.aclient.scroll(COLLECTION, limit=10)
    expected = str(uuid.uuid5(uuid.NAMESPACE_URL, "L1:chunk:0"))
    assert [str(p.id) for p in points] == [expected]

    await service.delete("L1")
    assert await service.search("Tai nghe") == []


async def test_aclose_closes_both_clients() -> None:
    backend = _backend()

    await backend.aclose()

    with pytest.raises(RuntimeError):
        await backend.aclient.get_collections()


async def test_addon_closes_the_clients_it_opened_on_shutdown(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from app.bootstrap.addons import rag as rag_module
    from app.bootstrap.addons.rag import RagAddon
    from app.bootstrap.resources import ApplicationResources
    from app.modules.ai.rag.factory import build_rag_service
    from tests.factories import build_test_settings, open_addon

    closed: list[object] = []

    async def aclose(self: object) -> None:
        closed.append(self)

    monkeypatch.setattr(qdrant_client.AsyncQdrantClient, "close", aclose)
    monkeypatch.setattr(
        qdrant_client.QdrantClient, "close", lambda self: closed.append(self)
    )
    # The suite-wide in-memory stand-in is for app-level tests; this one wants Qdrant.
    monkeypatch.setattr(rag_module, "build_rag_service", build_rag_service)
    resources = ApplicationResources()
    settings = build_test_settings(
        RAG_ENABLED=True,
        RAG_EMBED_SERVER_URL="http://embed:8080",
    )

    stack = await open_addon(RagAddon(), resources, settings)
    assert resources.rag_service is not None
    store = resources.rag_service.storage_context.vector_store
    await stack.aclose()

    assert store._client in closed  # pyright: ignore[reportPrivateUsage,reportAttributeAccessIssue]
    assert store._aclient in closed  # pyright: ignore[reportPrivateUsage,reportAttributeAccessIssue]
