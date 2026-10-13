"""The only module that talks to py-modelregistry (and MLflow).

Nothing personal leaves the job: params, metrics, run names and artifact files are checked against
allow-lists BEFORE any registry call, and the ALS artifact holds item factors only (never user factors).
"""

from __future__ import annotations

import json
import re
import tempfile
from collections.abc import Mapping
from pathlib import Path
from typing import Any

import modelregistry
from modelregistry import Decision, RegistryUnavailable
from modelregistry.client import Session

ALS_MODEL = "recsys-als"
GBDT_MODEL = "recsys-gbdt"
TWO_TOWER_MODEL = "recsys-two-tower"

DRIFT_SKETCHES_FILE = "drift_sketches.json"
_ITEM_FACTORS_FILE = "item_factors.parquet"

# Hyperparameters and aggregate settings only; anything else (user ids, per-user values) is refused.
_ALLOWED_PARAMS = frozenset(
    {
        "als_rank",
        "als_reg_param",
        "als_alpha",
        "als_max_iter",
        "top_n",
        "two_tower_dim",
        "two_tower_epochs",
        "two_tower_lr",
        "two_tower_batch_size",
        "two_tower_temperature",
        "two_tower_max_pairs",
        "two_tower_holdout_fraction",
        "gbdt_trees",
        "gbdt_max_depth",
        "gbdt_learning_rate",
        "gbdt_min_leaf",
        "gbdt_holdout_fraction",
    }
)
_METRIC_RE = re.compile(r"^[a-z][a-z0-9_]*(@\d+)?$")
_RUN_NAME_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
# Item-side artifact files only.
_ALLOWED_ARTIFACT_FILES = frozenset(
    {_ITEM_FACTORS_FILE, "item_vectors.parquet", "ranker.json", "weights.json"}
)

__all__ = [
    "ALS_MODEL",
    "GBDT_MODEL",
    "TWO_TOWER_MODEL",
    "Decision",
    "PayloadRejected",
    "RegistryUnavailable",
    "build_als_artifact",
    "champion_drift_baseline",
    "evaluate",
    "preflight",
    "start_run",
    "write_drift_sketches",
]


class PayloadRejected(ValueError):
    """The payload is not allow-listed; nothing was sent to the registry."""


def _check_params(params: Mapping[str, Any]) -> None:
    for k, v in params.items():
        if k not in _ALLOWED_PARAMS:
            raise PayloadRejected(f"param {k!r} is not allow-listed")
        if isinstance(v, bool) or not isinstance(v, (int, float)):
            raise PayloadRejected(f"param {k!r} must be numeric")


def _check_metrics(metrics: Mapping[str, Any]) -> None:
    for k, v in metrics.items():
        if not isinstance(k, str) or not _METRIC_RE.match(k):
            raise PayloadRejected(f"metric name {k!r} is not allow-listed")
        if isinstance(v, bool) or not isinstance(v, (int, float)):
            raise PayloadRejected(f"metric {k!r} must be numeric")


def _check_artifact_dir(artifact_dir: Path) -> None:
    for p in Path(artifact_dir).rglob("*"):
        if p.is_dir():
            raise PayloadRejected(f"artifact directory {p.name!r} is not allow-listed")
        if p.name not in _ALLOWED_ARTIFACT_FILES:
            raise PayloadRejected(f"artifact file {p.name!r} is not allow-listed")


def preflight() -> None:
    """Raise modelregistry.RegistryUnavailable unless the registry answers."""
    modelregistry.check_reachable()


def start_run(run_name: str, params: Mapping[str, Any]) -> str:
    """Create the MLflow run and log the allow-listed params; returns the run id."""
    if not _RUN_NAME_RE.match(run_name or ""):
        raise PayloadRejected(f"run name {run_name!r} is not allow-listed")
    _check_params(params)
    s = Session()
    run = s.call(s.client.create_run, "0", run_name=run_name)
    run_id: str = run.info.run_id
    for k, v in params.items():
        s.call(s.client.log_param, run_id, k, v)
    return run_id


def evaluate(
    model: str,
    run_id: str,
    metrics: Mapping[str, float],
    artifact_dir: Path,
    lineage: Mapping[str, Any],
) -> Decision:
    """Register a version of ``model`` and decide it against the policy. Moves no alias."""
    _check_metrics(metrics)
    _check_artifact_dir(Path(artifact_dir))
    return modelregistry.evaluate(model, run_id, metrics, artifact_dir, lineage)


def build_als_artifact(als: Any, out_dir: Path) -> Path:
    """Write the item factors (listing_id, features) of a fitted ALS model; user factors are never read."""
    out = Path(out_dir)
    out.mkdir(parents=True, exist_ok=True)
    df = als.item_factors.toPandas()[["listing_id", "features"]]
    df.to_parquet(out / _ITEM_FACTORS_FILE, index=False)
    return out


def write_drift_sketches(run_id: str, sketches: Mapping[str, Any]) -> None:
    """Log the drift sketches (aggregate quantiles) as a JSON artifact of the run."""
    s = Session()
    s.call(s.client.log_text, run_id, json.dumps(dict(sketches)), DRIFT_SKETCHES_FILE)


def champion_drift_baseline(model: str = ALS_MODEL) -> tuple[int, dict[str, Any]] | None:
    """(version, sketches) of the champion's run, or None without a champion or sketches artifact."""
    champ = modelregistry.get_champion(model)
    if champ is None:
        return None
    s = Session()
    try:
        with tempfile.TemporaryDirectory() as tmp:
            local = s.call(s.client.download_artifacts, champ["run_id"], DRIFT_SKETCHES_FILE, tmp)
            return int(champ["version"]), json.loads(Path(local).read_text(encoding="utf-8"))
    except RegistryUnavailable:
        raise
    except Exception:  # noqa: BLE001 - a missing artifact means "no baseline"
        return None
