"""Registry operations: two-phase gate (evaluate/apply), rollback, promote, seed."""
from __future__ import annotations

import math
import os
import time
from collections.abc import Mapping
from pathlib import Path
from typing import Any

from mlflow.entities import Metric
from mlflow.exceptions import MlflowException

from .client import Session
from .errors import GateError, RegistryUnavailable, RollbackError
from .gate import Decision, decide
from .policy import load_policy

CHAMPION = "champion"
PREVIOUS = "previous"
_MAX_TAG = 5000
PathLike = str | os.PathLike[str]


def enc(key: str) -> str:
    """MLflow rejects '@' in metric names: ndcg@10 is stored as ndcg_at_10."""
    return key.replace("@", "_at_")


def dec(key: str) -> str:
    return key.replace("_at_", "@")


def _not_found(e: MlflowException) -> bool:
    return getattr(e, "error_code", "") == "RESOURCE_DOES_NOT_EXIST"


def _tag(s: Session, model: str, version: int | str, key: str, value: str) -> None:
    s.call(s.client.set_model_version_tag, model, str(version), key, value[:_MAX_TAG])


def _get_version(s: Session, model: str, version: int | str) -> Any:
    try:
        return s.call(s.client.get_model_version, model, str(version))
    except MlflowException as e:
        if _not_found(e):
            raise GateError(f"version {version} of model {model!r} not found") from e
        raise


def _run_metrics(s: Session, run_id: str) -> dict[str, float]:
    run = s.call(s.client.get_run, run_id)
    return {dec(k): float(v) for k, v in run.data.metrics.items()}


def _alias_version(s: Session, model: str, alias: str) -> Any | None:
    try:
        return s.call(s.client.get_model_version_by_alias, model, alias)
    except MlflowException as e:
        if _not_found(e) or "not found" in str(e).lower():
            return None
        raise


def _champion_info(s: Session, model: str) -> tuple[Any, dict[str, float]] | None:
    mv = _alias_version(s, model, CHAMPION)
    if mv is None:
        return None
    if not isinstance(mv.run_id, str):
        raise RegistryUnavailable(f"malformed registry response for {model!r} {CHAMPION}")
    return mv, _run_metrics(s, mv.run_id)


def _ensure_model(s: Session, model: str) -> None:
    try:
        s.call(s.client.get_registered_model, model)
    except MlflowException as e:
        if not _not_found(e):
            raise
        try:
            s.call(s.client.create_registered_model, model)
        except MlflowException as e2:
            if getattr(e2, "error_code", "") != "RESOURCE_ALREADY_EXISTS":
                raise


def _log_metrics(s: Session, run_id: str, metrics: Mapping[str, float]) -> None:
    now = int(time.time() * 1000)
    batch = [Metric(enc(k), float(v), now, 0) for k, v in metrics.items()
             if v is not None and math.isfinite(float(v))]
    if batch:
        s.call(s.client.log_batch, run_id, metrics=batch)


def _log_artifact(s: Session, run_id: str, artifact_path: PathLike | None) -> None:
    if artifact_path is None:
        return
    p = Path(artifact_path)
    if p.is_dir():
        s.call(s.client.log_artifacts, run_id, str(p), "model")
    else:
        s.call(s.client.log_artifact, run_id, str(p), "model")


def _create_version(s: Session, model: str, run_id: str, lineage: Mapping[str, Any]) -> int:
    _ensure_model(s, model)
    mv = s.call(s.client.create_model_version, model, f"runs:/{run_id}/model", run_id=run_id)
    version = int(mv.version)
    for k, v in lineage.items():
        if v is not None:
            _tag(s, model, version, f"lineage.{k}", str(v))
    return version


def _set_gate(s: Session, model: str, version: int | str, outcome: str, reason: str) -> None:
    """Reason first, outcome last: a crash in between leaves the version `pending`."""
    _tag(s, model, version, "gate.reason", reason)
    _tag(s, model, version, "gate.outcome", outcome)


def _evaluate(s: Session, model: str, run_id: str, metrics: Mapping[str, float],
              artifact_path: PathLike | None, lineage: Mapping[str, Any],
              policies_path: PathLike | None) -> Decision:
    policy = load_policy(model, policies_path)
    champ = _champion_info(s, model)
    _log_metrics(s, run_id, metrics)
    _log_artifact(s, run_id, artifact_path)
    version = _create_version(s, model, run_id, lineage)
    champ_protocol = champ[0].tags.get("lineage.eval_protocol") if champ else None
    proto = lineage.get("eval_protocol")
    outcome, reason = decide(policy, metrics, champ[1] if champ else None,
                             None if proto is None else str(proto), champ_protocol)
    _set_gate(s, model, version, outcome, reason)
    return Decision(version, outcome, reason)


def _move_champion(s: Session, model: str, version: int | str) -> None:
    holder = _alias_version(s, model, CHAMPION)
    if holder is not None and str(holder.version) != str(version):
        s.call(s.client.set_registered_model_alias, model, PREVIOUS, str(holder.version))
    s.call(s.client.set_registered_model_alias, model, CHAMPION, str(version))


def _apply(s: Session, model: str, version: int | str) -> Decision:
    mv = _get_version(s, model, version)
    outcome = mv.tags.get("gate.outcome")
    if outcome != "promotable":
        raise GateError(
            f"version {version} of {model!r} is {outcome or 'pending'}, not promotable"
        )
    _move_champion(s, model, version)
    _tag(s, model, version, "gate.outcome", "promoted")
    return Decision(int(mv.version), "promoted", mv.tags.get("gate.reason", ""))


