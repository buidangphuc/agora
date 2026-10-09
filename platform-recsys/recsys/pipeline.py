"""End-to-end offline batch: governed dataset → triples → ALS → Eval & Promotion Gate → Qdrant + Redis.

Orchestrates the seams. The heavy Spark work (read, map, index, fit) stays in
the executors; the collected factor matrices are evaluated and gated before
being loaded into the artifact stores.
Optionally trains the Two-Tower retrieval model on governed feature snapshots and publishes its item
vectors with the generation when enabled.
"""

from __future__ import annotations

import logging

from . import recommend
from .config import Settings, load_settings
from .dataset import resolve_dataset
from .evals.evaluator import ModelEvaluator
from .evals.holdout import EVAL_PROTOCOL, leave_last_new_item_out
from .interactions import dataset_triples, index_interactions
from .load import redis_cache
from .model_version import resolve_model_version
from .monitoring import generation as drift_monitor
from .publish import publish_generation, refresh_serving_ttl
from .registry.metadata import ModelMetadata
from .registry.registry import ModelRegistry
from .spark import build_spark
from .structural_gate import structural_check
from .train import train_als
from .two_tower import stage as two_tower_stage

log = logging.getLogger("recsys.pipeline")


def _collect_factors(factors_df, id_col: str):
    """Collect a factors DataFrame to (ids, vectors) on the driver, L2-normalized."""
    ids: list[str] = []
    vectors: list[list[float]] = []
    for row in factors_df.collect():
        ids.append(row[id_col])
        vectors.append(recommend.l2_normalize(list(row["features"])))
    return ids, vectors


def timestamped_interactions(events):
    """(user_id, listing_id, timestamp) rows for the offline evaluation split.

    The timestamp is the pair's ``last_occurred_at`` from the dataset. It may be
    TIMESTAMP_NTZ (DuckDB/pandas export) or TIMESTAMP: cast it to TIMESTAMP before
    taking epoch seconds. A direct CAST(... AS DOUBLE) is an analysis error on
    TIMESTAMP_NTZ, which used to leave the evaluation set empty.
    """
    from pyspark.sql import functions as F  # noqa: PLC0415

    ts = F.unix_timestamp(F.col("last_occurred_at").cast("timestamp")).cast("double")
    return events.select(
        F.col("user_key").alias("user_id"),
        F.col("listing_id").alias("listing_id"),
        F.coalesce(ts, F.lit(0.0)).alias("timestamp"),
    ).filter(F.col("listing_id") != "")


