"""Promote a tuning variant into the shared registry - manual, you pick the winner.

Reads the loop's ranking (artifacts/tune_runs/results.json) and submits the chosen variant's
snapshotted model to py-modelregistry with the MLflow run the loop trained it in. The shared gate
decides whether it becomes `champion`; a rejection is recorded on the version, not raised. The
variant's resolved config snapshot is what the eval protocol id is derived from.
"""

import json

from datascience import log, registry
from datascience.config.configuration import ConfigurationManager
from datascience.constants import paths


def promote(variant: str):
    if not paths.TUNE_RESULTS.exists():
        raise SystemExit("no tuning results - run `python main.py tune` first")

    results = json.loads(paths.TUNE_RESULTS.read_text())
    entry = next((r for r in results["runs"] if r["variant"] == variant), None)
    if entry is None:
        choices = [r["variant"] for r in results["runs"]]
        raise SystemExit(f"variant '{variant}' not found; choices: {choices}")
    if "error" in entry:
        raise SystemExit(f"variant '{variant}' failed during tuning: {entry['error']}")

    run_dir = paths.TUNE_DIR / variant
    metrics = json.loads((run_dir / "metrics.json").read_text())
    cm = ConfigurationManager(params_filepath=run_dir / "config.yaml")  # the variant's own config
    decision = registry.publish(cm, entry.get("run_id"), metrics, run_dir / "model")
    log.info(f"promote '{variant}' -> {cm.registry().name} v{decision.version}: {decision.outcome}")
    return decision
