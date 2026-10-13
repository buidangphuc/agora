"""The GBDT stage of the batch run: governed impressions in, a gated ranker artifact out.

Order of the run (``pipeline.py``): resolve the inputs before Spark starts (ConfigError, exit 2), train after
the ALS candidate is promotable, publish the artifact with the generation, apply the GBDT candidate only
after the publish succeeded. The candidate is a version of ``recsys-gbdt`` in the MLflow registry, judged by
its own policy against its own champion, never the ALS one. The stage evaluates; it never applies.
"""

from __future__ import annotations

import json
import logging
import tempfile
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Any

import numpy as np
import pandas as pd
from modelregistry import Decision

from recsys import registry_client as rc
from recsys.config import ConfigError, Settings
from recsys.dataset import Dataset, resolve_rank_dataset
from recsys.ranker import contract, gbdt, training

log = logging.getLogger("recsys.ranker.stage")

NDCG_K = 10
PRIMARY_METRIC = "ndcg@10"
BASELINE_MARGIN = 1.01


@dataclass
class RankerInputs:
    dataset: Dataset
    popularity: training.SnapshotSeries
    attributes: training.SnapshotSeries | None


@dataclass
class RankerResult:
    """``decision`` is the registry's verdict (``promotable`` | ``rejected``); ``artifact`` is the
    ``agora-gbdt/1`` document when promotable, else None."""

    decision: Decision
    reason: str
    summary: dict[str, Any]
    artifact: dict[str, Any] | None


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


def _recall(matrix: training.Matrix, rows: np.ndarray, scores: np.ndarray, k: int = NDCG_K) -> float:
    """Mean recall@k over the lists with a positive (same tie-break as ``_ndcg``)."""
    local = {int(g): i for i, g in enumerate(rows)}
    groups = [[local[i] for i in grp] for grp in training.group_rows(matrix.frame, rows)]
    labels = matrix.frame["label"].to_numpy(dtype=float)[rows]
    tie = training.tiebreak(matrix.frame["listing_id"].to_numpy()[rows])
    values = []
    for g in groups:
        idx = np.array(g)
        positives = float((labels[idx] > 0).sum())
        if positives == 0:
            continue
        order = np.lexsort((tie[idx], -scores[idx]))
        values.append(float((labels[idx][order][:k] > 0).sum()) / positives)
    return float(np.mean(values)) if values else 0.0


def _context_matrix(matrix: training.Matrix, x: np.ndarray) -> np.ndarray:
    """``x`` plus one integer-coded column per session context column (evaluation only, never published)."""
    cols = [
        pd.factorize(matrix.frame[c].fillna("(none)").astype(str))[0].astype(float)
        for c in training.CONTEXT_COLUMNS
    ]
    return np.column_stack([x, *cols])


def evaluate_variant(
    settings: Settings, matrix: training.Matrix, split: training.Split, *, with_context: bool
) -> dict[str, float]:
    """Train on the time-based train rows, score the holdout; optionally with the session context columns."""
    x, _ = training.apply_debiased_ctr(matrix, split.train, settings.gbdt_ctr_min_impressions)
    if with_context:
        x = _context_matrix(matrix, x)
    model = _fit(settings, x, matrix, split.train)
    scores = model.predict(x[split.test])
    ndcg, _ = _ndcg(matrix, split.test, scores)
    return {"ndcg@10": round(ndcg, 6), "recall@10": round(_recall(matrix, split.test, scores), 6)}


def _evaluate_context(settings: Settings, matrix: training.Matrix, split: training.Split) -> dict[str, Any]:
    without = evaluate_variant(settings, matrix, split, with_context=False)
    if not all(c in matrix.frame.columns for c in training.CONTEXT_COLUMNS):
        return {
            "context_features": "disabled",
            "context_reason": "columns_absent",
            "context_metrics": {"with_context": None, "without_context": without},
        }
    try:
        with_ctx = evaluate_variant(settings, matrix, split, with_context=True)
    except Exception as exc:  # noqa: BLE001 - context never fails the run
        log.warning("context evaluation failed: %s", exc)
        return {
            "context_features": "disabled",
            "context_reason": "evaluation_failed",
            "context_metrics": {"with_context": None, "without_context": without},
        }
    ok = with_ctx["ndcg@10"] >= without["ndcg@10"] and with_ctx["recall@10"] >= without["recall@10"]
    return {
        "context_features": "enabled" if ok else "disabled",
        "context_reason": "qualified" if ok else "regression",
        "context_metrics": {"with_context": with_ctx, "without_context": without},
    }


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