def evaluate_generation(events, settings: Settings) -> dict:
    """Score this generation's training recipe on a holdout it never saw.

    The published model is trained on the whole dataset. Evaluation instead fits a second
    ALS model on the per-user training split (leave-last-new-item-out, evals.holdout), so
    the held-out pairs and anything after them are not in its training data. It ranks
    each test user's unseen items only. The returned metrics carry EVAL_PROTOCOL.
    """
    from pyspark.sql import functions as F  # noqa: PLC0415

    evaluator = ModelEvaluator(k_values=[5, 10, 20])
    raw_interactions = []
    try:
        raw_interactions = [
            {
                "user_id": r["user_id"],
                "listing_id": r["listing_id"],
                "timestamp": float(r["timestamp"] if r["timestamp"] is not None else 0.0),
            }
            for r in timestamped_interactions(events).collect()
        ]
    except Exception as exc:
        log.warning("could not extract timestamped interactions: %s", exc)

    holdout = leave_last_new_item_out(raw_interactions)
    if not holdout.actual:
        metrics = evaluator.evaluate(
            actual_dict={},
            predicted_dict={},
            split_strategy=EVAL_PROTOCOL,
            train_events_count=holdout.train_events,
            test_events_count=0,
        )
        return {**metrics, "eval_protocol": EVAL_PROTOCOL}

    # Drop each test user's pairs last seen at or after the moment they discovered the target.
    cutoffs = events.sparkSession.createDataFrame(list(holdout.cutoffs.items()), ["_uk", "_cut"])
    train_events = (
        events.withColumn("_uk", F.col("user_key"))
        .withColumn("_ts", F.unix_timestamp(F.col("last_occurred_at").cast("timestamp")).cast("double"))
        .join(cutoffs, on="_uk", how="left")
        .filter(F.col("_cut").isNull() | (F.col("_ts") < F.col("_cut")))
        .drop("_uk", "_ts", "_cut")
    )
    eval_triples = dataset_triples(train_events)
    artifacts = train_als(index_interactions(eval_triples, settings), settings)
    item_ids, item_vecs = _collect_factors(artifacts.item_factors, "listing_id")
    user_ids, user_vecs = _collect_factors(artifacts.user_factors, "user_key")

    test_users = set(holdout.actual)
    seen: dict[str, set[str]] = {}
    for r in (
        eval_triples.filter(F.col("user_key").isin(sorted(test_users)))
        .select("user_key", "listing_id")
        .collect()
    ):
        seen.setdefault(r["user_key"], set()).add(r["listing_id"])
    ranked = recommend.top_n_unseen(
        user_ids, user_vecs, item_ids, item_vecs, max(settings.top_n, 20), seen, only_users=test_users
    )
    metrics = evaluator.evaluate(
        actual_dict=holdout.actual,
        predicted_dict={u: [lid for lid, _ in recs] for u, recs in ranked.items()},
        all_catalog_items=item_ids,
        split_strategy=EVAL_PROTOCOL,
        cutoff_timestamp=holdout.cutoff_timestamp,
        train_events_count=holdout.train_events,
        test_events_count=holdout.test_events,
    )
    return {**metrics, "eval_protocol": EVAL_PROTOCOL}


def _open_registry(settings: Settings, redis_client=None) -> ModelRegistry:
    if redis_client is None:
        try:
            redis_client = redis_cache.connect(settings)
            redis_client.ping()
        except Exception as exc:
            log.info("redis not available for model registry: %s (using memory)", exc)
            redis_client = None
    return ModelRegistry(redis_client=redis_client)


def _refresh_ttl(settings: Settings, redis_client=None) -> None:
    """Keep the serving and previous generations' keys alive on every run. Best effort: a run that
    published nothing must not fail because Redis is down."""
    try:
        refresh_serving_ttl(settings, redis_client)
    except Exception as exc:
        log.warning("could not refresh the serving generations' TTL: %s", exc)


def assess_drift(settings: Settings, registry: ModelRegistry, current: dict[str, list[float]]) -> dict:
    """Compare this run's distributions with the champion's (the generation it would replace).

    Observational: the verdict goes to the log, the optional Prometheus file and the model metadata
    (the caller records it), never into the gate. Returns the drift record (see monitoring.generation).
    """
    incumbent_version = registry.get_champion_version()
    incumbent = registry.get_model(incumbent_version) if incumbent_version else None
    baseline = (incumbent.parameters or {}).get("distribution") if incumbent else None
    record, report = drift_monitor.compare(
        baseline, current, incumbent_version, settings.drift_alert_threshold
    )
    if record["status"] == "drifted":
        log.warning("drift against %s: %s", incumbent_version, record)
    else:
        log.info("drift against %s: %s", incumbent_version, record)
    if report is not None and settings.drift_metrics_path:
        try:
            drift_monitor.write_prometheus(
                settings.drift_metrics_path, report, settings.drift_alert_threshold
            )
        except OSError as exc:
            log.warning("could not write drift metrics to %s: %s", settings.drift_metrics_path, exc)
    return record


