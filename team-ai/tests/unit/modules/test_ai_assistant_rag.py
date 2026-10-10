"""ShoppingAssistant is grounded in the RAG store the listing indexer feeds."""

from __future__ import annotations

from typing import Any

import pytest
from llama_index.core.embeddings import MockEmbedding
from llama_index.core.schema import NodeWithScore, TextNode

from app.core.redaction import RedactionPolicy
from app.modules.ai.rag.service import KnowledgeRetrievalService, build_rag_node_parser
from app.modules.business.ai_assistant.schemas import ShoppingAssistantRequest
from app.modules.business.ai_assistant.service import (
    CATALOG,
    RAG_UNAVAILABLE_REPLY,
    AIAssistantService,
)
from app.modules.messaging.indexer.handler import (
    ListingEventIndexer,
    ListingEventPayload,
)
from app.modules.platform.identity.schemas import Principal
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2
from app.transport.grpc.context import bind_principal, reset_principal
from app.transport.grpc.servicers.ai import AIServicer


def _rag() -> KnowledgeRetrievalService:
    return KnowledgeRetrievalService(
        embed_model=MockEmbedding(embed_dim=16),
        node_parser=build_rag_node_parser(chunk_size=64, chunk_overlap=8),
        redaction_policy=RedactionPolicy(mode="redacted"),
    )


def _event(listing_id: str, action: str = "CREATED", **kw: Any) -> ListingEventPayload:
    base: dict[str, Any] = {
        "listing_id": listing_id,
        "action": action,
        "title": f"Laptop {listing_id}",
        "price": 1_500_000.0,
        "currency": "VND",
        "status": "PUBLISHED",
    }
    base.update(kw)
    return ListingEventPayload(**base)


class _Stub:
    def __init__(self, nodes: list[NodeWithScore] | Exception) -> None:
        self.nodes = nodes
        self.top_k: int | None = None

    async def search(self, query: str, *, top_k: int | None = None, **_: Any):
        self.top_k = top_k
        if isinstance(self.nodes, Exception):
            raise self.nodes
        return self.nodes


def _hit(listing_id: str, score: float, chunk: int = 0) -> NodeWithScore:
    node = TextNode(
        id_=f"{listing_id}:chunk:{chunk}",
        text="x",
        metadata={
            "listing_id": listing_id,
            "title": f"T {listing_id}",
            "price": 100.0,
            "currency": "VND",
        },
    )
    return NodeWithScore(node=node, score=score)


async def test_indexed_listing_is_returned_then_gone_once_unpublished_or_deleted():
    rag = _rag()
    indexer = ListingEventIndexer(rag)
    await indexer.handle_event(_event("L-1"))
    await indexer.handle_event(_event("L-2"))
    svc = AIAssistantService(rag_service=rag)
    req = ShoppingAssistantRequest(message="laptop", top_k=5)

    ids = {c.listing_id for c in (await svc.shopping_assistant(req)).product_cards}
    assert ids == {"L-1", "L-2"}
    card = next(
        c
        for c in (await svc.shopping_assistant(req)).product_cards
        if c.listing_id == "L-1"
    )
    assert (card.title, card.price, card.currency) == ("Laptop L-1", 1_500_000, "VND")

    await indexer.handle_event(_event("L-1", "UPDATED", status="DRAFT"))
    await indexer.handle_event(_event("L-2", "DELETED"))
    result = await svc.shopping_assistant(req)
    assert result.product_cards == []
    assert not any(c["listing_id"] in {"L-1", "L-2"} for c in CATALOG)  # no stand-in


async def test_cards_are_distinct_bounded_and_filtered_by_score():
    stub = _Stub(
        [
            _hit("A", 0.9),
            _hit("A", 0.95, 1),
            _hit("B", 0.8),
            _hit("C", 0.7),
            _hit("D", 0.1),
        ]
    )
    svc = AIAssistantService(rag_service=stub, rag_min_score=0.3)

    cards = (
        await svc.shopping_assistant(ShoppingAssistantRequest(message="q", top_k=2))
    ).product_cards

    assert [c.listing_id for c in cards] == ["A", "B"]  # distinct, ordered, bounded
    assert stub.top_k is not None and stub.top_k > 2  # over-fetched for chunks

    all_cards = (
        await svc.shopping_assistant(ShoppingAssistantRequest(message="q", top_k=10))
    ).product_cards
    assert [c.listing_id for c in all_cards] == ["A", "B", "C"]  # D under the floor


