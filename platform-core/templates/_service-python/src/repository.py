"""Persistence boundary.

A :class:`Protocol` defines the storage contract the service depends on; the
servicer talks to the Protocol, never to a concrete driver. The in-memory
implementation lets the service run and be tested with zero infrastructure.

Rule 3: this service owns its own storage. Cross-service data comes over gRPC,
never by reaching into another service's database.

Wiring a real backend (do NOT add these deps until you use them):
  * Qdrant (search/RAG for team-ai) -> ``QdrantSearchRepository`` using
    ``qdrant-client`` against ``settings.QDRANT_URL``.
  * Postgres -> ``PostgresRepository`` using ``asyncpg`` / SQLAlchemy against
    ``settings.DATABASE_URL`` (managed by alembic migrations).
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol, runtime_checkable


@dataclass(frozen=True, slots=True)
class SearchHit:
    """Plain result row (mirror of ``platform.search.v1.SearchHit``)."""

    listing_id: str
    score: float


@runtime_checkable
class SearchRepository(Protocol):
    """Read side for listing search. Implemented by Qdrant in production."""

    async def search(
        self, query: str, *, limit: int = 10, filters: dict[str, str] | None = None
    ) -> list[SearchHit]:
        """Return ranked hits for ``query`` (optionally structured-filtered)."""
        ...


class InMemorySearchRepository:
    """Zero-dependency stub. Returns deterministic mock hits.

    Swap for a Qdrant-backed implementation of :class:`SearchRepository`; the
    servicer depends only on the Protocol, so no call-site changes are needed.
    """

    def __init__(self, seed: list[SearchHit] | None = None) -> None:
        self._seed = seed or [
            SearchHit(listing_id="listing-1", score=0.99),
            SearchHit(listing_id="listing-2", score=0.87),
        ]

    async def search(
        self, query: str, *, limit: int = 10, filters: dict[str, str] | None = None
    ) -> list[SearchHit]:
        # Mock: ignores the query vector; a real impl embeds `query` and queries
        # Qdrant with `filters` as payload constraints.
        return self._seed[:limit]
