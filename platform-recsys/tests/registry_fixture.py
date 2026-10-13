"""Helpers for registry-backed tests: a local MLflow file store in tmp_path (no server).

State is prepared with the raw MLflow client, never through the code under test. The packaged
policies.yaml is used (MODELREGISTRY_POLICIES unset) so the shipped policy numbers are what is tested.
"""

from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest
from mlflow import MlflowClient
from mlflow.exceptions import MlflowException


def enc(key: str) -> str:
    """MLflow rejects '@' in metric names; the library stores ndcg@10 as ndcg_at_10."""
    return key.replace("@", "_at_")


def dec(key: str) -> str:
    return key.replace("_at_", "@")


class RegEnv:
    def __init__(self, root: Path, monkeypatch: pytest.MonkeyPatch) -> None:
        self.root = root
        self.tracking_uri = f"file:{root / 'store'}"
        monkeypatch.setenv("MLFLOW_TRACKING_URI", self.tracking_uri)
        monkeypatch.delenv("MODELREGISTRY_POLICIES", raising=False)
        monkeypatch.delenv("MLFLOW_REGISTRY_URI", raising=False)
        monkeypatch.delenv("MLFLOW_HTTP_REQUEST_TIMEOUT", raising=False)
        self.client = MlflowClient(tracking_uri=self.tracking_uri, registry_uri=self.tracking_uri)
        self._n = 0

    def new_run(self) -> str:
        return self.client.create_run("0").info.run_id

    def artifact_dir(self) -> Path:
        self._n += 1
        d = self.root / f"artifact-{self._n}"
        d.mkdir()
        (d / "weights.bin").write_text("w")
        return d

    def add_version(
        self, model: str, metrics: dict[str, float], protocol: str | None = "p1", alias: str | None = "champion"
    ) -> int:
        rid = self.new_run()
        for k, v in metrics.items():
            self.client.log_metric(rid, enc(k), v, step=0)
        self.client.log_text(rid, "w", "model/weights.txt")
        try:
            self.client.create_registered_model(model)
        except MlflowException:
            pass
        mv = self.client.create_model_version(model, f"runs:/{rid}/model", run_id=rid)
        if protocol is not None:
            self.client.set_model_version_tag(model, mv.version, "lineage.eval_protocol", protocol)
        self.client.set_model_version_tag(model, mv.version, "gate.outcome", "promoted")
        if alias:
            self.client.set_registered_model_alias(model, alias, mv.version)
        return int(mv.version)

    def evaluate(self, model: str, metrics: dict[str, float], protocol: str | None = "p1") -> Any:
        from modelregistry import evaluate

        lineage: dict[str, Any] = {"data_version": "d@v1"}
        if protocol is not None:
            lineage["eval_protocol"] = protocol
        return evaluate(model, self.new_run(), metrics, str(self.artifact_dir()), lineage)

    def tags(self, model: str, version: int) -> dict[str, str]:
        return dict(self.client.get_model_version(model, str(version)).tags)

    def aliases(self, model: str) -> dict[str, int]:
        try:
            rm = self.client.get_registered_model(model)
        except MlflowException:
            return {}
        return {k: int(v) for k, v in (rm.aliases or {}).items()}


@pytest.fixture
def reg(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> RegEnv:
    return RegEnv(tmp_path, monkeypatch)
