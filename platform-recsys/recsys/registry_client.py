"""The only module that talks to py-modelregistry (and MLflow).

Nothing personal leaves the job: params, metrics, run names and artifact files are checked against
allow-lists BEFORE any registry call, and the ALS artifact holds item factors only (never user factors).
"""

from __future__ import annotations

import json
import math
import re
import tempfile
from collections.abc import Mapping
from pathlib import Path
from typing import Any

import modelregistry
from mlflow.entities import Param
from mlflow.exceptions import MlflowException
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
_REASON_RE = re.compile(r"^[A-Za-z0-9 _.:@<=()/%+-]{1,200}$")
_USER_KEY_RE = re.compile(r"u-\d{4}")
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
    "alias_generation",
    "alias_info",
    "apply",
    "build_als_artifact",
    "champion_drift_baseline",
    "champion_generation",
    "dataset_lineage",
    "evaluate",
    "log_drift",
    "mark_rejected",
    "preflight",
    "register_rejected",
    "restore_aliases",
    "rollback_champion",
    "snapshot_lineage",
    "start_run",
    "write_drift_sketches",
]

# The four distributions a drift sketch may describe (recsys.monitoring.generation.FEATURES).
_SKETCH_FEATURES = ("weight", "user_items", "item_users", "top_score")
_SCALARS = (str, int, float, bool, type(None))


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


def _check_reason(reason: Any) -> None:
    """A rejection reason becomes a registry tag: short, a bounded charset, never a user key."""
    if not isinstance(reason, str) or not _REASON_RE.fullmatch(reason) or _USER_KEY_RE.search(reason):
        raise PayloadRejected("rejection reason is not allow-listed (length, charset or user key)")


def _check_lineage(lineage: Mapping[str, Any]) -> None:
    """Lineage becomes MLflow tags: scalars only (a mapping or a list could carry per-user data)."""
    for k, v in lineage.items():
        if not isinstance(k, str) or not isinstance(v, _SCALARS):
            raise PayloadRejected(f"lineage {k!r} must be a scalar")


def _is_number(v: Any) -> bool:
    return isinstance(v, (int, float)) and not isinstance(v, bool)


def _check_sketches(sketches: Mapping[str, Any]) -> None:
    for k, v in sketches.items():
        if k not in _SKETCH_FEATURES:
            raise PayloadRejected(f"drift sketch {k!r} is not allow-listed")
        if not isinstance(v, (list, tuple)) or not all(_is_number(x) and math.isfinite(x) for x in v):
            raise PayloadRejected(f"drift sketch {k!r} must be a list of finite numbers")


def _check_artifact_dir(artifact_dir: Path) -> None:
    root = Path(artifact_dir)
    if root.is_symlink():
        raise PayloadRejected("artifact directory must not be a symlink")
    for p in root.rglob("*"):
        if p.is_symlink():
            raise PayloadRejected(f"artifact entry {p.name!r} is a symlink")
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
    _check_lineage(lineage)
    return modelregistry.evaluate(model, run_id, metrics, artifact_dir, lineage)


def register_rejected(model: str, run_id: str, reason: str, lineage: Mapping[str, Any]) -> Decision:
    """Register a version as rejected without evaluating the policy (a structural rejection)."""
    _check_reason(reason)
    _check_lineage(lineage)
    return modelregistry.register_rejected(model, run_id, reason, lineage)


def apply(model: str, version: int) -> Decision:
    """Move ``champion`` to a promotable version (after the generation was published)."""
    return modelregistry.apply(model, version)


def mark_rejected(model: str, version: int, reason: str) -> None:
    """Record a stage or publish failure on an evaluated version; touches no alias."""
    _check_reason(reason)
    modelregistry.mark_rejected(model, version, reason)


def rollback_champion(model: str = ALS_MODEL) -> Decision:
    """Swap ``champion`` and ``previous`` (raises RollbackError without a previous)."""
    return modelregistry.rollback(model)


