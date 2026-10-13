from collections.abc import Sequence

from llama_index.core.schema import NodeWithScore, TextNode

from app.modules.ai.rag.service import KnowledgeRetrievalService
from app.modules.business.ai_assistant.catalog import CATALOG
from app.modules.business.ai_assistant.retrieval import (
    DemoCatalogRetriever,
    ListingSearch,
    RagProductRetriever,
)


class StubSearch:
    def __init__(self, nodes: Sequence[NodeWithScore]) -> None:
        self.nodes = nodes
        self.top_k: int | None = None

    async def search(
        self, query: str, *, top_k: int | None = None
    ) -> Sequence[NodeWithScore]:
        self.top_k = top_k
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


async def test_chunks_collapse_to_distinct_listings_best_first():
    search = StubSearch(
        [
            _hit("A", 0.9),
            _hit("A", 0.95, 1),
            _hit("B", 0.8),
            _hit("C", 0.7),
            _hit("D", 0.1),
        ]
    )
    retriever = RagProductRetriever(search, min_score=0.3)

    cards = await retriever.retrieve("q", top_k=2)

    assert [c.listing_id for c in cards] == ["A", "B"]
    assert search.top_k == 6  # top_k * 3 over-fetch


async def test_listings_under_min_score_are_dropped():
    search = StubSearch(
        [_hit("A", 0.9), _hit("B", 0.8), _hit("C", 0.7), _hit("D", 0.1)]
    )

    cards = await RagProductRetriever(search, min_score=0.3).retrieve("q", top_k=10)

    assert [c.listing_id for c in cards] == ["A", "B", "C"]


async def test_card_maps_metadata_and_has_no_rating_text():
    cards = await RagProductRetriever(
        StubSearch([_hit("A", 0.9)]), min_score=0.0
    ).retrieve("q", top_k=4)

    assert (cards[0].title, cards[0].price, cards[0].currency) == ("T A", 100, "VND")
    assert cards[0].rating_text == ""


async def test_demo_catalog_retriever_serves_catalog_ids():
    cards = await DemoCatalogRetriever().retrieve("tai nghe", top_k=4)

    assert cards
    assert {c.listing_id for c in cards} <= {item["listing_id"] for item in CATALOG}


def test_knowledge_retrieval_service_satisfies_listing_search():
    def accepts(search: KnowledgeRetrievalService) -> ListingSearch:
        return search

    assert callable(accepts)