@pytest.mark.parametrize("failure", [RuntimeError("redis down"), TimeoutError()])
async def test_retrieval_failure_still_answers_without_fabricated_cards(failure):
    svc = AIAssistantService(rag_service=_Stub(failure))

    result = await svc.shopping_assistant(ShoppingAssistantRequest(message="laptop"))

    assert result.product_cards == []
    assert result.reply_text == RAG_UNAVAILABLE_REPLY
    assert result.suggested_followups


async def test_without_rag_store_the_demo_catalog_answers():
    result = await AIAssistantService().shopping_assistant(
        ShoppingAssistantRequest(message="tai nghe")
    )

    demo_ids = {c["listing_id"] for c in CATALOG}
    assert result.product_cards
    assert {c.listing_id for c in result.product_cards} <= demo_ids


async def test_servicer_returns_the_retrieved_listing_ids_over_grpc():
    svc = AIAssistantService(rag_service=_Stub([_hit("real-1", 0.9)]))
    servicer = AIServicer(lambda: svc)

    token = bind_principal(Principal(id="b", type="user", scopes=("listing.read",)))
    try:
        resp = await servicer.ShoppingAssistant(
            ai_pb2.ShoppingAssistantRequest(message="laptop"),
            object(),  # type: ignore[arg-type]
        )
    finally:
        reset_principal(token)

    assert [c.listing_id for c in resp.product_cards] == ["real-1"]


async def test_empty_qdrant_collection_means_no_cards_not_an_error():
    """First use: nothing indexed yet, so the collection does not exist."""
    from llama_index.core.embeddings import MockEmbedding

    from app.core.redaction import RedactionPolicy
    from app.modules.ai.rag.service import (
        KnowledgeRetrievalService,
        build_rag_node_parser,
    )

    async def absent() -> bool:
        return False

    rag = KnowledgeRetrievalService(
        embed_model=MockEmbedding(embed_dim=8),
        node_parser=build_rag_node_parser(chunk_size=256, chunk_overlap=0),
        redaction_policy=RedactionPolicy(mode="redacted"),
        index_exists=absent,
    )
    assert await rag.search("laptop") == []
    await rag.delete("never-indexed")  # would 404 against a real absent collection

    result = await AIAssistantService(rag_service=rag).shopping_assistant(
        ShoppingAssistantRequest(message="laptop")
    )
    assert result.product_cards == []
    assert result.reply_text != RAG_UNAVAILABLE_REPLY


async def test_qdrant_backed_service_searches_an_unindexed_collection(monkeypatch):
    """Real QdrantVectorStore wiring (local in-memory qdrant): the async client is
    present (aretrieve raises ValueError without it) and the absent collection is
    an empty result."""
    qdrant_client = pytest.importorskip("qdrant_client")
    from llama_index.core.embeddings import MockEmbedding

    from app.modules.ai.rag.factory import build_rag_service
    from tests.factories import build_test_settings

    real_sync, real_async = qdrant_client.QdrantClient, qdrant_client.AsyncQdrantClient
    monkeypatch.setattr(
        qdrant_client, "QdrantClient", lambda url: real_sync(":memory:")
    )
    monkeypatch.setattr(
        qdrant_client, "AsyncQdrantClient", lambda url: real_async(":memory:")
    )

    rag = build_rag_service(
        build_test_settings(RAG_BACKEND="qdrant", RAG_QDRANT_URL="http://unused"),
        embed_model=MockEmbedding(embed_dim=8),
    )

    assert rag.storage_context.vector_store._aclient is not None  # type: ignore[attr-defined]
    assert await rag.search("laptop") == []
