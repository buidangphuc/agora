import asyncio
from contextlib import AsyncExitStack

import pytest

from app.bootstrap.addons import listing_indexer as listing_indexer_module
from app.bootstrap.addons.base import default_resource_addons
from app.bootstrap.addons.listing_indexer import ListingIndexerAddon
from app.bootstrap.resources import ApplicationResources
from app.modules.messaging.kafka.consumer import supervise
from tests.factories import build_test_settings


class _Rag:
    async def index(self, documents):
        return {}

    async def delete(self, document_id: str) -> None: ...


class _Idle:
    """Source and sink in one: blocks in ``getone`` until cancelled."""

    def __init__(self) -> None:
        self.started = asyncio.Event()
        self.stopped = False

    async def start(self) -> None:
        self.started.set()

    async def stop(self) -> None:
        self.stopped = True

    async def getone(self):
        await asyncio.Event().wait()
        raise AssertionError("unreachable")

    async def commit(self, record) -> None: ...

    async def send(self, record, *, error_type: str) -> None: ...


@pytest.fixture(autouse=True)
def real_supervisor(monkeypatch: pytest.MonkeyPatch) -> None:
    """The suite-wide idle stand-in is for app-level tests; these use the real one."""
    monkeypatch.setattr(listing_indexer_module, "supervise", supervise)


def test_gated_by_rag_enabled() -> None:
    addon = ListingIndexerAddon()

    assert addon.is_enabled(build_test_settings()) is False
    assert (
        addon.is_enabled(
            build_test_settings(RAG_ENABLED=True, RAG_EMBED_SERVER_URL="http://embed")
        )
        is True
    )


def test_registered_after_rag() -> None:
    names = [a.name for a in default_resource_addons()]

    assert names.index("listing_indexer") > names.index("rag")


async def test_open_starts_supervisor_and_close_cancels_it() -> None:
    idle = _Idle()
    built: list[dict[str, str]] = []

    def build_source(*, brokers: str, topic: str, group: str) -> _Idle:
        built.append({"brokers": brokers, "topic": topic, "group": group})
        return idle

    settings = build_test_settings(
        RAG_ENABLED=True,
        RAG_EMBED_SERVER_URL="http://embed",
        KAFKA_BROKERS="b:9092",
        LISTING_INDEXER_GROUP="g",
    )
    resources = ApplicationResources()
    resources.rag_service = _Rag()  # type: ignore[assignment]
    addon = ListingIndexerAddon(build_source=build_source, build_dlq=lambda **_: idle)

    async with AsyncExitStack() as stack:
        await addon.open(resources, settings, stack)
        await asyncio.wait_for(idle.started.wait(), 1)
        assert built == [{"brokers": "b:9092", "topic": "listing.events", "group": "g"}]

    assert idle.stopped is True
