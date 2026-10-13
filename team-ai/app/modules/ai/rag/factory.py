from __future__ import annotations

from collections.abc import Awaitable, Callable
from typing import TYPE_CHECKING

from app.core.config import Settings
from app.core.redaction import RedactionPolicy
from app.core.resilience import TimeoutPolicy
from app.modules.ai._deps import require_llama_index

if TYPE_CHECKING:
    from llama_index.core.base.embeddings.base import BaseEmbedding

    from app.modules.ai.rag.service import KnowledgeRetrievalService

# LlamaIndex lives behind the [ai] extra: everything here imports it lazily so
# registering RagAddon (which happens on every boot) never pulls the package —
# only actually opening the addon (RAG_ENABLED=true) does.
# Tests replace ``build_rag_service`` (as imported by ``app.bootstrap.addons.rag``)
# with an in-memory store and mock embeddings.


def build_embed_model(
    settings: Settings,
) -> tuple[BaseEmbedding, Callable[[], Awaitable[None]]]:
    """The model_server embedding model plus the close of the clients it owns."""
    require_llama_index()
    import httpx

    from app.modules.ai.rag.embeddings import ModelServerEmbedding

    url, timeout = settings.RAG_EMBED_SERVER_URL, settings.RAG_EMBED_TIMEOUT_SECONDS
    embedding = ModelServerEmbedding(
        sync_client=httpx.Client(base_url=url, timeout=timeout),
        async_client=httpx.AsyncClient(base_url=url, timeout=timeout),
        path=settings.RAG_EMBED_SERVER_PATH,
        embed_dim=settings.RAG_EMBED_DIM,
    )
    return embedding, embedding.aclose


def build_rag_service(settings: Settings) -> KnowledgeRetrievalService:
    """The Qdrant-backed retrieval service with model_server embeddings."""
    require_llama_index()

    from app.modules.ai.rag.qdrant import connect_qdrant_backend
    from app.modules.ai.rag.service import (
        KnowledgeRetrievalService,
        build_rag_node_parser,
    )

    backend = connect_qdrant_backend(
        url=settings.RAG_QDRANT_URL, collection=settings.RAG_QDRANT_COLLECTION
    )
    embed_model, embed_close = build_embed_model(settings)

    async def close() -> None:
        await backend.aclose()
        await embed_close()

    return KnowledgeRetrievalService(
        embed_model=embed_model,
        node_parser=build_rag_node_parser(
            chunk_size=settings.RAG_CHUNK_SIZE,
            chunk_overlap=settings.RAG_CHUNK_OVERLAP,
        ),
        redaction_policy=RedactionPolicy(mode="redacted"),
        storage_context=backend.storage_context,
        default_top_k=settings.RAG_DEFAULT_TOP_K,
        retrieve_timeout=TimeoutPolicy(
            timeout_seconds=settings.RAG_RETRIEVE_TIMEOUT_SECONDS
        ),
        collection_exists=backend.collection_exists,
        on_close=close,
    )
