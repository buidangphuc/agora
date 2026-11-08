import json
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Any

from fakeredis import aioredis

from app.modules.business.recommendation.adapters.memory import (
    MemoryCandidateSource,
)
from app.modules.business.recommendation.adapters.redis_cache import RedisServingCache
from app.modules.business.recommendation.contract.keys import ServingKeys
from app.modules.business.recommendation.placements import (
    Placements,
    load_placements,
)
from app.modules.business.recommendation.ranking.artifact import TrainedRankerLoader
from app.modules.business.recommendation.schemas import Candidate
from app.modules.business.recommendation.service import RecommendationService


class Source(MemoryCandidateSource):
    def __init__(
        self,
        similar: Mapping[str, Sequence[Candidate]] | None = None,
        categories: Mapping[str, str] | None = None,
    ) -> None:
        super().__init__(similar or {})
        self.category_of = dict(categories or {})
        self.similar_calls: list[str] = []
        self.batch_calls: list[list[str]] = []

    async def similar(
        self, seed_listing_id: str, *, top_k: int, generation: str | None
    ) -> list[Candidate]:
        self.similar_calls.append(seed_listing_id)
        return await super().similar(
            seed_listing_id, top_k=top_k, generation=generation
        )

    async def similar_batch(
        self, listing_ids: Sequence[str], *, top_k: int, generation: str | None
    ) -> dict[str, list[Candidate]]:
        self.batch_calls.append(list(listing_ids))
        return {
            lid: await MemoryCandidateSource.similar(
                self, lid, top_k=top_k, generation=generation
            )
            for lid in listing_ids
        }

    async def categories(
        self, listing_ids: Sequence[str], *, generation: str | None
    ) -> dict[str, str]:
        return {
            lid: self.category_of[lid] for lid in listing_ids if lid in self.category_of
        }


def cand(
    listing_id: str,
    score: float = 1.0,
    *,
    seller_id: str = "",
    category_id: str = "",
    visible: bool = True,
    in_stock: bool = True,
) -> Candidate:
    return Candidate(
        listing_id=listing_id,
        score=score,
        in_stock=in_stock,
        seller_id=seller_id,
        category_id=category_id,
        visible=visible,
    )


def popular_entry(
    listing_id: str,
    score: float = 1.0,
    *,
    category_id: str | None = None,
    seller_id: str | None = None,
    visible: bool | None = True,
) -> dict[str, Any]:
    entry: dict[str, Any] = {"listing_id": listing_id, "score": score}
    if category_id is not None:
        entry["category_id"] = category_id
    if seller_id is not None:
        entry["seller_id"] = seller_id
    if visible is not None:
        entry["visible"] = visible
    return entry


async def seed_popular(redis: Any, entries: list[dict[str, Any]]) -> None:
    await redis.set("recs:v1:popular", json.dumps(entries))


def write_placements(tmp_path: Path, body: str) -> Placements:
    path = tmp_path / "placements.yaml"
    if "  home_feed:" not in body:
        body += (
            "  home_feed:\n    candidate_ladder:\n      - strategy: global_popular\n"
        )
    path.write_text("version: '1.0'\nplacements:\n" + body, encoding="utf-8")
    return load_placements(path)


def build_service(
    source: Any = None,
    redis: Any = None,
    placements: Placements | None = None,
) -> RecommendationService:
    redis = aioredis.FakeRedis(decode_responses=True) if redis is None else redis
    cache = RedisServingCache(redis, ServingKeys("recs", "v1"))
    return RecommendationService(
        source=Source() if source is None else source,
        cache=cache,
        placements=placements or load_placements(),
        popularity_store=None,
        attribute_store=None,
        nearline=None,
        ranker_loader=TrainedRankerLoader(cache),
        candidate_top_k=100,
        result_top_k=10,
        model_version="test-v1",
    )
