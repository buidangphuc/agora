"""Keep the RAG index in step with listing state-change events."""

from collections import OrderedDict
from collections.abc import Sequence
from typing import TYPE_CHECKING, Literal, Protocol

from loguru import logger

from app.modules.messaging.listing_indexer.decode import ListingEventPayload

if TYPE_CHECKING:
    from llama_index.core import Document

# A draft is not live yet and a rejected listing was taken down: neither may be
# retrievable. An unknown status ("") is indexed (the producer is not unpublishing).
_NOT_ACTIVE_STATUSES = frozenset({"DRAFT", "REJECTED"})

_DEDUPE_CAPACITY = 10_000
_VERSION_CAPACITY = 50_000


class RagIndex(Protocol):
    async def index(self, documents: Sequence["Document"]) -> dict[str, int]: ...

    async def delete(self, document_id: str) -> None: ...


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
    """Idempotent under at-least-once delivery.

    A repeated ``event_id`` is a no-op, an event older than the last one applied to
    the same listing is skipped, and an update replaces the listing's chunks (delete,
    then index) so a shorter description never leaves stale chunks behind.
    """

    def __init__(self, rag: RagIndex) -> None:
        self._rag = rag
        self._applied_events = _BoundedMap(_DEDUPE_CAPACITY)
        self._listing_versions = _BoundedMap(_VERSION_CAPACITY)

    async def handle(
        self, payload: ListingEventPayload
    ) -> Literal["indexed", "deleted", "ignored", "duplicate", "stale"]:
        listing_id = payload.listing_id

        if payload.event_id and self._applied_events.get(payload.event_id) is not None:
            logger.info("indexer.duplicate_event event_id={}", payload.event_id)
            return "duplicate"

        last = self._listing_versions.get(listing_id)
        if payload.occurred_ns and last is not None and payload.occurred_ns < last:
            logger.info("indexer.stale_event listing_id={}", listing_id)
            return "stale"

        if payload.action == "UNSPECIFIED":
            logger.warning("indexer.unspecified_action listing_id={}", listing_id)
            return "ignored"

        if payload.action == "DELETED" or payload.status in _NOT_ACTIVE_STATUSES:
            await self._rag.delete(listing_id)
            self._applied(payload)
            return "deleted"

        from llama_index.core import Document

        document = Document(
            id_=listing_id,
            text=_document_text(payload),
            metadata={
                "listing_id": listing_id,
                "title": payload.title,
                "category": payload.category,
                "price": payload.price,
                "currency": payload.currency,
                "seller_id": payload.seller_id,
                "status": payload.status or "PUBLISHED",
            },
        )
        # Replace, not append: drop the previous chunks first (a no-op for a new id).
        await self._rag.delete(listing_id)
        await self._rag.index([document])
        self._applied(payload)
        return "indexed"

    def _applied(self, payload: ListingEventPayload) -> None:
        """Remember an applied event (only after success, so a retry still runs)."""
        if payload.event_id:
            self._applied_events.put(payload.event_id)
        if payload.occurred_ns:
            self._listing_versions.put(payload.listing_id, payload.occurred_ns)


def _document_text(payload: ListingEventPayload) -> str:
    parts = [
        f"Product: {payload.title}",
        f"Category: {payload.category}" if payload.category else "",
        f"Price: {payload.price}" if payload.price > 0 else "",
        f"Description: {payload.description}" if payload.description else "",
    ]
    return "\n".join(part for part in parts if part)