def evaluate(model: str, run_id: str, metrics: Mapping[str, float],
             artifact_path: PathLike | None, lineage: Mapping[str, Any],
             policies_path: PathLike | None = None) -> Decision:
    """Register a version, decide against the policy and tag it. Moves no alias."""
    return _evaluate(Session(), model, run_id, metrics, artifact_path, lineage, policies_path)


def apply(model: str, version: int | str) -> Decision:
    """Move `champion` (old holder -> `previous`) for a `promotable` version."""
    return _apply(Session(), model, version)


def submit(model: str, run_id: str, metrics: Mapping[str, float],
           artifact_path: PathLike | None, lineage: Mapping[str, Any],
           policies_path: PathLike | None = None) -> Decision:
    """evaluate, then apply if promotable."""
    s = Session()
    d = _evaluate(s, model, run_id, metrics, artifact_path, lineage, policies_path)
    if d.outcome == "promotable":
        return _apply(s, model, d.version)
    return d


def mark_rejected(model: str, version: int | str, reason: str) -> None:
    """Mark a version rejected (e.g. after a publish failure). Touches no alias."""
    _set_gate(Session(), model, version, "rejected", reason)


def register_rejected(model: str, run_id: str, reason: str,
                      lineage: Mapping[str, Any]) -> Decision:
    """Register a version as rejected without evaluating the policy."""
    s = Session()
    version = _create_version(s, model, run_id, lineage)
    _set_gate(s, model, version, "rejected", reason)
    return Decision(version, "rejected", reason)


def promote(model: str, version: int | str, force: bool = False,
            reason: str | None = None, policies_path: PathLike | None = None) -> Decision:
    """Promote an existing version. `force` skips the gate and requires a reason."""
    if force and (reason is None or not reason.strip()):
        raise ValueError("--reason is required with --force")
    s = Session()
    mv = _get_version(s, model, version)
    if force:
        assert reason is not None
        holder = _alias_version(s, model, CHAMPION)
        if holder is None or str(holder.version) != str(mv.version):
            _move_champion(s, model, version)
        _tag(s, model, version, "gate.forced", "true")
        _set_gate(s, model, version, "promoted", reason)
        return Decision(int(mv.version), "promoted", reason)
    policy = load_policy(model, policies_path)
    champ = _champion_info(s, model)
    if champ is not None and str(champ[0].version) == str(mv.version):
        # retry of a partially failed promote: only (re)write the outcome
        _tag(s, model, mv.version, "gate.outcome", "promoted")
        return Decision(int(mv.version), "promoted", mv.tags.get("gate.reason", ""))
    candidate = _run_metrics(s, mv.run_id)
    outcome, why = decide(policy, candidate, champ[1] if champ else None,
                          mv.tags.get("lineage.eval_protocol"),
                          champ[0].tags.get("lineage.eval_protocol") if champ else None)
    if outcome != "promotable":
        _set_gate(s, model, mv.version, "rejected", why)
        return Decision(int(mv.version), "rejected", why)
    _move_champion(s, model, mv.version)
    _set_gate(s, model, mv.version, "promoted", why)
    return Decision(int(mv.version), "promoted", why)


def rollback(model: str) -> Decision:
    """Swap `champion` and `previous`; a second rollback toggles back."""
    s = Session()
    try:
        rm = s.call(s.client.get_registered_model, model)
    except MlflowException as e:
        if _not_found(e):
            raise RollbackError(f"model {model!r} not found") from e
        raise
    aliases = dict(rm.aliases or {})
    if PREVIOUS not in aliases or CHAMPION not in aliases:
        raise RollbackError(f"model {model!r} has no previous version to roll back to")
    old, new = str(aliases[CHAMPION]), str(aliases[PREVIOUS])
    if old == new:
        # partial rollback (previous written, champion not): recover the target from the tag
        mv = _get_version(s, model, old)
        new = mv.tags.get("gate.rollback_target", "")
        if not new:
            raise RollbackError(f"model {model!r} has no previous version to roll back to")
    else:
        _tag(s, model, old, "gate.rollback_target", new)
    s.call(s.client.set_registered_model_alias, model, PREVIOUS, old)
    s.call(s.client.set_registered_model_alias, model, CHAMPION, new)
    reason = f"rollback from v{old}"
    _tag(s, model, new, "gate.reason", reason)
    return Decision(int(new), "promoted", reason)


def seed(model: str, metrics: Mapping[str, float], lineage: Mapping[str, Any], reason: str,
         artifact_path: PathLike | None = None) -> Decision:
    """Create the first champion without the gate. Only for a model with no versions."""
    if "'" in model or '"' in model:
        raise ValueError(f"invalid model name {model!r}: quotes are not allowed")
    s = Session()
    existing = s.call(s.client.search_model_versions, f"name='{model}'", max_results=1)
    if existing:
        raise GateError(f"model {model!r} already has versions; seed is only for an empty model")
    run_id = s.call(s.client.create_run, "0").info.run_id
    _log_metrics(s, run_id, metrics)
    _log_artifact(s, run_id, artifact_path)
    s.call(s.client.set_terminated, run_id)
    version = _create_version(s, model, run_id, lineage)
    why = f"seed: {reason}"
    _set_gate(s, model, version, "promoted", why)
    s.call(s.client.set_registered_model_alias, model, CHAMPION, str(version))
    return Decision(version, "promoted", why)


def get_champion(model: str) -> dict[str, Any] | None:
    """Return {version, run_id, metrics, tags} of the champion, or None."""
    s = Session()
    info = _champion_info(s, model)
    if info is None:
        return None
    mv, metrics = info
    return {"version": int(mv.version), "run_id": mv.run_id, "metrics": metrics,
            "tags": dict(mv.tags)}
