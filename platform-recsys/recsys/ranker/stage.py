"""The GBDT stage of the batch run: governed impressions in, a gated ranker artifact out.

Order of the run (``pipeline.py``): resolve the inputs before Spark starts (ConfigError, exit 2), train after
the ALS promotion decision, publish the artifact with the generation, promote the GBDT candidate only after
the publish succeeded. The candidate is registered as ``gbdt-<model_version>`` and gated against its own
champion pointer (``recs:model:champion:gbdt``), never the ALS champion.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from datetime import datetime
from typing import Any

import numpy as np

from recsys.config import ConfigError, Settings
from recsys.dataset import Dataset, resolve_rank_dataset
from recsys.ranker import contract, gbdt, training
from recsys.registry.metadata import ModelMetadata
from recsys.registry.registry import GBDT_CHAMPION_KEY, ModelRegistry, PromotionDecision

log = logging.getLogger("recsys.ranker.stage")

MODEL_NAME = "recsys-gbdt-ranker"
NDCG_K = 10
PRIMARY_METRIC = "ndcg@10"


@dataclass
class RankerInputs:
    dataset: Dataset
    popularity: training.SnapshotSeries
    attributes: training.SnapshotSeries | None


@dataclass
class RankerResult:
    promoted: bool
    reason: str
    summary: dict[str, Any]
    artifact: dict[str, Any] | None
    decision: PromotionDecision
    registry: ModelRegistry


def resolve_inputs(settings: Settings) -> RankerInputs:
    """The ranking dataset and the snapshot series. Raises ConfigError (the job exits 2, before Spark starts
    and before anything is registered) when the dataset or every item_popularity snapshot is missing."""
    dataset = resolve_rank_dataset(settings)
    popularity = training.SnapshotSeries(settings.item_features_dir, "item_popularity", 1, "listing_id")
    if not popularity:
        raise ConfigError(
            f"no item_popularity@v1 feature snapshot under ITEM_FEATURES_DIR={settings.item_features_dir}: "
            "the GBDT stage joins impressions to the snapshots in force at their time "
            "(run `python -m featurestore materialize` regularly)"
        )
    attributes = training.SnapshotSeries(settings.item_attributes_dir, "item_attributes", 1, "listing_id")
    return RankerInputs(dataset, popularity, attributes or None)


def _fit(settings: Settings, x: np.ndarray, matrix: training.Matrix, rows: np.ndarray) -> gbdt.Model:
    """Boost over ``rows`` only (bins and splits never see the other rows)."""
    local = {int(g): i for i, g in enumerate(rows)}
    groups = [[local[i] for i in grp] for grp in training.group_rows(matrix.frame, rows)]
    labels = matrix.frame["label"].to_numpy(dtype=float)[rows]
    return gbdt.fit(
        x[rows],
        gbdt.Lists.build(groups, labels, k=NDCG_K),
        trees=settings.gbdt_trees,
        max_depth=settings.gbdt_max_depth,
        learning_rate=settings.gbdt_learning_rate,
        min_leaf=settings.gbdt_min_leaf,
    )


def _ndcg(matrix: training.Matrix, rows: np.ndarray, scores: np.ndarray) -> tuple[float, int]:
    local = {int(g): i for i, g in enumerate(rows)}
    groups = [[local[i] for i in grp] for grp in training.group_rows(matrix.frame, rows)]
    labels = matrix.frame["label"].to_numpy(dtype=float)[rows]
    tie = training.tiebreak(matrix.frame["listing_id"].to_numpy()[rows])
    return gbdt.mean_ndcg(groups, labels, scores, tie, k=NDCG_K)


def _iso(ts: datetime | None) -> str | None:
    return None if ts is None else ts.strftime("%Y-%m-%dT%H:%M:%SZ")


def _write_rows(
    path: str, matrix: training.Matrix, x: np.ndarray, source: np.ndarray, split: training.Split
) -> None:
    import pandas as pd  # noqa: PLC0415

    out = matrix.frame.copy()
    for j, name in enumerate(contract.RANKING_FEATURES):
        out[name] = x[:, j]
    out["ctr_source"] = source
    holdout = np.zeros(len(out), dtype=bool)
    holdout[split.test] = True
    out["split"] = np.where(holdout, "holdout", "train")
    pd.DataFrame(out).to_parquet(path, index=False)


def run_stage(
    settings: Settings, inputs: RankerInputs, model_version: str, registry: ModelRegistry
) -> RankerResult:
    """Train, evaluate on the temporal holdout, gate. Returns the verdict; a promoted result carries the
    artifact to publish. A rejected candidate is already recorded as rejected."""
    gbdt_version = f"gbdt-{model_version}"
    reg = registry.scoped(GBDT_CHAMPION_KEY)

    frame = training.read_dataset(inputs.dataset.path)
    frame = training.limit_lists(frame, settings.gbdt_max_list, settings.gbdt_max_lists)
    matrix = training.build_matrix(frame, inputs.popularity, inputs.attributes)
    split = training.temporal_split(matrix.frame, settings.gbdt_holdout_fraction)
    params: dict[str, Any] = {
        "dataset": inputs.dataset.lineage,
        "feature_list": list(contract.RANKING_FEATURES),
        "features": {
            "item_popularity": inputs.popularity.lineage,
            "item_attributes": inputs.attributes.lineage if inputs.attributes else None,
        },
        "rows": matrix.counts,
        "holdout": {
            "fraction": settings.gbdt_holdout_fraction,
            "cutoff": _iso(split.cutoff),
            "train_last_at": _iso(split.train_last_at),
            "test_first_at": _iso(split.test_first_at),
            "train_lists": split.train_lists,
            "test_lists": split.test_lists,
        },
        "hyper": {
            "trees": settings.gbdt_trees,
            "max_depth": settings.gbdt_max_depth,
            "learning_rate": settings.gbdt_learning_rate,
            "min_leaf": settings.gbdt_min_leaf,
            "ctr_min_impressions": settings.gbdt_ctr_min_impressions,
        },
    }
    metrics: dict[str, Any] = {"eval_protocol": contract.EVAL_PROTOCOL}

    def reject(reason: str) -> RankerResult:
        candidate = ModelMetadata(
            model_version=gbdt_version,
            model_name=MODEL_NAME,
            model_type="gbdt",
            metrics=metrics,
            parameters=params,
            status="candidate",
        )
        reg.register_model(candidate)
        decision = PromotionDecision(False, reason, candidate)
        reg.reject(decision, reason=reason)
        log.warning("GBDT candidate %s rejected: %s", gbdt_version, reason)
        return RankerResult(
            False, reason, _summary(gbdt_version, "rejected", reason, metrics, params), None, decision, reg
        )

    if len(matrix.frame) == 0 or split.train_lists == 0 or split.test_lists == 0:
        return reject(
            f"no usable rows: {matrix.counts['rows']} rows after the point-in-time join "
            f"({matrix.counts['rows_dropped_no_snapshot']} dropped for lack of an earlier snapshot), "
            f"{split.train_lists} train and {split.test_lists} holdout impressions"
        )

    # ── evaluation: train on the split, judge on the holdout ─────────────────
    x_eval, source_eval = training.apply_debiased_ctr(matrix, split.train, settings.gbdt_ctr_min_impressions)
    eval_model = _fit(settings, x_eval, matrix, split.train)
    ndcg, scored_lists = _ndcg(matrix, split.test, eval_model.predict(x_eval[split.test]))
    baseline, _ = _ndcg(matrix, split.test, training.baseline_scores(x_eval[split.test]))
    metrics.update(
        {
            PRIMARY_METRIC: round(ndcg, 6),
            "baseline_ndcg@10": round(baseline, 6),
            "holdout_lists_with_positive": scored_lists,
            "train_events_count": int(len(split.train)),
            "test_events_count": int(len(split.test)),
        }
    )
    if scored_lists == 0:
        return reject("no usable holdout: no held-out impression has a click or an add-to-cart")

    threshold = settings.promotion_min_relative_improvement
    if not settings.promotion_force and not ndcg > baseline * (1.0 + threshold):
        return reject(
            f"{PRIMARY_METRIC} {ndcg:.4f} does not beat the fixed-weight baseline {baseline:.4f} "
            f"by the required {threshold:.2%}"
        )

    # ── the model that ships: refit on every row, CTR recomputed over all of them ──
    everything = np.arange(len(matrix.frame))
    x_final, source_final = training.apply_debiased_ctr(matrix, everything, settings.gbdt_ctr_min_impressions)
    final = _fit(settings, x_final, matrix, everything)
    counts = {s: int((source_final == s).sum()) for s in (training.DEBIASED, training.FALLBACK)}
    params["ctr_source_counts"] = counts
    params["trees"] = len(final.trees)
    if settings.gbdt_rows_path:
        _write_rows(settings.gbdt_rows_path, matrix, x_final, source_final, split)

    candidate = ModelMetadata(
        model_version=gbdt_version,
        model_name=MODEL_NAME,
        model_type="gbdt",
        metrics=metrics,
        parameters=params,
        status="candidate",
    )
    reg.register_model(candidate)
    decision = reg.evaluate(
        gbdt_version,
        primary_metric=PRIMARY_METRIC,
        min_relative_improvement=threshold,
        min_coverage_ratio=settings.promotion_min_coverage_ratio,
        force=settings.promotion_force,
    )
    if not decision.promoted:
        reg.reject(decision)
        log.warning("GBDT candidate %s rejected by the registry gate: %s", gbdt_version, decision.reason)
        return RankerResult(
            False,
            decision.reason,
            _summary(gbdt_version, "rejected", decision.reason, metrics, params),
            None,
            decision,
            reg,
        )
    artifact = gbdt.to_artifact(
        final, model_version=gbdt_version, generation=model_version, metrics=dict(metrics)
    )
    return RankerResult(
        True,
        decision.reason,
        _summary(gbdt_version, "promoted", decision.reason, metrics, params),
        artifact,
        decision,
        reg,
    )


def _summary(version: str, decision: str, reason: str, metrics: dict, params: dict) -> dict[str, Any]:
    return {
        "model_version": version,
        "decision": decision,
        "reason": reason,
        "metrics": dict(metrics),
        "rows": params["rows"].get("rows", 0),
        "ctr_source": params.get("ctr_source_counts"),
        "holdout": params["holdout"],
    }
