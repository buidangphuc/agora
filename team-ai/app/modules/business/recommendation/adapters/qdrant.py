"""Candidate retrieval from the Qdrant collection platform-recsys trains.

Read-only: this service never creates or writes the collection."""

import asyncio
from typing import Any

from loguru import logger
from qdrant_client import AsyncQdrantClient, models

from app.modules.business.recommendation.contract.qdrant import (
    generation_collection,
    point_id,
)
from app.modules.business.recommendation.schemas import Candidate


class QdrantCandidateSource:
    def __init__(
        self,
        client: AsyncQdrantClient,
        *,
        collection: str,
        vector_dim: int,
        distance: str,
        timeout_s: float,
    ) -> None:
        self._client = client
        self._collection = collection
        self._vector_dim = vector_dim
        self._distance = distance
        self._timeout_s = timeout_s

    async def similar(
        self, seed_listing_id: str, *, top_k: int, generation: str | None
    ) -> list[Candidate]:
        """Neighbours of the seed's vector within the retrieve budget.

        The query is a task of this request: on timeout it is cancelled, not abandoned, so
        nothing keeps running (or holding a worker thread) after the ladder moves on. Any
        failure yields no candidates.
        """
        collection = generation_collection(self._collection, generation)
        try:
            async with asyncio.timeout(self._timeout_s):
                # The collection is keyed by the producer's uuid5 point id, not the raw
                # listing id; Qdrant leaves the seed itself out of the results.
                response = await self._client.query_points(
                    collection_name=collection,
                    query=models.RecommendQuery(
                        recommend=models.RecommendInput(
                            positive=[point_id(seed_listing_id)]
                        )
                    ),
                    limit=top_k,
                    with_payload=True,
                )
        except TimeoutError:
            logger.warning(
                "recs.retrieve.timeout seed={} budget_s={}",
                seed_listing_id,
                self._timeout_s,
            )
            return []
        except Exception as exc:
            logger.warning("recs.retrieve.failed seed={} err={}", seed_listing_id, exc)
            return []
        return [c for hit in response.points if (c := _candidate(hit))]

    async def ready(self, generation: str | None) -> bool:
        """The collection exists with the configured vector size and distance."""
        collection = generation_collection(self._collection, generation)
        try:
            info = await self._client.get_collection(collection_name=collection)
        except Exception as exc:
            logger.warning(
                "recs.collection.unreadable collection={} err={}", collection, exc
            )
            return False
        vectors = info.config.params.vectors
        # platform-recsys writes one unnamed vector per point.
        ok = (
            isinstance(vectors, models.VectorParams)
            and vectors.size == self._vector_dim
            and vectors.distance.value.lower() == self._distance.lower()
        )
        if not ok:
            logger.error(
                "recs.collection.mismatch collection={} dim={} distance={} found={}",
                collection,
                self._vector_dim,
                self._distance,
                vectors,
            )
        return ok


def _candidate(hit: Any) -> Candidate | None:
    # The candidate id is the payload listing id; the point id is a uuid5 of it.
    payload = hit.payload or {}
    listing_id = payload.get("listing_id")
    if not listing_id:
        return None
    return Candidate(
        listing_id=str(listing_id),
        score=float(hit.score),
        in_stock=bool(payload.get("in_stock", True)),
    )
