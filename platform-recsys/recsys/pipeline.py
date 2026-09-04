"""End-to-end offline batch: warehouse → triples → ALS → Qdrant + Redis.

Orchestrates the seams. The heavy Spark work (read, map, index, fit) stays in
the executors; the collected factor matrices are ranked on the driver
(recsys.recommend) and loaded into the artifact stores. Idempotent per
model_version: re-running the same window reproduces the same artifacts.
"""

from __future__ import annotations

import logging

from . import recommend
from .config import Settings, load_settings
from .interactions import build_triples, index_interactions
from .load import qdrant as qdrant_load
from .load import redis_cache
from .model_version import resolve_model_version
from .spark import build_spark
from .train import train_als
from .warehouse import read_tracking_events

log = logging.getLogger("recsys.pipeline")


def _collect_factors(factors_df, id_col: str):
    """Collect a factors DataFrame to (ids, vectors) on the driver, L2-normalized."""
    ids: list[str] = []
    vectors: list[list[float]] = []
    for row in factors_df.collect():
        ids.append(row[id_col])
        vectors.append(recommend.l2_normalize(list(row["features"])))
    return ids, vectors


def run(settings: Settings | None = None) -> dict:
    """Run the full pipeline. Returns a summary dict of what was produced."""
    settings = settings or load_settings()
    model_version = resolve_model_version(settings)
    log.info("starting ALS batch model_version=%s driver=%s", model_version, settings.warehouse_driver)

    spark = build_spark(settings)
    try:
        events = read_tracking_events(spark, settings)
        triples = build_triples(events, settings)

        # Popularity fallback from the (user,item,weight) triples — summed weight
        # per listing (computed on Spark, small result collected to the driver).
        from pyspark.sql import functions as F  # noqa: PLC0415

        pop_rows = (
            triples.groupBy("listing_id")
            .agg(F.sum("weight").alias("w"))
            .orderBy(F.desc("w"))
            .limit(settings.top_n)
            .collect()
        )
        popular = [(r["listing_id"], float(r["w"])) for r in pop_rows]

        indexed = index_interactions(triples, settings)
        artifacts = train_als(indexed, settings)

        item_ids, item_vecs = _collect_factors(artifacts.item_factors, "listing_id")
        user_ids, user_vecs = _collect_factors(artifacts.user_factors, "user_key")
        log.info("trained factors items=%d users=%d rank=%d", len(item_ids), len(user_ids), artifacts.rank)

        # Precomputed Top-N artifacts (numpy on the driver).
        user_recs = recommend.top_n_for_users(user_ids, user_vecs, item_ids, item_vecs, settings.top_n)
        item_recs = recommend.similar_items(item_ids, item_vecs, settings.top_n)

        qdrant_counts = qdrant_load.load_vectors(
            settings,
            model_version,
            item_rows=zip(item_ids, item_vecs, strict=False),
            user_rows=zip(user_ids, user_vecs, strict=False),
        )
        cache_counts = redis_cache.load_cache(settings, model_version, user_recs, item_recs, popular)

        summary = {
            "model_version": model_version,
            "qdrant": qdrant_counts,
            "cache": cache_counts,
            "items": len(item_ids),
            "users": len(user_ids),
            "popular": len(popular),
        }
        log.info("batch complete %s", summary)
        return summary
    finally:
        spark.stop()
