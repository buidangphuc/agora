"""Candidate-retrieval backends behind one interface (mirrors the RAG_BACKEND seam).

``memory`` returns deterministic in-process fixtures for offline dev / e2e and
unit tests — no real infra. ``qdrant`` reads the collection the ALS training job
populates (team-ai never writes it); ``qdrant_client`` is imported lazily so this
module (and the whole recommend pipeline) imports without the [ai] extra.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Protocol

from app.modules.business.recommend.schemas import Candidate

if TYPE_CHECKING:
    from app.core.config import Settings


class RetrievalBackend(Protocol):
    async def retrieve_similar(
        self, seed_listing_id: str, *, top_k: int
    ) -> list[Candidate]:
        """ANN candidates near the seed listing's vector (empty if no seed)."""
        ...

    async def popular(self, *, top_k: int) -> list[Candidate]:
        """Popularity fallback so a Recommend call is never empty."""
        ...

    async def collection_ok(self) -> bool:
        """Startup contract check: collection name/dim/metric match config."""
        ...


class MemoryRetrievalBackend:
    """Deterministic fixture backend — the offline/dev/e2e path.

    Similar-item candidates are derived deterministically from the seed id so
    tests get a stable, non-empty result without Qdrant.
    """

    def __init__(
        self,
        *,
        catalog: list[Candidate] | None = None,
        similar: dict[str, list[Candidate]] | None = None,
        popular: list[Candidate] | None = None,
    ) -> None:
        self._catalog = catalog or _DEFAULT_CATALOG
        self._similar = similar or {}
        self._popular = popular if popular is not None else list(self._catalog)

    async def retrieve_similar(
        self, seed_listing_id: str, *, top_k: int
    ) -> list[Candidate]:
        if not seed_listing_id:
            return []
        if seed_listing_id in self._similar:
            return list(self._similar[seed_listing_id][:top_k])
        # Deterministic default: everything in the catalog except the seed,
        # scored by a stable pseudo-similarity so ordering is reproducible.
        out = [
            Candidate(
                listing_id=c.listing_id,
                score=round(
                    1.0 / (1 + abs(hash((seed_listing_id, c.listing_id)) % 97)), 6
                ),
                in_stock=c.in_stock,
                category_id=c.category_id,
                seller_id=c.seller_id,
            )
            for c in self._catalog
            if c.listing_id != seed_listing_id
        ]
        out.sort(key=lambda c: c.score, reverse=True)
        return out[:top_k]

    async def popular(self, *, top_k: int) -> list[Candidate]:
        return list(self._popular[:top_k])

    async def collection_ok(self) -> bool:
        return True


class QdrantRetrievalBackend:
    """Reads the ALS item-vector collection populated by build-als-training-job.

    Read-only: team-ai never creates or writes the collection. ``qdrant_client``
    is imported lazily inside the methods so importing this module stays free of
    the [ai] extra.
    """

    def __init__(
        self,
        *,
        url: str,
        collection: str,
        vector_dim: int,
        distance: str,
    ) -> None:
        self._url = url
        self._collection = collection
        self._vector_dim = vector_dim
        self._distance = distance
        self._client = None  # lazily built

    def _get_client(self):  # pragma: no cover - requires qdrant_client + live infra
        if self._client is None:
            from qdrant_client import QdrantClient

            self._client = QdrantClient(url=self._url)
        return self._client

    async def collection_ok(self) -> bool:  # pragma: no cover - needs live Qdrant
        import asyncio

        def _check() -> bool:
            client = self._get_client()
            info = client.get_collection(self._collection)
            params = info.config.params.vectors
            size = getattr(params, "size", None)
            distance = getattr(params, "distance", None)
            if size is not None and int(size) != self._vector_dim:
                return False
            return not (
                distance is not None and str(distance).lower() != self._distance.lower()
            )

        try:
            return await asyncio.to_thread(_check)
        except Exception:
            return False

    async def retrieve_similar(
        self, seed_listing_id: str, *, top_k: int
    ) -> list[Candidate]:  # pragma: no cover - needs live Qdrant
        import asyncio

        if not seed_listing_id:
            return []

        def _query() -> list[Candidate]:
            client = self._get_client()
            hits = client.recommend(
                collection_name=self._collection,
                positive=[seed_listing_id],
                limit=top_k,
                with_payload=True,
            )
            return [_hit_to_candidate(h) for h in hits]

        return await asyncio.to_thread(_query)

    async def popular(self, *, top_k: int) -> list[Candidate]:  # pragma: no cover
        import asyncio

        def _scroll() -> list[Candidate]:
            client = self._get_client()
            points, _ = client.scroll(
                collection_name=self._collection,
                limit=top_k,
                with_payload=True,
            )
            return [_hit_to_candidate(p) for p in points]

        return await asyncio.to_thread(_scroll)


def _hit_to_candidate(hit: object) -> Candidate:  # pragma: no cover - live Qdrant
    payload = getattr(hit, "payload", None) or {}
    return Candidate(
        listing_id=str(getattr(hit, "id", payload.get("listing_id", ""))),
        score=float(getattr(hit, "score", 0.0) or 0.0),
        in_stock=bool(payload.get("in_stock", True)),
        category_id=str(payload.get("category_id", "")),
        seller_id=str(payload.get("seller_id", "")),
    )


def build_backend(settings: Settings) -> RetrievalBackend:
    if settings.RECS_BACKEND == "memory":
        return MemoryRetrievalBackend()
    if settings.RECS_BACKEND == "qdrant":
        return QdrantRetrievalBackend(
            url=settings.RECS_QDRANT_URL,
            collection=settings.RECS_QDRANT_COLLECTION,
            vector_dim=settings.RECS_VECTOR_DIM,
            distance=settings.RECS_QDRANT_DISTANCE,
        )
    raise RuntimeError(
        f"RECS_BACKEND={settings.RECS_BACKEND!r} not supported "
        "(use 'memory' or 'qdrant')."
    )


_DEFAULT_CATALOG: list[Candidate] = [
    Candidate(listing_id="listing-1", score=0.99, category_id="cat-a", seller_id="s1"),
    Candidate(listing_id="listing-2", score=0.95, category_id="cat-a", seller_id="s2"),
    Candidate(listing_id="listing-3", score=0.90, category_id="cat-b", seller_id="s1"),
    Candidate(listing_id="listing-4", score=0.85, category_id="cat-b", seller_id="s3"),
    Candidate(listing_id="listing-5", score=0.80, category_id="cat-c", seller_id="s2"),
]
