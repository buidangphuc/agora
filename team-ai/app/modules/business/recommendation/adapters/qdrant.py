"""Candidate retrieval from the Qdrant collection platform-recsys trains.

Read-only: this service never creates or writes the collection."""

import asyncio
from collections.abc import Sequence
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

    async def similar_batch(
        self, listing_ids: Sequence[str], *, top_k: int, generation: str | None
    ) -> dict[str, list[Candidate]]:
        ids = list(dict.fromkeys(listing_ids))
        if not ids:
            return {}
        collection = generation_collection(self._collection, generation)
        requests = [
            models.QueryRequest(
                query=models.RecommendQuery(
                    recommend=models.RecommendInput(positive=[point_id(lid)])
                ),
                limit=top_k,
                with_payload=True,
            )
            for lid in ids
        ]
        try:
            async with asyncio.timeout(self._timeout_s):
                responses = await self._client.query_batch_points(
                    collection_name=collection, requests=requests
                )
        except TimeoutError:
            logger.warning(
                "recs.retrieve_batch.timeout n={} budget_s={}",
                len(ids),
                self._timeout_s,
            )
            return {lid: [] for lid in ids}
        except Exception as exc:
            logger.warning("recs.retrieve_batch.failed n={} err={}", len(ids), exc)
            return {lid: [] for lid in ids}
        out: dict[str, list[Candidate]] = {lid: [] for lid in ids}
        for lid, response in zip(ids, responses, strict=False):
            out[lid] = [c for hit in response.points if (c := _candidate(hit))]
        return out

    async def categories(
        self, listing_ids: Sequence[str], *, generation: str | None
    ) -> dict[str, str]:
        """Category id of each listing that has one, within the retrieve budget."""
        if not listing_ids:
            return {}
        collection = generation_collection(self._collection, generation)
        try:
            async with asyncio.timeout(self._timeout_s):
                points = await self._client.retrieve(
                    collection_name=collection,
                    ids=[point_id(lid) for lid in listing_ids],
                    with_payload=True,
                    with_vectors=False,
                )
        except Exception as exc:
            logger.warning("recs.categories.failed err={}", exc)
            return {}
        out: dict[str, str] = {}
        for point in points:
            payload = point.payload or {}
            if payload.get("listing_id") and payload.get("category_id"):
                out[str(payload["listing_id"])] = str(payload["category_id"])
        return out

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
        seller_id=str(payload.get("seller_id") or ""),
        category_id=str(payload.get("category_id") or ""),
        visible=bool(payload.get("visible", False)),
    )
