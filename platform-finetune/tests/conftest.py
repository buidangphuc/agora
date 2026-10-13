"""Shared fixtures: redirect the fixed artifact paths into a tmp tree, and a local MLflow
file-store registry (no server, no network) for the publish-gate tests.

Tests that write artifacts monkeypatch `datascience.constants.paths`
attributes; this fixture centralizes the common ones so individual tests
stop repeating the boilerplate.
"""

import json
import socket
from pathlib import Path

import pytest
from mlflow import MlflowClient
from mlflow.exceptions import MlflowException

from datascience.constants import paths

MODEL = "finetune-classifier"
TESTS_DIR = Path(__file__).resolve().parent
PROJECT = TESTS_DIR.parent


@pytest.fixture()
def tmp_artifacts(tmp_path, monkeypatch):
    """Point the artifact tree at a tmp dir; returns the tmp root."""
    mapping = {
        "ARTIFACTS": tmp_path,
        "REGISTRY_DIR": tmp_path / "registry",
        "REGISTRY_CACHE": tmp_path / "registry_cache",
        "EVAL_DIR": tmp_path / "model_evaluation",
        "METRICS_FILE": tmp_path / "model_evaluation" / "metrics.json",
        "PERF_DIR": tmp_path / "perf",
        "TUNE_DIR": tmp_path / "tune_runs",
        "TUNE_RESULTS": tmp_path / "tune_runs" / "results.json",
        "SPLIT_DIR": tmp_path / "data_transformation",
        "TRAIN_FILE": tmp_path / "data_transformation" / "train.jsonl",
        "VAL_FILE": tmp_path / "data_transformation" / "val.jsonl",
        "TEST_FILE": tmp_path / "data_transformation" / "test.jsonl",
        "LABEL_MAP": tmp_path / "data_transformation" / "label_map.json",
        "MODEL_DIR": tmp_path / "model_trainer" / "model",
        "CKPT_DIR": tmp_path / "model_trainer" / "checkpoints",
        "TRAIN_STATS": tmp_path / "model_trainer" / "train_stats.json",
        "RUN_STATUS": tmp_path / "run_status.json",
    }
    for name, value in mapping.items():
        monkeypatch.setattr(paths, name, value, raising=False)
    for d in ("EVAL_DIR", "SPLIT_DIR", "MODEL_DIR"):
        mapping[d].mkdir(parents=True, exist_ok=True)
    return tmp_path


# --- registry fixtures (local MLflow file store, no server) ----------------------------------

class Reg:
    """Raw-MLflow helper bound to a file store; state is built and read without the library."""

    def __init__(self, uri: str) -> None:
        self.uri = uri
        self.client = MlflowClient(tracking_uri=uri, registry_uri=uri)

    def new_run(self) -> str:
        return self.client.create_run("0").info.run_id

    def add_version(self, metrics, protocol="aaaaaaaaaaaa", outcome="promoted", alias=None) -> int:
        rid = self.new_run()
        for k, v in metrics.items():
            self.client.log_metric(rid, k, v, step=0)
        self.client.log_text(rid, "w", "model/weights.txt")
        try:
            self.client.create_registered_model(MODEL)
        except MlflowException:
            pass
        mv = self.client.create_model_version(MODEL, f"runs:/{rid}/model", run_id=rid)
        if protocol is not None:
            self.client.set_model_version_tag(MODEL, mv.version, "lineage.eval_protocol", protocol)
        if outcome is not None:
            self.client.set_model_version_tag(MODEL, mv.version, "gate.outcome", outcome)
        if alias:
            self.client.set_registered_model_alias(MODEL, alias, mv.version)
        return int(mv.version)

    def aliases(self) -> dict:
        try:
            rm = self.client.get_registered_model(MODEL)
        except MlflowException:
            return {}
        return {k: int(v) for k, v in (rm.aliases or {}).items()}

    def versions(self) -> list:
        try:
            found = self.client.search_model_versions(f"name='{MODEL}'")
        except MlflowException:
            return []
        return sorted(int(v.version) for v in found)

    def tags(self, version: int) -> dict:
        return dict(self.client.get_model_version(MODEL, str(version)).tags)

    def run_id(self, version: int) -> str:
        return str(self.client.get_model_version(MODEL, str(version)).run_id)

    def source(self, version: int) -> str:
        return str(self.client.get_model_version(MODEL, str(version)).source)

    def snapshot(self) -> dict:
        return {"aliases": self.aliases(), "tags": {v: self.tags(v) for v in self.versions()}}


