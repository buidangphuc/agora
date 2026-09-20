"""Unit tests for ListingEventIndexer."""

from __future__ import annotations

import asyncio
from typing import Any

from app.modules.messaging.indexer.handler import (
    ListingEventIndexer,
    ListingEventPayload,
)


class FakeRAGService:
    def __init__(self) -> None:
        self.indexed_docs: list[Any] = []
        self.deleted_ids: list[str] = []

    async def index(self, documents: list[Any]) -> dict[str, int]:
        self.indexed_docs.extend(documents)
        return {"indexed_count": len(documents), "chunk_count": len(documents)}

    async def delete(self, document_id: str) -> None:
        self.deleted_ids.append(document_id)


def test_listing_indexer_create_event() -> None:
    async def _run():
        fake_rag = FakeRAGService()
        indexer = ListingEventIndexer(rag_service=fake_rag)

        event = ListingEventPayload(
            listing_id="listing-101",
            action="CREATED",
            title="Wireless Ergonomic Mouse",
            description="Bluetooth 5.0 rechargeable mouse",
            category="Electronics",
            price=29.99,
            attributes={"color": "black", "dpi": 4000},
        )

        result = await indexer.handle_event(event)
        assert result["status"] == "indexed"
        assert result["listing_id"] == "listing-101"

        assert len(fake_rag.indexed_docs) == 1
        doc = fake_rag.indexed_docs[0]
        assert doc.id_ == "listing-101"
        assert "Wireless Ergonomic Mouse" in doc.text
        assert "Electronics" in doc.text
        assert "29.99" in doc.text
        assert doc.metadata["category"] == "Electronics"

    asyncio.run(_run())


def test_listing_indexer_delete_event() -> None:
    async def _run():
        fake_rag = FakeRAGService()
        indexer = ListingEventIndexer(rag_service=fake_rag)

        event = ListingEventPayload(
            listing_id="listing-101",
            action="DELETED",
        )

        result = await indexer.handle_event(event)
        assert result["status"] == "deleted"
        assert "listing-101" in fake_rag.deleted_ids

    asyncio.run(_run())


def test_listing_indexer_skipped_empty_id() -> None:
    async def _run():
        fake_rag = FakeRAGService()
        indexer = ListingEventIndexer(rag_service=fake_rag)

        event = ListingEventPayload(
            listing_id="",
            action="CREATED",
        )

        result = await indexer.handle_event(event)
        assert result["status"] == "skipped"

    asyncio.run(_run())
