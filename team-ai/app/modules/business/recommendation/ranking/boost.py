from collections.abc import Mapping
from typing import Any

from app.modules.business.recommendation.ranking.rows import item_feature
from app.modules.business.recommendation.schemas import RecommendedItem

# Online features break ties and add a bounded boost; the model score stays dominant.
_BOOST_WEIGHT = 0.05
_BOOST_CAP = 0.05


def _boost(row: dict[str, Any]) -> float:
    return min(_BOOST_CAP, max(0.0, _BOOST_WEIGHT * item_feature(row, "ctr_7d")[0]))


def apply_online_features(
    items: list[RecommendedItem],
    features: Mapping[str, dict[str, Any]],
    limit: int,
) -> list[RecommendedItem]:
    """Re-rank by ``score + bounded ctr_7d boost``; ties by ctr_7d, then favorites_current.

    A no-op, apart from the cut to ``limit`` (``<= 0`` keeps all), when no candidate has
    features.
    """
    if not features:
        return items[:limit] if limit > 0 else items

    def row(item: RecommendedItem) -> dict[str, Any]:
        return features.get(item.listing_id, {})

    boosted = sorted(
        ((item, item.score + _boost(row(item))) for item in items),
        key=lambda pair: (
            -pair[1],
            -item_feature(row(pair[0]), "ctr_7d")[0],
            -item_feature(row(pair[0]), "favorites_current")[0],
        ),
    )
    if limit > 0:
        boosted = boosted[:limit]
    return [
        RecommendedItem(listing_id=item.listing_id, score=score, rank=rank)
        for rank, (item, score) in enumerate(boosted, start=1)
    ]