def champion_generation(model: str = ALS_MODEL) -> str | None:
    """The generation id (``lineage.model_version`` tag) the champion was built for, or None."""
    champ = modelregistry.get_champion(model)
    if champ is None:
        return None
    return champ["tags"].get("lineage.model_version")


def alias_generation(alias: str, model: str = ALS_MODEL) -> str | None:
    """The generation id (``lineage.model_version`` tag) of the version ``alias`` names, or None."""
    s = Session()
    try:
        mv = s.call(s.client.get_model_version_by_alias, model, alias)
    except MlflowException as exc:
        if "not found" in str(exc).lower() or getattr(exc, "error_code", "") == "RESOURCE_DOES_NOT_EXIST":
            return None
        raise
    return mv.tags.get("lineage.model_version")


def alias_info(model: str, alias: str) -> dict[str, Any] | None:
    """``{version, run_id, generation, has_artifact}`` of the version ``alias`` names, or None.

    ``generation`` is the ``lineage.model_version`` tag; ``has_artifact`` is False for a metrics-only
    version (one made by ``seed`` without an artifact)."""
    s = Session()
    try:
        mv = s.call(s.client.get_model_version_by_alias, model, alias)
    except MlflowException as exc:
        if "not found" in str(exc).lower() or getattr(exc, "error_code", "") == "RESOURCE_DOES_NOT_EXIST":
            return None
        raise
    files = s.call(s.client.list_artifacts, mv.run_id, "model") if mv.run_id else []
    return {
        "version": int(mv.version),
        "run_id": mv.run_id,
        "generation": mv.tags.get("lineage.model_version"),
        "has_artifact": bool(files),
    }


def restore_aliases(model: str, champion: int, previous: int | None) -> None:
    """Write ``champion`` (and ``previous``) back to the given versions: the undo of a rollback swap."""
    s = Session()
    if previous is not None:
        s.call(s.client.set_registered_model_alias, model, "previous", str(previous))
    s.call(s.client.set_registered_model_alias, model, "champion", str(champion))


def dataset_lineage(lineage: Mapping[str, Any]) -> dict[str, Any]:
    """Scalar lineage of a governed dataset (name@version, as_of, sha256)."""
    return {
        "dataset": f"{lineage['name']}@v{lineage['version']}",
        "dataset_as_of": lineage["as_of"],
        "dataset_sha256": lineage["sha256"],
    }


def snapshot_lineage(prefix: str, lineage: Mapping[str, Any] | None) -> dict[str, Any]:
    """Scalar lineage of a feature snapshot (series): ``<prefix>`` = ``name:sha256`` entries."""
    if not lineage:
        return {}
    entries = lineage.get("snapshots")
    if entries is None:
        entries = [lineage]
    return {prefix: ",".join(f"{e['snapshot']}:{e['sha256']}" for e in entries)}


def log_drift(run_id: str, record: Mapping[str, Any]) -> None:
    """Record the drift verdict of the run as ``drift.*`` params (status, baseline, per-feature PSI)."""
    params: dict[str, str] = {"drift.status": str(record["status"])}
    if record.get("baseline_version") is not None:
        params["drift.baseline_version"] = str(record["baseline_version"])
    for name in _SKETCH_FEATURES:
        res = (record.get("features") or {}).get(name)
        if res:
            params[f"drift.{name}.psi"] = str(res["psi"])
            params[f"drift.{name}.level"] = str(res["drift_level"])
    s = Session()
    s.call(s.client.log_batch, run_id, params=[Param(k, v) for k, v in params.items()])


def build_als_artifact(als: Any, out_dir: Path) -> Path:
    """Write the item factors (listing_id, features) of a fitted ALS model; user factors are never read."""
    out = Path(out_dir)
    out.mkdir(parents=True, exist_ok=True)
    df = als.item_factors.toPandas()[["listing_id", "features"]]
    df.to_parquet(out / _ITEM_FACTORS_FILE, index=False)
    return out


def write_drift_sketches(run_id: str, sketches: Mapping[str, Any]) -> None:
    """Log the drift sketches (aggregate quantiles) as a JSON artifact of the run."""
    _check_sketches(sketches)
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
