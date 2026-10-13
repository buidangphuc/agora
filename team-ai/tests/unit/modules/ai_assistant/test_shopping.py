from collections.abc import Sequence

from llama_index.core.schema import NodeWithScore
from loguru import logger

from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.catalog import CATALOG
from app.modules.business.ai_assistant.retrieval import (
    DemoCatalogRetriever,
    RagProductRetriever,
)
from app.modules.business.ai_assistant.schemas import ShoppingAssistantRequest
from app.modules.business.ai_assistant.shopping import (
    RAG_UNAVAILABLE_REPLY,
    ShoppingAssistantService,
)

REDACTION = RedactionPolicy(mode="redacted")


class FailingSearch:
    async def search(
        self, query: str, *, top_k: int | None = None
    ) -> Sequence[NodeWithScore]:
        raise RuntimeError("redis down")


class EmptySearch:
    async def search(
        self, query: str, *, top_k: int | None = None
    ) -> Sequence[NodeWithScore]:
        return []


async def test_retrieval_failure_answers_anyway_and_logs_only_the_exception_type():
    service = ShoppingAssistantService(
        RagProductRetriever(FailingSearch(), min_score=0.0), redaction=REDACTION
    )
    lines: list[str] = []
    sink = logger.add(lambda m: lines.append(str(m)), level="WARNING")
    try:
        result = await service.answer(
            ShoppingAssistantRequest(message="laptop gaming secret-query")
        )
    finally:
        logger.remove(sink)

    assert result.reply_text == RAG_UNAVAILABLE_REPLY
    assert result.product_cards == []
    assert result.suggested_followups
    blob = "\n".join(lines)
    assert "RuntimeError" in blob
    assert "secret-query" not in blob


async def test_demo_catalog_never_stands_in_for_an_empty_store():
    service = ShoppingAssistantService(
        RagProductRetriever(EmptySearch(), min_score=0.0), redaction=REDACTION
    )

    result = await service.answer(ShoppingAssistantRequest(message="tai nghe"))

    assert result.product_cards == []
    assert result.reply_text != RAG_UNAVAILABLE_REPLY
    assert not any(
        c.listing_id in {i["listing_id"] for i in CATALOG} for c in result.product_cards
    )


async def test_demo_retriever_answers_when_no_store_is_configured():
    service = ShoppingAssistantService(DemoCatalogRetriever(), redaction=REDACTION)

    result = await service.answer(ShoppingAssistantRequest(message="tai nghe", top_k=2))

    assert 0 < len(result.product_cards) <= 2
