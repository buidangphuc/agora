"""Shared fixtures for the py-modelregistry offline suite.

State is prepared with the raw MLflow client (never through the library under test)
against a file store in tmp_path. Network faults use local stub TCP listeners.
"""
from __future__ import annotations

import os
import socket
import subprocess
import sys
import threading
from pathlib import Path
from typing import Any, Iterator

import pytest
import yaml
from mlflow import MlflowClient
from mlflow.exceptions import MlflowException

def enc(key: str) -> str:
    """MLflow rejects '@' in metric names (ndcg@10). INTERFACE ASSUMPTION: the library stores
    a metric `ndcg@10` on the run as `ndcg_at_10` and decodes it back when reading."""
    return key.replace("@", "_at_")


def dec(key: str) -> str:
    return key.replace("_at_", "@")


SRC = str(Path(__file__).resolve().parent.parent / "src")


class Env:
    """Raw-MLflow helper bound to the current MLFLOW_TRACKING_URI."""

    def __init__(self, root: Path, monkeypatch: pytest.MonkeyPatch) -> None:
        self.root = root
        self._mp = monkeypatch
        self.policies_path = root / "policies.yaml"
        self._counter = 0
        self.use_store("store")
        monkeypatch.setenv("MODELREGISTRY_POLICIES", str(self.policies_path))
        self.set_policies({"m": {"primary": "ndcg@10", "min_relative_improvement": 0.01}})

    # -- registry selection -------------------------------------------------
    def use_store(self, name: str) -> None:
        store = self.root / name
        self.tracking_uri = f"file:{store}"
        self._mp.setenv("MLFLOW_TRACKING_URI", self.tracking_uri)
        self.client = MlflowClient(tracking_uri=self.tracking_uri, registry_uri=self.tracking_uri)

    # -- policies -----------------------------------------------------------
    def set_policies(self, policies: dict[str, dict[str, Any]], path: Path | None = None) -> Path:
        target = path or self.policies_path
        target.write_text(yaml.safe_dump(policies))
        return target

    # -- raw state builders -------------------------------------------------
    def new_run(self) -> str:
        return self.client.create_run("0").info.run_id

    def artifact_dir(self, name: str = "artifact") -> Path:
        self._counter += 1
        d = self.root / f"{name}-{self._counter}"
        d.mkdir()
        (d / "weights.bin").write_text("w")
        return d

    def add_version(
        self,
        model: str,
        metrics: dict[str, float] | None = None,
        protocol: str | None = "p1",
        outcome: str | None = "promoted",
        alias: str | None = None,
        steps: dict[str, list[float]] | None = None,
    ) -> int:
        rid = self.new_run()
        for k, v in (metrics or {}).items():
            self.client.log_metric(rid, enc(k), v, step=0)
        for k, series in (steps or {}).items():
            for i, v in enumerate(series):
                self.client.log_metric(rid, enc(k), v, step=i)
        self.client.log_text(rid, "w", "model/weights.txt")
        try:
            self.client.create_registered_model(model)
        except MlflowException:
            pass
        mv = self.client.create_model_version(model, f"runs:/{rid}/model", run_id=rid)
        if protocol is not None:
            self.client.set_model_version_tag(model, mv.version, "lineage.eval_protocol", protocol)
        if outcome is not None:
            self.client.set_model_version_tag(model, mv.version, "gate.outcome", outcome)
        if alias:
            self.client.set_registered_model_alias(model, alias, mv.version)
        return int(mv.version)

    def set_alias(self, model: str, alias: str, version: int) -> None:
        self.client.set_registered_model_alias(model, alias, str(version))

    # -- raw state readers --------------------------------------------------
    def aliases(self, model: str) -> dict[str, int]:
        try:
            rm = self.client.get_registered_model(model)
        except MlflowException:
            return {}
        return {k: int(v) for k, v in (rm.aliases or {}).items()}

    def versions(self, model: str) -> list[int]:
        return sorted(
            int(v.version) for v in self.client.search_model_versions(f"name='{model}'")
        )

    def tags(self, model: str, version: int) -> dict[str, str]:
        return dict(self.client.get_model_version(model, str(version)).tags)

    def run_id(self, model: str, version: int) -> str:
        return str(self.client.get_model_version(model, str(version)).run_id)

    def run_metrics(self, run_id: str) -> dict[str, float]:
        return {dec(k): v for k, v in self.client.get_run(run_id).data.metrics.items()}

    def model_files(self, run_id: str) -> list[str]:
        return [f.path for f in self.client.list_artifacts(run_id, "model")]

    def snapshot(self, model: str) -> dict[str, Any]:
        """Aliases, versions and every version's tags, for 'nothing changed' asserts."""
        return {
            "aliases": self.aliases(model),
            "tags": {v: self.tags(model, v) for v in self.versions(model)},
        }

    # -- convenience around the library ------------------------------------
    def candidate(self, metrics: dict[str, float], protocol: str | None = "p1",
                  hw: str | None = None) -> dict[str, Any]:
        lineage: dict[str, Any] = {"data_version": "d@v1"}
        if protocol is not None:
            lineage["eval_protocol"] = protocol
        if hw is not None:
            lineage["train_hw"] = hw
        return {
            "run_id": self.new_run(),
            "metrics": metrics,
            "artifact_path": str(self.artifact_dir()),
            "lineage": lineage,
        }


