"""Fit Spark MLlib ALS (implicit feedback) → item factors + user factors.

All hyperparameters come from config (they are tunable defaults, not final tuned
values). Outputs are relabelled back to the original listing/user ids via the
reverse maps produced in interactions.py.
"""

from __future__ import annotations

from dataclasses import dataclass

from .config import Settings
from .interactions import IndexedInteractions


@dataclass
class AlsArtifacts:
    """Trained factors, labelled with the original ids.

    item_factors: DataFrame[listing_id:string, features:array<float>]
    user_factors: DataFrame[user_key:string,  features:array<float>]
    rank:         vector dimension (== ALS_RANK)
    """

    item_factors: object
    user_factors: object
    rank: int


def train_als(indexed: IndexedInteractions, settings: Settings) -> AlsArtifacts:
    from pyspark.ml.recommendation import ALS  # noqa: PLC0415

    als = ALS(
        userCol="user_index",
        itemCol="item_index",
        ratingCol="weight",
        implicitPrefs=True,
        rank=settings.als_rank,
        regParam=settings.als_reg_param,
        alpha=settings.als_alpha,
        maxIter=settings.als_max_iter,
        coldStartStrategy="drop",
        nonnegative=False,
        seed=42,
    )
    model = als.fit(indexed.triples)

    # model.itemFactors: [id:int, features:array<float>]; relabel id → listing_id.
    item_factors = model.itemFactors.join(
        indexed.item_labels, model.itemFactors["id"] == indexed.item_labels["item_index"], "inner"
    ).select(indexed.item_labels["listing_id"], model.itemFactors["features"])
    user_factors = model.userFactors.join(
        indexed.user_labels, model.userFactors["id"] == indexed.user_labels["user_index"], "inner"
    ).select(indexed.user_labels["user_key"], model.userFactors["features"])
    return AlsArtifacts(item_factors=item_factors, user_factors=user_factors, rank=settings.als_rank)
