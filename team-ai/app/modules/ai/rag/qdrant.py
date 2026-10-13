"""Qdrant vector store for RAG (llama-index's ``QdrantVectorStore``)."""

from __future__ import annotations

from dataclasses import dataclass

from llama_index.core import StorageContext
from llama_index.vector_stores.qdrant import QdrantVectorStore
from qdrant_client import AsyncQdrantClient, QdrantClient


@dataclass(frozen=True, slots=True)
class QdrantBackend:
    """The store plus what the service needs around it.

    Indexing is synchronous (``insert_nodes``) and retrieval async (``aretrieve``
    raises without ``aclient``), so both clients are owned and closed together.
    """

    client: QdrantClient
    aclient: AsyncQdrantClient
    collection: str
    storage_context: StorageContext

    async def collection_exists(self) -> bool:
        """Qdrant creates the collection on first write; before that, search is empty."""
        return await self.aclient.collection_exists(self.collection)

    async def aclose(self) -> None:
        await self.aclient.close()
        self.client.close()


def build_qdrant_backend(
    *, client: QdrantClient, aclient: AsyncQdrantClient, collection: str
) -> QdrantBackend:
    store = QdrantVectorStore(
        client=client, aclient=aclient, collection_name=collection
    )
    return QdrantBackend(
        client=client,
        aclient=aclient,
        collection=collection,
        storage_context=StorageContext.from_defaults(vector_store=store),
    )


def connect_qdrant_backend(*, url: str, collection: str) -> QdrantBackend:
    return build_qdrant_backend(
        client=QdrantClient(url=url),
        aclient=AsyncQdrantClient(url=url),
        collection=collection,
    )
