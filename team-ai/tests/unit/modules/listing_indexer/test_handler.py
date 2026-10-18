from collections.abc import Sequence

import pytest
from llama_index.core import Document

from app.modules.messaging.listing_indexer.decode import (
    ListingEventPayload,
    decode_listing_event,
)
from app.modules.messaging.listing_indexer.handler import ListingEventIndexer
from app.transport.grpc._pb.platform.listing.v1 import listing_pb2
from tests.unit.modules.listing_indexer.test_decode import envelope


class FakeRag:
    def __init__(self) -> None:
        self.calls: list[tuple[str, str]] = []
        self.documents: list[Document] = []

    async def index(self, documents: Sequence[Document]) -> dict[str, int]:
        self.calls.append(("index", documents[0].id_))
        self.documents.extend(documents)
        return {"chunk_count": len(documents)}

    async def delete(self, document_id: str) -> None:
        self.calls.append(("delete", document_id))


def payload(listing_id: str = "l-1", **kwargs: object) -> ListingEventPayload:
    decoded = decode_listing_event(envelope(listing_id, **kwargs))  # type: ignore[arg-type]
    assert decoded is not None
    return decoded


async def test_document_text_and_metadata_verbatim() -> None:
    rag = FakeRag()
    result = await ListingEventIndexer(rag).handle(payload())
    assert result == "indexed"
    (document,) = rag.documents
    assert document.id_ == "l-1"
    assert (
        document.text
        == "Product: Tai nghe\nCategory: cat-electronics\nPrice: 1500000.0"
    )
    assert document.metadata == {
        "listing_id": "l-1",
        "title": "Tai nghe",
        "category": "cat-electronics",
        "price": 1_500_000.0,
        "currency": "VND",
        "seller_id": "seller-1",
        "status": "PUBLISHED",
    }


async def test_text_includes_description_and_skips_zero_price() -> None:
    rag = FakeRag()
    await ListingEventIndexer(rag).handle(payload(description="Bluetooth", price=0))
    assert rag.documents[0].text == (
        "Product: Tai nghe\nCategory: cat-electronics\nDescription: Bluetooth"
    )


async def test_unknown_status_defaults_to_published_metadata() -> None:
    rag = FakeRag()
    await ListingEventIndexer(rag).handle(
        payload(status=listing_pb2.LISTING_STATUS_UNSPECIFIED)
    )
    assert rag.documents[0].metadata["status"] == "PUBLISHED"


async def test_update_deletes_before_indexing() -> None:
    rag = FakeRag()
    await ListingEventIndexer(rag).handle(
        payload(change=listing_pb2.CHANGE_TYPE_UPDATED, description="short")
    )
    assert rag.calls == [("delete", "l-1"), ("index", "l-1")]
    assert len(rag.documents) == 1


async def test_deleted_action_only_deletes() -> None:
    rag = FakeRag()
    result = await ListingEventIndexer(rag).handle(
        payload(change=listing_pb2.CHANGE_TYPE_DELETED)
    )
    assert (result, rag.calls) == ("deleted", [("delete", "l-1")])


async def test_unpublish_status_only_deletes() -> None:
    rag = FakeRag()
    indexer = ListingEventIndexer(rag)
    for i, status in enumerate(
        (listing_pb2.LISTING_STATUS_DRAFT, listing_pb2.LISTING_STATUS_REJECTED)
    ):
        result = await indexer.handle(
            payload(
                change=listing_pb2.CHANGE_TYPE_UPDATED,
                status=status,
                event_id=f"e{i}",
                seconds=1_000 + i,
            )
        )
        assert result == "deleted"
    assert rag.calls == [("delete", "l-1")] * 2


async def test_unspecified_action_is_ignored() -> None:
    rag = FakeRag()
    result = await ListingEventIndexer(rag).handle(
        payload(change=listing_pb2.CHANGE_TYPE_UNSPECIFIED)
    )
    assert (result, rag.calls) == ("ignored", [])


async def test_repeated_event_id_is_duplicate() -> None:
    rag = FakeRag()
    indexer = ListingEventIndexer(rag)
    assert await indexer.handle(payload()) == "indexed"
    assert await indexer.handle(payload()) == "duplicate"
    assert len(rag.documents) == 1


async def test_older_event_is_stale() -> None:
    rag = FakeRag()
    indexer = ListingEventIndexer(rag)
    await indexer.handle(payload(event_id="new", seconds=2_000))
    result = await indexer.handle(
        payload(
            change=listing_pb2.CHANGE_TYPE_UPDATED,
            status=listing_pb2.LISTING_STATUS_HELD,
            event_id="old",
            seconds=1_000,
        )
    )
    assert result == "stale"
    assert rag.calls == [("delete", "l-1"), ("index", "l-1")]


async def test_failed_event_is_not_recorded_so_retry_runs() -> None:
    class FlakyRag(FakeRag):
        failures = 1

        async def index(self, documents: Sequence[Document]) -> dict[str, int]:
            if self.failures:
                self.failures -= 1
                raise ConnectionError("embed server down")
            return await super().index(documents)

    rag = FlakyRag()
    indexer = ListingEventIndexer(rag)
    with pytest.raises(ConnectionError):
        await indexer.handle(payload())
    assert await indexer.handle(payload()) == "indexed"


@pytest.mark.parametrize(
    "status",
    [
        listing_pb2.LISTING_STATUS_PENDING_REVIEW,
        listing_pb2.LISTING_STATUS_BANNED,
        listing_pb2.LISTING_STATUS_HELD,
    ],
)
async def test_non_published_statuses_delete_from_index(status: int) -> None:
    rag = FakeRag()
    result = await ListingEventIndexer(rag).handle(
        payload(change=listing_pb2.CHANGE_TYPE_UPDATED, status=status)
    )
    assert result == "deleted"
    assert rag.calls == [("delete", "l-1")]
    assert rag.documents == []


async def test_held_then_published_is_reindexed() -> None:
    rag = FakeRag()
    indexer = ListingEventIndexer(rag)
    first = await indexer.handle(
        payload(
            change=listing_pb2.CHANGE_TYPE_UPDATED,
            status=listing_pb2.LISTING_STATUS_HELD,
            event_id="e1",
            seconds=1_000,
        )
    )
    assert first == "deleted"
    second = await indexer.handle(
        payload(
            change=listing_pb2.CHANGE_TYPE_UPDATED,
            status=listing_pb2.LISTING_STATUS_PUBLISHED,
            event_id="e2",
            seconds=2_000,
        )
    )
    assert second == "indexed"
    (document,) = rag.documents
    assert document.metadata["status"] == "PUBLISHED"


async def test_unknown_status_deletes() -> None:
    rag = FakeRag()
    result = await ListingEventIndexer(rag).handle(
        payload(change=listing_pb2.CHANGE_TYPE_UPDATED, status=99)
    )
    assert result == "deleted"
    assert rag.calls == [("delete", "l-1")]
    assert rag.documents == []


async def test_unspecified_envelope_is_indexed() -> None:
    rag = FakeRag()
    decoded = payload(status=0)
    assert decoded.status == ""
    assert await ListingEventIndexer(rag).handle(decoded) == "indexed"
    (document,) = rag.documents
    assert document.metadata["status"] == "PUBLISHED"
