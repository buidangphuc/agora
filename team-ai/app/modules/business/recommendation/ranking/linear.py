from collections.abc import Iterable, Mapping
from typing import Any

from app.modules.business.recommendation.ranking.filter import eligible
from app.modules.business.recommendation.ranking.rows import item_feature, popularity
from app.modules.business.recommendation.schemas import Candidate, RecommendedItem

# team-ai's fixed ranker scored [similarity, category_match, popularity, price, freshness,
# ctr, cvr] with weights [0.30, 0.25, 0.15, 0.05, 0.05, 0.10, 0.10]. A query carries no
# category, and no view provides price, freshness or cvr, so those slots were always 0.0 and
# only these three weights act.
_SIMILARITY_WEIGHT = 0.30
_POPULARITY_WEIGHT = 0.15
_CTR_WEIGHT = 0.10


def _ranked(scored: list[tuple[Candidate, float]], limit: int) -> list[RecommendedItem]:
    cut = scored[:limit] if limit > 0 else scored
    return [
        RecommendedItem(listing_id=cand.listing_id, score=score, rank=rank)
        for rank, (cand, score) in enumerate(cut, start=1)
    ]


def rank_by_similarity(
    candidates: Iterable[Candidate], seed_listing_id: str, limit: int
) -> list[RecommendedItem]:
    """Retrieval score order; ``limit <= 0`` keeps everything."""
    ordered = sorted(
        eligible(candidates, seed_listing_id), key=lambda c: c.score, reverse=True
    )
    return _ranked([(cand, cand.score) for cand in ordered], limit)


def rank_by_fixed_weights(
    candidates: Iterable[Candidate],
    seed_listing_id: str,
    features: Mapping[str, dict[str, Any]],
    nearline: Mapping[str, float],
    limit: int,
) -> list[RecommendedItem]:
    """Retrieval similarity, item popularity and CTR under fixed weights.

    A positive nearline (position-debiased) CTR replaces the item's prior ``ctr_7d``.
    """
    scored: list[tuple[Candidate, float]] = []
    for cand in eligible(candidates, seed_listing_id):
        row = features.get(cand.listing_id, {})
        ctr = item_feature(row, "ctr_7d")[0]
        nearline_ctr = nearline.get(cand.listing_id, 0.0)
        if nearline_ctr > 0:
            ctr = nearline_ctr
        score = (
            cand.score * _SIMILARITY_WEIGHT
            + popularity(row) * _POPULARITY_WEIGHT
            + min(1.0, max(0.0, ctr)) * _CTR_WEIGHT
        )
        scored.append((cand, score))
    scored.sort(key=lambda pair: -pair[1])
    return _ranked(scored, limit)
