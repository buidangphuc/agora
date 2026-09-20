"""End-to-end offline batch: warehouse → triples → ALS → Eval & Promotion Gate → Qdrant + Redis.

Orchestrates the seams. The heavy Spark work (read, map, index, fit) stays in
the executors; the collected factor matrices are evaluated and gated before
being loaded into the artifact stores.
Optionally trains and indexes Two-Tower neural candidate retrieval model when enabled.
"""

from __future__ import annotations

import logging

from . import recommend
from .config import Settings, load_settings
from .evals.evaluator import ModelEvaluator
from .interactions import build_triples, index_interactions
from .load import qdrant as qdrant_load
from .load import redis_cache
from .model_version import resolve_model_version
from .registry.metadata import ModelMetadata
from .registry.registry import ModelRegistry
from .spark import build_spark
from .train import train_als
from .two_tower.pipeline import train_and_index_two_tower
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


def run(settings: Settings | None = None, registry: ModelRegistry | None = None) -> dict:
    """Run the full pipeline. Returns a summary dict of what was produced."""
    settings = settings or load_settings()
    model_version = resolve_model_version(settings)
    log.info("starting ALS batch model_version=%s driver=%s", model_version, settings.warehouse_driver)

    spark = build_spark(settings)
    try:
        events = read_tracking_events(spark, settings)
        triples = build_triples(events, settings)

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

        # ── Evaluation & Holdout Split ──────────────────────────────────────────
        predicted_dict = {
            u: [lid for lid, _ in recs]
            for u, recs in user_recs.items()
        }

        raw_interactions = []
        try:
            user_key_expr = F.coalesce(
                F.when(F.col("principal_id") != "", F.col("principal_id")),
                F.when(F.col("anonymous_id") != "", F.col("anonymous_id")),
                F.lit("anonymous"),
            ).alias("user_id")
            raw_rows = events.select(
                user_key_expr,
                F.col("listing_id").alias("listing_id"),
                F.coalesce(
                    F.unix_timestamp(F.col("occurred_at")).cast("double"),
                    F.col("occurred_at").cast("double"),
                    F.lit(0.0),
                ).alias("timestamp"),
            ).filter(F.col("listing_id") != "").collect()
            raw_interactions = [
                {
                    "user_id": r["user_id"],
                    "listing_id": r["listing_id"],
                    "timestamp": float(r["timestamp"] if r["timestamp"] is not None else 0.0),
                }
                for r in raw_rows
            ]
        except Exception as exc:
            log.warning("could not extract raw timestamped events: %s", exc)

        evaluator = ModelEvaluator(k_values=[5, 10, 20])
        eval_results = evaluator.evaluate(
            predicted_dict=predicted_dict,
            raw_interactions=raw_interactions,
            all_catalog_items=item_ids,
            split_k=1,
        )
        metrics = eval_results
        log.info("model evaluation metrics: %s", metrics)

        # ── Model Registry & Promotion Gate ──────────────────────────────────────
        if registry is None:
            redis_client = None
            try:
                from redis import Redis  # noqa: PLC0415
                redis_client = Redis(
                    host=settings.redis_host,
                    port=settings.redis_port,
                    password=settings.redis_password or None,
                    db=settings.redis_db,
                    decode_responses=True,
                )
                redis_client.ping()
            except Exception as exc:
                log.info("redis not available for model registry: %s (using memory)", exc)
                redis_client = None
            registry = ModelRegistry(redis_client=redis_client)

        metadata = ModelMetadata(
            model_version=model_version,
            model_name="recsys-als",
            model_type="als",
            metrics=metrics,
            status="candidate",
        )
        registry.register_model(metadata)
        promoted, reason = registry.evaluate_and_promote(
            model_version,
            primary_metric=settings.promotion_primary_metric,
            min_relative_improvement=settings.promotion_min_relative_improvement,
            min_coverage_ratio=settings.promotion_min_coverage_ratio,
        )

        summary: dict = {
            "model_version": model_version,
            "decision": "promoted" if promoted else "rejected",
            "reason": reason,
            "metrics": metrics,
            "items": len(item_ids),
            "users": len(user_ids),
            "popular": len(popular),
        }

        if not promoted:
            log.info("candidate model %s rejected by promotion gate: %s", model_version, reason)
            return summary

        # ── Publish to Qdrant and Redis (ONLY if Promoted) ──────────────────────
        qdrant_counts = qdrant_load.load_vectors(
            settings,
            model_version,
            item_rows=zip(item_ids, item_vecs, strict=False),
            user_rows=zip(user_ids, user_vecs, strict=False),
        )
        cache_counts = redis_cache.load_cache(settings, model_version, user_recs, item_recs, popular)

        summary["qdrant"] = qdrant_counts
        summary["cache"] = cache_counts

        # ── Two-Tower Stage (Optional) ───────────────────────────────────────────
        if settings.enable_two_tower:
            catalog_items = [
                {
                    "listing_id": lid,
                    "price": 100.0,
                    "popularity": 1.0,
                    "category_id": "general",
                }
                for lid in item_ids
            ]
            _tt_model, tt_vectors = train_and_index_two_tower(
                catalog_items=catalog_items,
                embedding_dim=settings.two_tower_dim,
            )
            tt_count = qdrant_load.load_two_tower_vectors(
                settings=settings,
                model_version=model_version,
                item_vectors=tt_vectors,
            )
            summary["two_tower_items"] = tt_count
            log.info("two-tower stage indexed items=%d", tt_count)

        log.info("batch complete %s", summary)
        return summary
    finally:
        spark.stop()
