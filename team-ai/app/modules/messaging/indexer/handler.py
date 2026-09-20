"""Handler for indexing listing state-change events into AI RAG."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

from loguru import logger


@dataclass
class IndexDocument:
    """Document representation for RAG indexing."""

    text: str
    id_: str
    metadata: dict[str, Any] = field(default_factory=dict)


def _create_document(text: str, id_: str, metadata: dict[str, Any]) -> Any:
    try:
        from llama_index.core import Document

        return Document(text=text, id_=id_, metadata=metadata)
    except ImportError:
        return IndexDocument(text=text, id_=id_, metadata=metadata)


@dataclass
class ListingEventPayload:
    listing_id: str
    action: str  # "CREATED", "UPDATED", "DELETED", "ARCHIVED"
    title: str = ""
    description: str = ""
    category: str = ""
    price: float = 0.0
    attributes: dict[str, Any] | None = None


class ListingEventIndexer:
    """Consumes listing events and keeps RAG vector index synchronized."""

    def __init__(self, rag_service: Any) -> None:
        self.rag_service = rag_service

    async def handle_event(self, event: ListingEventPayload) -> dict[str, Any]:
        action = event.action.upper()
        listing_id = event.listing_id

        if not listing_id:
            logger.warning("indexer.skipped_empty_listing_id")
            return {"status": "skipped", "reason": "empty_listing_id"}

        if action in ("DELETED", "ARCHIVED", "UNPUBLISHED"):
            logger.info("indexer.deleting_listing listing_id={}", listing_id)
            await self.rag_service.delete(listing_id)
            return {"status": "deleted", "listing_id": listing_id}

        if action in ("CREATED", "UPDATED", "PUBLISHED"):
            # Build search-optimized document representation
            text_parts = [
                f"Product: {event.title}",
                f"Category: {event.category}" if event.category else "",
                f"Price: {event.price}" if event.price > 0 else "",
                f"Description: {event.description}" if event.description else "",
            ]
            if event.attributes:
                attr_str = ", ".join(f"{k}: {v}" for k, v in event.attributes.items())
                text_parts.append(f"Attributes: {attr_str}")

            doc_text = "\n".join(part for part in text_parts if part)

            metadata: dict[str, Any] = {
                "listing_id": listing_id,
                "title": event.title,
                "category": event.category,
                "price": event.price,
            }

            document = _create_document(
                text=doc_text,
                id_=listing_id,
                metadata=metadata,
            )

            logger.info("indexer.indexing_listing listing_id={}", listing_id)
            result = await self.rag_service.index([document])
            return {
                "status": "indexed",
                "listing_id": listing_id,
                "chunk_count": result.get("chunk_count", 1),
            }

        logger.warning("indexer.unhandled_action action={} listing_id={}", action, listing_id)
        return {"status": "ignored", "action": action}
