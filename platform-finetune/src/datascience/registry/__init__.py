"""Publish through the shared model registry (py-modelregistry, MLflow).

Both register sites (`pipeline/stages.py`, `tune/promote.py`) call `publish`: the model is already
on disk, the shared gate decides, and a rejection is a normal outcome (no exception).
"""

import hashlib
import json

from datascience import log
from datascience.utils.io import read_data_version


def train_hw() -> str:
    """`cpu` on a host without a GPU, else the perf platform's hardware id."""
    from datascience.perf import hw

    gpus, _ = hw._gpus()
    return hw._hw_id(gpus) if gpus else "cpu"


def eval_protocol(params) -> str:
    """12 hex chars identifying how the metrics were measured: test-split identity (dataset name,
    split config, data version) plus the metric config. Equal inputs -> equal id."""
    d, m = params.data, params.metrics
    ident = {
        "dataset": params.ingestion.get("dataset_name"),
        "split": {"val_size": d.val_size, "test_size": d.test_size, "seed": d.seed},
        "data_version": read_data_version(),
        "metrics": {"compute": list(m.compute), "objective": m.objective},
    }
    return hashlib.sha256(json.dumps(ident, sort_keys=True, default=str).encode()).hexdigest()[:12]


def preflight() -> None:
    """Fail before any work (RegistryUnavailable) when the registry does not answer."""
    from modelregistry import check_reachable

    check_reachable()


def publish(cm, run_id, metrics: dict, model_dir):
    """Submit `model_dir` + scalar `metrics` for the trainer's `run_id`; returns the Decision."""
    from modelregistry import submit

    if not run_id:
        raise ValueError("no MLflow run id (train_stats.json run_id) - the registry needs tracking.backend: mlflow")
    scalar = {k: v for k, v in metrics.items() if isinstance(v, (int, float)) and not isinstance(v, bool)}
    data_version = read_data_version() or {}
    lineage = {
        "data_version": data_version.get("sha256"),
        "train_hw": train_hw(),
        "eval_protocol": eval_protocol(cm.params),
    }
    decision = submit(cm.registry().name, run_id, scalar, str(model_dir), lineage)
    log.info(f"registry: {cm.registry().name} v{decision.version} {decision.outcome} ({decision.reason})")
    return decision