@pytest.fixture
def env(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Env:
    monkeypatch.delenv("MLFLOW_HTTP_REQUEST_TIMEOUT", raising=False)
    monkeypatch.delenv("MLFLOW_REGISTRY_URI", raising=False)
    return Env(tmp_path, monkeypatch)


def run_cli(*args: str, timeout: float = 90.0) -> subprocess.CompletedProcess[str]:
    """Run `python -m modelregistry ...` in a subprocess with the current os.environ."""
    child_env = dict(os.environ)
    child_env["PYTHONPATH"] = SRC + os.pathsep + child_env.get("PYTHONPATH", "")
    return subprocess.run(
        [sys.executable, "-m", "modelregistry", *args],
        capture_output=True, text=True, env=child_env, timeout=timeout, check=False,
    )


# -- stub TCP listeners ------------------------------------------------------
class StubServer:
    """Accepts connections. mode='hang': never replies. mode='once': replies to the
    first HTTP request with `200 {}` and then accepts but never replies again."""

    def __init__(self, mode: str) -> None:
        self.mode = mode
        self.sock = socket.socket()
        self.sock.bind(("127.0.0.1", 0))
        self.sock.listen(64)
        self.port = self.sock.getsockname()[1]
        self.url = f"http://127.0.0.1:{self.port}"
        self._held: list[socket.socket] = []
        self._answered = 0
        self._lock = threading.Lock()
        self._stop = False
        threading.Thread(target=self._accept_loop, daemon=True).start()

    def _accept_loop(self) -> None:
        while not self._stop:
            try:
                conn, _ = self.sock.accept()
            except OSError:
                return
            threading.Thread(target=self._handle, args=(conn,), daemon=True).start()

    def _handle(self, conn: socket.socket) -> None:
        self._held.append(conn)
        if self.mode != "once":
            return
        try:
            buf = b""
            while b"\r\n\r\n" not in buf:
                chunk = conn.recv(65536)
                if not chunk:
                    return
                buf += chunk
            head, _, rest = buf.partition(b"\r\n\r\n")
            length = 0
            for line in head.split(b"\r\n"):
                if line.lower().startswith(b"content-length:"):
                    length = int(line.split(b":", 1)[1])
            while len(rest) < length:
                rest += conn.recv(65536)
            with self._lock:
                first = self._answered == 0
                self._answered += 1
            if first:
                body = b"{}"
                conn.sendall(
                    b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n"
                    + b"Content-Length: 2\r\nConnection: close\r\n\r\n" + body
                )
                conn.close()
        except OSError:
            return

    def close(self) -> None:
        self._stop = True
        try:
            self.sock.close()
        except OSError:
            pass
        for c in self._held:
            try:
                c.close()
            except OSError:
                pass


@pytest.fixture
def hang_server() -> Iterator[StubServer]:
    s = StubServer("hang")
    yield s
    s.close()


@pytest.fixture
def once_then_stall_server() -> Iterator[StubServer]:
    s = StubServer("once")
    yield s
    s.close()


@pytest.fixture
def closed_port_url() -> str:
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return f"http://127.0.0.1:{port}"
