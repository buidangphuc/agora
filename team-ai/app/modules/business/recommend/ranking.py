"""Stage-2 ranking + business-rule filtering (pure, no I/O).

Applied uniformly to candidates from every source (Redis cache, Qdrant ANN,
popularity fallback): drop the seed listing, drop out-of-stock, de-duplicate by
listing id (keeping the highest-scoring occurrence), sort by score descending,
assign a 1-based rank, and truncate to the result Top-K.
"""

from __future__ import annotations

from collections.abc import Iterable

from app.modules.business.recommend.schemas import (
    Candidate,
    RecommendedItem,
    RecommendQuery,
)


def rank_and_filter(
    candidates: Iterable[Candidate],
    query: RecommendQuery,
    result_top_k: int,
) -> list[RecommendedItem]:
    seed = query.seed_listing_id
    best: dict[str, Candidate] = {}
    for cand in candidates:
        if not cand.listing_id:
            continue
        if cand.listing_id == seed:
            continue
        if not cand.in_stock:
            continue
        existing = best.get(cand.listing_id)
        if existing is None or cand.score > existing.score:
            best[cand.listing_id] = cand

    ordered = sorted(best.values(), key=lambda c: c.score, reverse=True)
    limit = result_top_k if result_top_k > 0 else len(ordered)
    return [
        RecommendedItem(listing_id=c.listing_id, score=c.score, rank=rank)
        for rank, c in enumerate(ordered[:limit], start=1)
    ]
