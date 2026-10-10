"""Handler for indexing listing state-change events into AI RAG."""

from __future__ import annotations

from collections import OrderedDict
from dataclasses import dataclass, field
from typing import Any

from loguru import logger

# Listing statuses (platform.listing.v1.ListingStatus) that must NOT be retrievable by
# RAG: a draft is not live yet and a rejected listing was taken down. UNSPECIFIED is
# indexed (a producer that does not set a status is not unpublishing).
_NOT_ACTIVE_STATUSES = frozenset({"DRAFT", "REJECTED"})
_REMOVE_ACTIONS = frozenset({"DELETED", "ARCHIVED", "UNPUBLISHED"})
_INDEX_ACTIONS = frozenset({"CREATED", "UPDATED", "PUBLISHED"})

_DEDUPE_CAPACITY = 10_000
_VERSION_CAPACITY = 50_000


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
    status: str = ""  # "DRAFT" | "PUBLISHED" | "REJECTED" | "" (unknown)
    seller_id: str = ""
    currency: str = ""
    # Envelope identity, for idempotency: a redelivered event carries the same
    # event_id; occurred_ns orders events of one listing (0 = unknown).
    event_id: str = ""
    occurred_ns: int = 0


class _BoundedMap:
    """A small LRU map: bounded memory for the idempotency bookkeeping."""

    def __init__(self, capacity: int) -> None:
        self._capacity = capacity
        self._data: OrderedDict[str, int] = OrderedDict()

    def get(self, key: str) -> int | None:
        return self._data.get(key)

    def put(self, key: str, value: int = 0) -> None:
        self._data[key] = value
        self._data.move_to_end(key)
        while len(self._data) > self._capacity:
            self._data.popitem(last=False)


class ListingEventIndexer:
    """Consumes listing events and keeps RAG vector index synchronized.

    Idempotent under at-least-once delivery: an already applied ``event_id`` is a
    no-op (no second embedding call), an event older than the last one applied to the
    same listing is skipped, and an update replaces the listing's chunks (delete then
    index) so a shorter description never leaves stale chunks behind.
    """

    def __init__(self, rag_service: Any) -> None:
        self.rag_service = rag_service
        self._applied_events = _BoundedMap(_DEDUPE_CAPACITY)
        self._listing_versions = _BoundedMap(_VERSION_CAPACITY)

    async def handle_event(self, event: ListingEventPayload) -> dict[str, Any]:
        action = event.action.upper()
        listing_id = event.listing_id

        if not listing_id:
            logger.warning("indexer.skipped_empty_listing_id")
            return {"status": "skipped", "reason": "empty_listing_id"}

        if event.event_id and self._applied_events.get(event.event_id) is not None:
            logger.info("indexer.duplicate_event event_id={}", event.event_id)
            return {"status": "duplicate", "listing_id": listing_id}

        last = self._listing_versions.get(listing_id)
        if event.occurred_ns and last is not None and event.occurred_ns < last:
            logger.info("indexer.stale_event listing_id={}", listing_id)
            return {"status": "stale", "listing_id": listing_id}

        if action in _REMOVE_ACTIONS or (
            action in _INDEX_ACTIONS and event.status.upper() in _NOT_ACTIVE_STATUSES
        ):
            logger.info("indexer.deleting_listing listing_id={}", listing_id)
            await self.rag_service.delete(listing_id)
            self._applied(event)
            return {"status": "deleted", "listing_id": listing_id}

        if action in _INDEX_ACTIONS:
            document = _create_document(
                text=_document_text(event),
                id_=listing_id,
                metadata={
                    "listing_id": listing_id,
                    "title": event.title,
                    "category": event.category,
                    "price": event.price,
                    "currency": event.currency,
                    "seller_id": event.seller_id,
                    "status": event.status or "PUBLISHED",
                },
            )
            logger.info("indexer.indexing_listing listing_id={}", listing_id)
            # Replace, not append: drop the previous chunks first (a no-op for a new id).
            await self.rag_service.delete(listing_id)
            result = await self.rag_service.index([document])
            self._applied(event)
            return {
                "status": "indexed",
                "listing_id": listing_id,
                "chunk_count": result.get("chunk_count", 1),
            }

        logger.warning(
            "indexer.unhandled_action action={} listing_id={}", action, listing_id
        )
        return {"status": "ignored", "action": action}

    def _applied(self, event: ListingEventPayload) -> None:
        """Remember an applied event (only after it succeeded, so a retry still runs)."""
        if event.event_id:
            self._applied_events.put(event.event_id)
        if event.occurred_ns:
            self._listing_versions.put(event.listing_id, event.occurred_ns)


def _document_text(event: ListingEventPayload) -> str:
    # Search-optimized document representation.
    parts = [
        f"Product: {event.title}",
        f"Category: {event.category}" if event.category else "",
        f"Price: {event.price}" if event.price > 0 else "",
        f"Description: {event.description}" if event.description else "",
    ]
    if event.attributes:
        attr_str = ", ".join(f"{k}: {v}" for k, v in event.attributes.items())
        parts.append(f"Attributes: {attr_str}")
    return "\n".join(part for part in parts if part)