def run_stage(settings: Settings, inputs: RankerInputs, model_version: str) -> RankerResult:
    """Train, evaluate on the temporal holdout, register and evaluate as ``recsys-gbdt``. Returns the
    verdict; a promotable result carries the artifact to publish. A rejected candidate is already recorded
    as rejected; a promotable one is applied by the pipeline once the generation is published."""
    gbdt_version = f"gbdt-{model_version}"

    frame = training.read_dataset(inputs.dataset.path)
    frame = training.limit_lists(frame, settings.gbdt_max_list, settings.gbdt_max_lists)
    matrix = training.build_matrix(frame, inputs.popularity, inputs.attributes)
    split = training.temporal_split(matrix.frame, settings.gbdt_holdout_fraction)
    params: dict[str, Any] = {
        "rows": matrix.counts,
        "holdout": {
            "fraction": settings.gbdt_holdout_fraction,
            "cutoff": _iso(split.cutoff),
            "train_last_at": _iso(split.train_last_at),
            "test_first_at": _iso(split.test_first_at),
            "train_lists": split.train_lists,
            "test_lists": split.test_lists,
        },
    }
    lineage: dict[str, Any] = {
        "eval_protocol": contract.EVAL_PROTOCOL,
        "model_version": model_version,
        **rc.dataset_lineage(inputs.dataset.lineage),
        **rc.snapshot_lineage("item_popularity", inputs.popularity.lineage),
        **rc.snapshot_lineage("item_attributes", inputs.attributes.lineage if inputs.attributes else None),
    }
    run_id = rc.start_run(
        f"recsys-gbdt-{model_version}",
        {
            "gbdt_trees": settings.gbdt_trees,
            "gbdt_max_depth": settings.gbdt_max_depth,
            "gbdt_learning_rate": settings.gbdt_learning_rate,
            "gbdt_min_leaf": settings.gbdt_min_leaf,
            "gbdt_holdout_fraction": settings.gbdt_holdout_fraction,
        },
    )
    metrics: dict[str, float] = {}
    context: dict[str, Any] = {}

    def reject(reason: str) -> RankerResult:
        decision = rc.register_rejected(rc.GBDT_MODEL, run_id, reason, lineage)
        log.warning("GBDT candidate %s rejected: %s", gbdt_version, reason)
        return RankerResult(
            decision, reason, _summary(gbdt_version, "rejected", reason, metrics, params, context), None
        )

    if len(matrix.frame) == 0 or split.train_lists == 0 or split.test_lists == 0:
        return reject(
            f"no usable rows: {matrix.counts['rows']} rows after the point-in-time join "
            f"({matrix.counts['rows_dropped_no_snapshot']} dropped for lack of an earlier snapshot), "
            f"{split.train_lists} train and {split.test_lists} holdout impressions"
        )

    # ── evaluation: train on the split, judge on the holdout ─────────────────
    x_eval, _source_eval = training.apply_debiased_ctr(matrix, split.train, settings.gbdt_ctr_min_impressions)
    eval_model = _fit(settings, x_eval, matrix, split.train)
    scores = eval_model.predict(x_eval[split.test])
    ndcg, scored_lists = _ndcg(matrix, split.test, scores)
    baseline, _ = _ndcg(matrix, split.test, training.baseline_scores(x_eval[split.test]))
    metrics.update(
        {
            PRIMARY_METRIC: round(ndcg, 6),
            "recall@10": round(_recall(matrix, split.test, scores), 6),
            "baseline_ndcg@10": round(baseline, 6),
            "holdout_lists_with_positive": scored_lists,
            "train_events_count": int(len(split.train)),
            "test_events_count": int(len(split.test)),
        }
    )
    if scored_lists == 0:
        return reject("no usable holdout: no held-out impression has a click or an add-to-cart")
    context.update(_evaluate_context(settings, matrix, split))
    if ndcg < baseline * BASELINE_MARGIN:
        return reject(
            f"below fixed-weight baseline: holdout {PRIMARY_METRIC} {ndcg:.6f} < "
            f"baseline {baseline:.6f} x {BASELINE_MARGIN}"
        )

    # ── the model that ships: refit on every row, CTR recomputed over all of them ──
    everything = np.arange(len(matrix.frame))
    x_final, source_final = training.apply_debiased_ctr(matrix, everything, settings.gbdt_ctr_min_impressions)
    final = _fit(settings, x_final, matrix, everything)
    counts = {s: int((source_final == s).sum()) for s in (training.DEBIASED, training.FALLBACK)}
    params["ctr_source_counts"] = counts
    if settings.gbdt_rows_path:
        _write_rows(settings.gbdt_rows_path, matrix, x_final, source_final, split)

    artifact = gbdt.to_artifact(
        final, model_version=gbdt_version, generation=model_version, metrics=dict(metrics)
    )
    with tempfile.TemporaryDirectory() as tmp:
        (Path(tmp) / "ranker.json").write_text(json.dumps(artifact), encoding="utf-8")
        decision = rc.evaluate(rc.GBDT_MODEL, run_id, metrics, Path(tmp), lineage)
    summary = _summary(gbdt_version, decision.outcome, decision.reason, metrics, params, context)
    if decision.outcome != "promotable":
        log.warning("GBDT candidate %s rejected by the registry gate: %s", gbdt_version, decision.reason)
        return RankerResult(decision, decision.reason, summary, None)
    return RankerResult(decision, decision.reason, summary, artifact)


def _summary(
    version: str, decision: str, reason: str, metrics: dict, params: dict, context: dict | None = None
) -> dict[str, Any]:
    return {
        **(context or {}),
        "model_version": version,
        "decision": decision,
        "reason": reason,
        "metrics": dict(metrics),
        "rows": params["rows"].get("rows", 0),
        "ctr_source": params.get("ctr_source_counts"),
        "holdout": params["holdout"],
    }