@pytest.fixture()
def reg(tmp_path, monkeypatch):
    """Registry = a local file store; policies = the packaged shared policies.yaml."""
    uri = f"file:{tmp_path / 'mlruns'}"
    monkeypatch.setenv("MLFLOW_TRACKING_URI", uri)
    monkeypatch.delenv("MLFLOW_REGISTRY_URI", raising=False)
    monkeypatch.delenv("MODELREGISTRY_POLICIES", raising=False)
    monkeypatch.delenv("MLFLOW_HTTP_REQUEST_TIMEOUT", raising=False)
    return Reg(uri)


@pytest.fixture()
def closed_port_url():
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return f"http://127.0.0.1:{port}"


def lineage(protocol="aaaaaaaaaaaa", **extra):
    return {"data_version": "abc123def456", "train_hw": "cpu", "eval_protocol": protocol, **extra}


def submit_candidate(reg, metrics, protocol="aaaaaaaaaaaa"):
    """Submit through the shared gate with the shipped finetune-classifier policy."""
    from modelregistry import submit

    art = Path(reg.uri.removeprefix("file:")).parent / f"art-{len(reg.versions())}"
    art.mkdir(exist_ok=True)
    (art / "weights.bin").write_text("w")
    return submit(MODEL, reg.new_run(), metrics, str(art), lineage(protocol))


# --- pipeline-stage seams (heavy components faked, registry real) ----------------------------

@pytest.fixture()
def stage_env(tmp_artifacts, reg, monkeypatch):
    """Run the real `run_evaluate` against the file store.

    INTERFACE ASSUMPTIONS: the trainer persists its MLflow run id as `run_id` in
    paths.TRAIN_STATS (train_stats.json); `stages.ModelEvaluation` is the evaluation component;
    `stages._cm()` builds the ConfigurationManager.
    """
    from datascience.pipeline import stages

    monkeypatch.setattr(paths, "ensure_dirs", lambda: None)
    monkeypatch.setattr(paths, "DATA_VERSION", tmp_artifacts / "data_transformation" / "data_version.json",
                        raising=False)
    write_data_version(paths.DATA_VERSION)

    class StageEnv:
        root = tmp_artifacts
        registry = reg

        def run_evaluate(self, metrics, run_id=None, cm=None):
            """Pretend training produced a model and evaluation produced `metrics`."""
            paths.MODEL_DIR.mkdir(parents=True, exist_ok=True)
            (paths.MODEL_DIR / "model.safetensors").write_text("w")
            paths.TRAIN_STATS.parent.mkdir(parents=True, exist_ok=True)
            run_id = run_id or reg.new_run()
            paths.TRAIN_STATS.write_text(json.dumps({"run_id": run_id, "samples_per_s": 1.0}))

            class FakeEval:
                def __init__(self, cfg):
                    pass

                def evaluate(self):
                    paths.METRICS_FILE.parent.mkdir(parents=True, exist_ok=True)
                    paths.METRICS_FILE.write_text(json.dumps({**metrics, "per_class": {"a": {"f1": 1.0}}}))

            monkeypatch.setattr(stages, "ModelEvaluation", FakeEval)
            if cm is not None:
                monkeypatch.setattr(stages, "_cm", lambda: cm)
            stages.run_evaluate()
            return run_id

    return StageEnv()


def write_data_version(path, sha="abc123def456"):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps({"sha256": sha, "n_rows": 10}))
