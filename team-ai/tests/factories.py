import hashlib
from collections.abc import AsyncIterator
from contextlib import AsyncExitStack, asynccontextmanager

from fastapi import FastAPI
from httpx import ASGITransport, AsyncClient

from app.core.config import Settings
from app.core.redaction import RedactionPolicy
from app.modules.ai.rag.service import (
    KnowledgeRetrievalService,
    build_rag_node_parser,
)
from app.modules.platform.rate_limit.service import InMemoryRateLimiter


def sha256_hex(token: str) -> str:
    return hashlib.sha256(token.encode()).hexdigest()


def api_key_entry(subject: str, token: str, *scopes: str) -> str:
    """One ``AUTH_API_KEYS`` entry: ``subject:scope|scope:sha256hex``."""
    return f"{subject}:{'|'.join(scopes)}:{sha256_hex(token)}"


def build_test_settings(**overrides: object) -> Settings:
    values: dict[str, object] = {
        "_env_file": None,
        "ENVIRONMENT": "test",
        "REDIS_HOST": "localhost",
        "REDIS_PORT": 6379,
        "REDIS_PASSWORD": "",  # pragma: allowlist secret
        "REDIS_DATABASE": 0,
        "GRPC_PORT": 0,
        "AUTH_API_KEYS": api_key_entry("test-user", "test-token", "admin", "developer"),
    }
    values.update(overrides)
    return Settings(**values)


@asynccontextmanager
async def api_client_for(app: FastAPI) -> AsyncIterator[AsyncClient]:
    """Client for an app with its lifespan running, as in production."""
    async with app.router.lifespan_context(app):
        async with AsyncClient(
            transport=ASGITransport(app=app),
            base_url="http://test",
        ) as client:
            yield client


async def open_addon(addon, resources, settings: Settings) -> AsyncExitStack:
    """Open one addon outside the app; ``await stack.aclose()`` closes it."""
    stack = AsyncExitStack()
    await addon.open(resources, settings, stack)
    return stack


def build_memory_rag_service(settings: Settings) -> KnowledgeRetrievalService:
    """The RAG test seam: in-memory vector store and mock embeddings."""
    from llama_index.core.embeddings import MockEmbedding

    return KnowledgeRetrievalService(
        embed_model=MockEmbedding(embed_dim=16),
        node_parser=build_rag_node_parser(
            chunk_size=settings.RAG_CHUNK_SIZE,
            chunk_overlap=settings.RAG_CHUNK_OVERLAP,
        ),
        redaction_policy=RedactionPolicy(mode="redacted"),
        default_top_k=settings.RAG_DEFAULT_TOP_K,
    )


def allow_all_limiter() -> InMemoryRateLimiter:
    """A gRPC rate limiter that never throttles (the server always has one)."""
    return InMemoryRateLimiter(limit=10**9, window_seconds=60)