def run(
    settings: Settings | None = None,
    registry: ModelRegistry | None = None,
    redis_client=None,
    qdrant_client=None,
) -> dict:
    """Run the full pipeline. Returns a summary dict of what was produced.

    ``redis_client`` / ``qdrant_client`` are injectable for tests; by default the job connects from
    the settings.
    """
    settings = settings or load_settings()
    model_version = resolve_model_version(settings)
    # Resolve the governed dataset BEFORE starting Spark: with none, refuse to run
    # (ConfigError → exit 2) and register nothing. No fallback to raw events.
    dataset = resolve_dataset(settings)
    # The two-tower stage trains on governed feature snapshots only: with none, refuse to run now,
    # before Spark starts and before anything is registered (ConfigError → exit 2).
    two_tower_inputs = two_tower_stage.resolve_inputs(settings) if settings.enable_two_tower else None
    log.info("starting ALS batch model_version=%s dataset=%s", model_version, dataset.path)

    spark = build_spark(settings)
    try:
        events = spark.read.parquet(dataset.path)
        triples = dataset_triples(events)

        from pyspark.sql import functions as F  # noqa: PLC0415

        pop_rows = (
            triples.groupBy("listing_id")
            .agg(F.sum("weight").alias("w"))
            .orderBy(F.desc("w"))
            .limit(settings.top_n)
            .collect()
        )
        popular = [(r["listing_id"], float(r["w"])) for r in pop_rows]

        dataset_users = triples.select("user_key").distinct().count()
        dataset_items = triples.select("listing_id").distinct().count()

        indexed = index_interactions(triples, settings)
        artifacts = train_als(indexed, settings)

        item_ids, item_vecs = _collect_factors(artifacts.item_factors, "listing_id")
        user_ids, user_vecs = _collect_factors(artifacts.user_factors, "user_key")
        log.info("trained factors items=%d users=%d rank=%d", len(item_ids), len(user_ids), artifacts.rank)

        # Precomputed Top-N artifacts (numpy on the driver).
        user_recs = recommend.top_n_for_users(user_ids, user_vecs, item_ids, item_vecs, settings.top_n)
        item_recs = recommend.similar_items(item_ids, item_vecs, settings.top_n)

        # ── Drift of this run's distributions against the generation it would replace ──
        if registry is None:
            registry = _open_registry(settings, redis_client)
        distribution = drift_monitor.spark_distributions(triples, user_recs)
        drift = assess_drift(settings, registry, distribution)
        drift_metrics = {"drift_psi_max": drift["max_psi"]} if "max_psi" in drift else {}

        # ── Structural gate: a degenerate candidate never reaches the metric gate ──
        structural_ok, structural_reason = structural_check(
            user_recs,
            item_recs,
            (user_vecs, item_vecs),
            dataset_users,
            dataset_items,
            settings,
        )
        if not structural_ok:
            # metrics.gate_reason is a string next to eval_protocol (also a string in metrics); the
            # reason is mirrored in parameters.gate_reason for readers that treat metrics as floats.
            registry.register_model(
                ModelMetadata(
                    model_version=model_version,
                    model_name="recsys-als",
                    model_type="als",
                    metrics={"gate_reason": structural_reason, **drift_metrics},
                    parameters={
                        "dataset": dataset.lineage,
                        "gate_reason": structural_reason,
                        "distribution": distribution,
                        "drift": drift,
                    },
                    status="rejected",
                )
            )
            _refresh_ttl(settings, redis_client)
            log.warning("candidate %s rejected by structural gate: %s", model_version, structural_reason)
            return {
                "model_version": model_version,
                "dataset": dataset.lineage,
                "decision": "rejected",
                "reason": structural_reason,
                "gate": "structural",
                "drift": drift,
                "items": len(item_ids),
                "users": len(user_ids),
                "popular": len(popular),
            }

        # ── Evaluation (leakage-free, on a separately trained model) ────────────
        metrics = evaluate_generation(events, settings)
        log.info("model evaluation metrics: %s", metrics)

        # A run that could not be evaluated is not a candidate: nothing is
        # registered, the gate is not consulted and nothing is published.
        if not metrics.get("test_events"):
            summary = {
                "model_version": model_version,
                "dataset": dataset.lineage,
                "decision": "skipped",
                "reason": "no usable holdout: the temporal split left no test events, "
                "so no evaluation was possible",
                "metrics": metrics,
                "drift": drift,
                "items": len(item_ids),
                "users": len(user_ids),
                "popular": len(popular),
            }
            log.info("candidate model %s not evaluated: %s", model_version, summary["reason"])
            _refresh_ttl(settings, redis_client)
            return summary

        # ── Model Registry & Promotion Gate ──────────────────────────────────────
        metrics = {**metrics, **drift_metrics}
        metadata = ModelMetadata(
            model_version=model_version,
            model_name="recsys-als",
            model_type="als",
            metrics=metrics,
            parameters={"dataset": dataset.lineage, "distribution": distribution, "drift": drift},
            status="candidate",
        )
        registry.register_model(metadata)
        incumbent_version = registry.get_champion_version()
        incumbent = registry.get_model(incumbent_version) if incumbent_version else None
        decision = registry.evaluate(
            model_version,
            primary_metric=settings.promotion_primary_metric,
            min_relative_improvement=settings.promotion_min_relative_improvement,
            min_coverage_ratio=settings.promotion_min_coverage_ratio,
            force=settings.promotion_force,
        )
        promoted, reason = decision.promoted, decision.reason

        summary: dict = {
            "model_version": model_version,
            "dataset": dataset.lineage,
            "decision": "promoted" if promoted else "rejected",
            "reason": reason,
            # The comparison the gate made, auditable from the run's own output.
            "primary_metric": settings.promotion_primary_metric,
            "candidate_value": metrics.get(settings.promotion_primary_metric),
            "incumbent_version": incumbent_version,
            "incumbent_value": (
                incumbent.metrics.get(settings.promotion_primary_metric) if incumbent else None
            ),
            "metrics": metrics,
            "drift": drift,
            "items": len(item_ids),
            "users": len(user_ids),
            "popular": len(popular),
        }

        if not promoted:
            registry.reject(decision)
            log.info("candidate model %s rejected by promotion gate: %s", model_version, reason)
            _refresh_ttl(settings, redis_client)
            return summary

        # ── Two-tower stage (optional): trained and checked BEFORE the publish ───
        # The generation is written whole or not at all, so a stage that cannot produce safe
        # vectors (missing features, every vector degenerate) rejects the candidate like a failed
        # publish does: serving and the champion stay as they were.
        two_tower = None
        if two_tower_inputs is not None:
            try:
                pairs = two_tower_stage.collect_pairs(triples, settings.two_tower_max_pairs)
                two_tower = two_tower_stage.run_stage(settings, two_tower_inputs, pairs)
            except Exception as exc:
                registry.reject(decision, reason=f"two-tower stage failed: {type(exc).__name__}: {exc}")
                log.error("two-tower stage of %s failed, candidate rejected: %s", model_version, exc)
                raise
            decision.candidate.parameters["two_tower"] = two_tower.as_parameters(settings.two_tower_dim)

        # ── Publish as a generation (ONLY if Promoted) ───────────────────────────
        # The champion changes only once the publish has succeeded: a failed publish leaves the
        # previous champion (and serving) untouched and the candidate recorded as rejected.
        try:
            published = publish_generation(
                settings,
                model_version,
                user_recs,
                item_recs,
                popular,
                item_rows=zip(item_ids, item_vecs, strict=False),
                user_rows=zip(user_ids, user_vecs, strict=False),
                redis_client=redis_client,
                qdrant_client=qdrant_client,
                two_tower_vectors=two_tower.vectors if two_tower else None,
            )
        except Exception as exc:
            registry.reject(decision, reason=f"publish failed: {type(exc).__name__}")
            log.error("publish of %s failed, candidate rejected: %s", model_version, exc)
            raise
        registry.apply_promotion(decision)
        summary["qdrant"] = published["qdrant"]
        summary["cache"] = published["cache"]
        summary["serving"] = published["serving"]
        summary["previous"] = published["previous"]

        if two_tower:
            summary["two_tower_items"] = published["two_tower_items"]
            summary["two_tower"] = two_tower.report.as_dict()
            log.info("two-tower stage published %d item vectors", published["two_tower_items"])

        log.info("batch complete %s", summary)
        return summary
    finally:
        spark.stop()
