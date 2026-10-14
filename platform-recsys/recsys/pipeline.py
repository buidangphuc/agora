"""End-to-end offline batch: governed dataset → triples → ALS → registry gate → Qdrant + Redis.

Orchestrates the seams. The heavy Spark work (read, map, index, fit) stays in
the executors; the collected factor matrices are evaluated and gated (py-modelregistry, two phases:
``evaluate`` every candidate, publish, then ``apply`` each) before being loaded into the artifact stores.
Optionally trains the GBDT ranker and the Two-Tower retrieval model on governed snapshots and publishes
them with the generation when enabled and the ALS candidate is promotable.
"""

from __future__ import annotations

import logging
import tempfile
from pathlib import Path

from . import recommend
from . import registry_client as rc
from . import spark as spark_session
from .config import Settings, load_settings
from .dataset import resolve_dataset
from .evals.evaluator import ModelEvaluator
from .evals.holdout import EVAL_PROTOCOL, leave_last_new_item_out
from .interactions import dataset_triples, index_interactions
from .model_version import resolve_model_version
from .monitoring import generation as drift_monitor
from .publish import publish_generation, refresh_serving_ttl
from .ranker import stage as ranker_stage
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


def _refresh_ttl(settings: Settings, redis_client=None) -> None:
    """Keep the serving and previous generations' keys alive on every run. Best effort: a run that
    published nothing must not fail because Redis is down."""
    try:
        refresh_serving_ttl(settings, redis_client)
    except Exception as exc:
        log.warning("could not refresh the serving generations' TTL: %s", exc)


def assess_drift(settings: Settings, current: dict[str, list[float]]) -> dict:
    """Compare this run's distributions with the champion's (the generation it would replace).

    The baseline is the ``drift_sketches.json`` artifact of the run behind the ``recsys-als`` champion; a
    champion without one (a seeded v1, or no champion at all) means no baseline. Observational: the
    verdict goes to the log, the optional Prometheus file and the candidate's run (the caller records it),
    never into the gate. Returns the drift record (see monitoring.generation).
    """
    found = rc.champion_drift_baseline()
    version, baseline = (found[0], found[1]) if found else (None, None)
    record, report = drift_monitor.compare(
        baseline, current, None if version is None else str(version), settings.drift_alert_threshold
    )
    if record["status"] == "drifted":
        log.warning("drift against champion version %s: %s", version, record)
    else:
        log.info("drift against champion version %s: %s", version, record)
    if report is not None and settings.drift_metrics_path:
        try:
            drift_monitor.write_prometheus(
                settings.drift_metrics_path, report, settings.drift_alert_threshold
            )
        except OSError as exc:
            log.warning("could not write drift metrics to %s: %s", settings.drift_metrics_path, exc)
    return record


def _numeric(metrics: dict) -> dict[str, float]:
    """The numeric aggregate metrics of an evaluation (strings such as the protocol name are not metrics)."""
    return {k: v for k, v in metrics.items() if isinstance(v, (int, float)) and not isinstance(v, bool)}


def _mark_rejected(candidates: list[tuple[str, int]], reason: str) -> None:
    """Record ``reason`` on every evaluated version. Best effort: a registry that is down cannot be told,
    and the version then stays ``promotable`` (later runs ignore it)."""
    for model, version in candidates:
        try:
            rc.mark_rejected(model, version, reason)
        except Exception as exc:
            log.error("could not mark %s v%s rejected (%s): %s", model, version, reason, exc)


def _apply_all(candidates: list[tuple[str, int]]) -> None:
    """Move each model's champion to its published version. A failure does not stop the others; it is
    logged naming every model whose champion was not moved, and the first error is raised."""
    errors: list[Exception] = []
    unmoved: list[str] = []
    for model, version in candidates:
        try:
            rc.apply(model, version)
        except Exception as exc:
            log.error("could not apply %s v%s: %s", model, version, exc)
            errors.append(exc)
            unmoved.append(f"{model} v{version}")
    if errors:
        log.error(
            "the generation is serving but the registry champion was NOT moved for: %s "
            "(the versions stay promotable)",
            ", ".join(unmoved),
        )
        raise errors[0]


def run(settings: Settings | None = None, redis_client=None, qdrant_client=None) -> dict:
    """Run the full pipeline. Returns a summary dict of what was produced.

    Order: validate and resolve every input (exit 2) → registry pre-flight (exit 1) → train ALS, evaluate
    it → only when it is ``promotable``: the GBDT and two-tower stages evaluate theirs → publish the
    generation → ``apply`` each promotable model. A stage or publish failure marks the evaluated versions
    rejected and raises; no alias moved before the publish succeeded.

    ``redis_client`` / ``qdrant_client`` are injectable for tests; by default the job connects from
    the settings.
    """
    settings = settings or load_settings()
    settings.validate_for_training()
    model_version = resolve_model_version(settings)
    # Resolve the governed dataset BEFORE starting Spark: with none (or a stale one), refuse to run
    # (ConfigError → exit 2) and register nothing. No fallback to raw events.
    dataset = resolve_dataset(settings)
    # The two-tower stage trains on governed feature snapshots only: with none, refuse to run now,
    # before Spark starts and before anything is registered (ConfigError → exit 2).
    two_tower_inputs = two_tower_stage.resolve_inputs(settings) if settings.enable_two_tower else None
    # The GBDT stage needs its ranking dataset and feature snapshots: same refusal rule, same moment.
    ranker_inputs = ranker_stage.resolve_inputs(settings) if settings.enable_gbdt else None
    # Fail closed: without a reachable registry nothing is trained (RegistryUnavailable → exit 1).
    rc.preflight()
    log.info("starting ALS batch model_version=%s dataset=%s", model_version, dataset.path)

    spark = spark_session.build_spark(settings)
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

        # ── Drift of this run's distributions against the champion it would replace ──
        distribution = drift_monitor.spark_distributions(triples, user_recs)
        drift = assess_drift(settings, distribution)
        drift_metrics = {"drift_psi_max": drift["max_psi"]} if "max_psi" in drift else {}

        sizes = {"items": len(item_ids), "users": len(user_ids), "popular": len(popular)}
        lineage = {
            "eval_protocol": EVAL_PROTOCOL,
            "model_version": model_version,
            **rc.dataset_lineage(dataset.lineage),
        }
        als_params = {
            "als_rank": settings.als_rank,
            "als_reg_param": settings.als_reg_param,
            "als_alpha": settings.als_alpha,
            "als_max_iter": settings.als_max_iter,
            "top_n": settings.top_n,
        }

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
            run_id = rc.start_run(f"recsys-als-{model_version}", als_params)
            rc.log_drift(run_id, drift)
            rc.register_rejected(rc.ALS_MODEL, run_id, structural_reason, lineage)
            _refresh_ttl(settings, redis_client)
            log.warning("candidate %s rejected by structural gate: %s", model_version, structural_reason)
            return {
                "model_version": model_version,
                "dataset": dataset.lineage,
                "decision": "rejected",
                "reason": structural_reason,
                "gate": "structural",
                "drift": drift,
                **sizes,
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
                **sizes,
            }
            log.info("candidate model %s not evaluated: %s", model_version, summary["reason"])
            _refresh_ttl(settings, redis_client)
            return summary

        # ── ALS candidate: register and evaluate (moves no alias) ────────────────
        metrics = {**metrics, **drift_metrics}
        run_id = rc.start_run(f"recsys-als-{model_version}", als_params)
        rc.log_drift(run_id, drift)
        rc.write_drift_sketches(run_id, distribution)
        with tempfile.TemporaryDirectory() as tmp:
            artifact_dir = rc.build_als_artifact(artifacts, Path(tmp) / "als")
            decision = rc.evaluate(rc.ALS_MODEL, run_id, _numeric(metrics), artifact_dir, lineage)

        summary: dict = {
            "model_version": model_version,
            "dataset": dataset.lineage,
            "decision": "promoted" if decision.outcome == "promotable" else "rejected",
            "reason": decision.reason,
            "metrics": metrics,
            "drift": drift,
            **sizes,
        }
        if decision.outcome != "promotable":
            log.info("candidate model %s rejected by promotion gate: %s", model_version, decision.reason)
            _refresh_ttl(settings, redis_client)
            return summary

        # ── GBDT and two-tower (optional): trained and evaluated BEFORE the publish ───
        # The generation is written whole or not at all, so a stage that fails outright rejects the
        # evaluated candidates like a failed publish does. A stage whose own candidate is rejected (or
        # skipped) does not block the generation: it ships without that model.
        promotable: list[tuple[str, int]] = [(rc.ALS_MODEL, decision.version)]
        gbdt = None
        if ranker_inputs is not None:
            try:
                gbdt = ranker_stage.run_stage(settings, ranker_inputs, model_version)
            except Exception as exc:
                _mark_rejected(promotable, f"gbdt stage failed: {type(exc).__name__}")
                log.error(
                    "gbdt stage of %s failed, candidates rejected: %s",
                    model_version,
                    type(exc).__name__,
                    exc_info=True,
                )
                raise
            summary["gbdt"] = gbdt.summary
            if gbdt.decision.outcome == "promotable":
                promotable.append((rc.GBDT_MODEL, gbdt.decision.version))

        two_tower = None
        if two_tower_inputs is not None:
            try:
                pairs = two_tower_stage.collect_pairs(triples, settings.two_tower_max_pairs)
                two_tower = two_tower_stage.run_stage(settings, two_tower_inputs, pairs, model_version)
            except Exception as exc:
                _mark_rejected(promotable, f"two-tower stage failed: {type(exc).__name__}")
                log.error(
                    "two-tower stage of %s failed, candidates rejected: %s",
                    model_version,
                    type(exc).__name__,
                    exc_info=True,
                )
                raise
            if two_tower.decision is None:
                summary["two_tower"] = {"decision": "skipped", "reason": two_tower.reason}
            else:
                summary["two_tower"] = {
                    **two_tower.as_parameters(settings.two_tower_dim),
                    "decision": two_tower.decision.outcome,
                    "reason": two_tower.reason,
                    "metrics": two_tower.metrics,
                }
                if two_tower.decision.outcome == "promotable":
                    promotable.append((rc.TWO_TOWER_MODEL, two_tower.decision.version))
        tower_ships = two_tower is not None and two_tower.decision is not None
        tower_ships = tower_ships and two_tower.decision.outcome == "promotable"
        ranker_ships = gbdt is not None and gbdt.decision.outcome == "promotable"

        # ── Publish as a generation (ONLY if Promoted) ───────────────────────────
        # The champions change only once the publish has succeeded: a failed publish leaves every
        # previous champion (and serving) untouched and the evaluated versions recorded as rejected.
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
                two_tower_vectors=two_tower.vectors if tower_ships else None,
                ranker_artifact=gbdt.artifact if ranker_ships else None,
            )
        except Exception as exc:
            _mark_rejected(promotable, f"publish failed: {type(exc).__name__}")
            log.error(
                "publish of %s failed, candidates rejected: %s",
                model_version,
                type(exc).__name__,
                exc_info=True,
            )
            raise
        _apply_all(promotable)
        if ranker_ships:
            summary["gbdt"]["decision"] = "promoted"
            summary["gbdt"]["ranker_key"] = published["ranker_key"]
        summary["qdrant"] = published["qdrant"]
        summary["cache"] = published["cache"]
        summary["serving"] = published["serving"]
        summary["previous"] = published["previous"]

        if tower_ships:
            summary["two_tower"]["decision"] = "promoted"
            summary["two_tower_items"] = published["two_tower_items"]
            log.info("two-tower stage published %d item vectors", published["two_tower_items"])

        log.info("batch complete %s", summary)
        return summary
    finally:
        spark.stop()
