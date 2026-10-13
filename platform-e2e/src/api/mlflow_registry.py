"""MLflow tracking-server REST client for the recsys registry e2e (change recsys-auto-train).

Black box: everything here goes through the MLflow REST API of the stack (default
http://localhost:5050); nothing imports recsys code. Registry champions are seeded by running the
monorepo copy of `py-modelregistry` (its public `seed`) in a subprocess, exactly as
model_registry_steps does, so the e2e never imports implementation code itself.

    MLFLOW_URL   default http://localhost:5050

Metric names are stored by the library with `@` encoded, so `ndcg@10` reads back as `ndcg_at_10`.
"""

from __future__ import annotations

import os
import re
import subprocess
import sys
import textwrap
import time
from dataclasses import dataclass, field
from pathlib import Path

import httpx

_REPO = Path(__file__).resolve().parents[3]
LIB_SRC = _REPO / "platform-core" / "packages" / "py-modelregistry" / "src"

RECSYS_MODELS = ("recsys-als", "recsys-gbdt", "recsys-two-tower")
EVAL_PROTOCOL = "leave-last-new-item-v1"  # the constant the recsys job stamps (see tpr_steps.py)

# Metrics of a champion the nightly candidate beats, and of one nothing can beat.
WEAK = {
    "recsys-als": {"ndcg@10": 0.001, "coverage@10": 0.001},
    "recsys-gbdt": {"ndcg@10": 0.001, "recall@10": 0.001},
    "recsys-two-tower": {"recall@10": 0.001, "coverage@10": 0.001},
}
# ndcg and coverage are in [0, 1]: a 2.0 champion cannot be matched, let alone improved on by 1 %.
STRONG = {
    "recsys-als": {"ndcg@10": 2.0, "coverage@10": 2.0},
    "recsys-gbdt": {"ndcg@10": 2.0, "recall@10": 2.0},
    "recsys-two-tower": {"recall@10": 2.0, "coverage@10": 2.0},
}


def mlflow_url() -> str:
    return os.getenv("MLFLOW_URL", "http://localhost:5050").rstrip("/")


@dataclass(frozen=True)
class Hit:
    # param | metric | tag | run_name | artifact_name | artifact_content | model_* | version_* | job_stdout
    where: str
    run_id: str
    detail: str  # key or artifact path
    key: str  # the user key that was found


@dataclass
class ScanReport:
    runs: int = 0
    artifact_files: int = 0
    artifact_paths: list[str] = field(default_factory=list)
    registry_objects: int = 0
    text_sources: int = 0
    hits: list[Hit] = field(default_factory=list)


class MlflowRegistry:
    def __init__(self, base: str | None = None) -> None:
        self.base = (base or mlflow_url()).rstrip("/")
        self.http = httpx.Client(timeout=30.0)

    # ── plumbing ──────────────────────────────────────────────────────────
    def _call(self, method: str, path: str, *, params=None, body=None) -> dict:
        resp = self.http.request(method, f"{self.base}{path}", params=params, json=body)
        if resp.status_code >= 300:
            raise AssertionError(f"mlflow {method} {path} -> {resp.status_code}: {resp.text[:300]}")
        return resp.json() if resp.content else {}

    def healthy(self) -> bool:
        try:
            return self.http.get(f"{self.base}/health", timeout=3.0).status_code == 200
        except httpx.HTTPError:
            return False

    def wait_healthy(self, timeout_s: float = 120.0) -> None:
        deadline = time.monotonic() + timeout_s
        while time.monotonic() < deadline:
            if self.healthy():
                return
            time.sleep(1.0)
        raise AssertionError(f"MLflow at {self.base} not healthy after {timeout_s:.0f}s")

    # ── registered models ─────────────────────────────────────────────────
    def registered_models(self, prefix: str = "recsys-") -> list[str]:
        names: list[str] = []
        token = None
        while True:
            params = {"max_results": 100, **({"page_token": token} if token else {})}
            data = self._call("GET", "/api/2.0/mlflow/registered-models/search", params=params)
            names += [m["name"] for m in data.get("registered_models", [])]
            token = data.get("next_page_token")
            if not token:
                break
        return sorted(n for n in names if n.startswith(prefix))

    def delete_model(self, name: str) -> None:
        self._call("DELETE", "/api/2.0/mlflow/registered-models/delete", body={"name": name})

    def reset_registry(self, prefix: str = "recsys-") -> None:
        """Delete every registered model whose name starts with `prefix` (D13 reset)."""
        for name in self.registered_models(prefix):
            self.delete_model(name)
        left = self.registered_models(prefix)
        assert not left, f"registered models survived the reset: {left}"

    def versions(self, name: str) -> list[dict]:
        out: list[dict] = []
        token = None
        while True:
            params = {"filter": f"name='{name}'", "max_results": 200}
            if token:
                params["page_token"] = token
            data = self._call("GET", "/api/2.0/mlflow/model-versions/search", params=params)
            out += data.get("model_versions", [])
            token = data.get("next_page_token")
            if not token:
                break
        return sorted(out, key=lambda v: int(v["version"]))

    def version_counts(self, prefix: str = "recsys-") -> dict[str, int]:
        return {name: len(self.versions(name)) for name in self.registered_models(prefix)}

    def newest_version(self, name: str) -> dict | None:
        found = self.versions(name)
        return found[-1] if found else None

    @staticmethod
    def tags(version: dict) -> dict[str, str]:
        raw = version.get("tags") or []
        if isinstance(raw, dict):
            return dict(raw)
        return {t["key"]: t["value"] for t in raw}

    def alias(self, name: str, alias: str) -> int | None:
        """Version number the alias names, or None when the alias (or the model) is absent."""
        resp = self.http.get(
            f"{self.base}/api/2.0/mlflow/registered-models/alias",
            params={"name": name, "alias": alias},
        )
        if resp.status_code == 200:
            return int(resp.json()["model_version"]["version"])
        return None

    # ── seeding (py-modelregistry `seed`, run as the library's own caller) ──
    def seed(self, model: str, metrics: dict[str, float], *, protocol: str | None = EVAL_PROTOCOL):
        lineage = {"data_version": "e2e-seed"}
        if protocol is not None:
            lineage["eval_protocol"] = protocol
        code = textwrap.dedent(f"""
            from modelregistry import seed
            seed({model!r}, {metrics!r}, {lineage!r}, "e2e seeded champion")
            """)
        env = {**os.environ, "PYTHONPATH": str(LIB_SRC), "MLFLOW_TRACKING_URI": self.base}
        proc = subprocess.run(
            [sys.executable, "-c", code],
            capture_output=True,
            text=True,
            env=env,
            timeout=180,
            check=False,
        )
        assert proc.returncode == 0, f"seed({model}) failed: {proc.stderr[-800:]}"

    # ── runs ──────────────────────────────────────────────────────────────
    def run(self, run_id: str) -> dict:
        return self._call("GET", "/api/2.0/mlflow/runs/get", params={"run_id": run_id})["run"]

    def run_metrics(self, run_id: str) -> dict[str, float]:
        """Latest value of every metric of the run, keyed as stored (`ndcg_at_10`)."""
        return {
            m["key"]: float(m["value"]) for m in self.run(run_id).get("data", {}).get("metrics", [])
        }

    def experiment_ids(self) -> list[str]:
        data = self._call(
            "POST",
            "/api/2.0/mlflow/experiments/search",
            body={"max_results": 1000, "view_type": "ACTIVE_ONLY"},
        )
        return [e["experiment_id"] for e in data.get("experiments", [])]

    def run_ids_since(self, start_ms: int) -> list[str]:
        ids: list[str] = []
        token = None
        while True:
            body = {
                "experiment_ids": self.experiment_ids(),
                "filter": f"attributes.start_time >= {start_ms}",
                "max_results": 1000,
                **({"page_token": token} if token else {}),
            }
            data = self._call("POST", "/api/2.0/mlflow/runs/search", body=body)
            ids += [r["info"]["run_id"] for r in data.get("runs", [])]
            token = data.get("next_page_token")
            if not token:
                return ids

    def list_artifacts(self, run_id: str, path: str = "") -> list[str]:
        """Every artifact file path (recursive) of the run."""
        data = self._call(
            "GET",
            "/api/2.0/mlflow/artifacts/list",
            params={"run_id": run_id, **({"path": path} if path else {})},
        )
        files: list[str] = []
        for entry in data.get("files", []):
            if entry.get("is_dir"):
                files += self.list_artifacts(run_id, entry["path"])
            else:
                files.append(entry["path"])
        return files

    def read_artifact(self, run_id: str, path: str) -> bytes:
        resp = self.http.get(f"{self.base}/get-artifact", params={"path": path, "run_uuid": run_id})
        assert resp.status_code == 200, f"artifact {path} of {run_id}: {resp.status_code}"
        return resp.content

    # ── scratch runs (negative control of the scan) ───────────────────────
    def create_scratch_run(self, experiment: str = "e2e-pii-control") -> str:
        resp = self.http.get(
            f"{self.base}/api/2.0/mlflow/experiments/get-by-name",
            params={"experiment_name": experiment},
        )
        if resp.status_code == 200:
            exp_id = resp.json()["experiment"]["experiment_id"]
        else:
            exp_id = self._call(
                "POST", "/api/2.0/mlflow/experiments/create", body={"name": experiment}
            )["experiment_id"]
        run = self._call(
            "POST",
            "/api/2.0/mlflow/runs/create",
            body={"experiment_id": exp_id, "start_time": int(time.time() * 1000)},
        )["run"]
        return run["info"]["run_id"]

    def log_param(self, run_id: str, key: str, value: str) -> None:
        self._call(
            "POST",
            "/api/2.0/mlflow/runs/log-parameter",
            body={"run_id": run_id, "key": key, "value": value},
        )

    def log_artifact_text(self, run_id: str, name: str, text: str) -> None:
        info = self.run(run_id)["info"]
        # artifact_uri is mlflow-artifacts:/<exp>/<run>/artifacts (proxied by the server)
        root = info["artifact_uri"].split(":/", 1)[1].lstrip("/")
        resp = self.http.put(
            f"{self.base}/api/2.0/mlflow-artifacts/artifacts/{root}/{name}",
            content=text.encode(),
        )
        assert resp.status_code < 300, f"artifact upload {name}: {resp.status_code} {resp.text}"

    def delete_run(self, run_id: str) -> None:
        self._call("POST", "/api/2.0/mlflow/runs/delete", body={"run_id": run_id})

    # ── the PII scan ──────────────────────────────────────────────────────
    def scan_runs(self, run_ids: list[str], user_keys: list[str]) -> ScanReport:
        """Look for any of `user_keys` in everything a run holds.

        Covers params (key and value), metrics (key), tags (system tags included), the run name
        and every artifact file (its path and its bytes).
        """
        report = ScanReport()
        pattern = re.compile("|".join(re.escape(k) for k in sorted(set(user_keys))))

        def check(where: str, run_id: str, detail: str, text: str) -> None:
            for m in pattern.finditer(text):
                report.hits.append(Hit(where, run_id, detail, m.group(0)))

        for run_id in dict.fromkeys(run_ids):
            run = self.run(run_id)
            report.runs += 1
            info, data = run["info"], run.get("data", {})
            check("run_name", run_id, "run_name", info.get("run_name") or "")
            for p in data.get("params", []):
                check("param", run_id, p["key"], f"{p['key']}={p['value']}")
            for m in data.get("metrics", []):
                check("metric", run_id, m["key"], m["key"])
            for t in data.get("tags", []):
                check("tag", run_id, t["key"], f"{t['key']}={t['value']}")
            for path in self.list_artifacts(run_id):
                report.artifact_files += 1
                report.artifact_paths.append(path)
                check("artifact_name", run_id, path, path)
                content = self.read_artifact(run_id, path)
                check("artifact_content", run_id, path, content.decode("utf-8", "replace"))
        return report

    # ── registry-side scan: model tags / descriptions, version tags / descriptions ──
    def registered_model(self, name: str) -> dict:
        return self._call("GET", "/api/2.0/mlflow/registered-models/get", params={"name": name})[
            "registered_model"
        ]

    def scan_registry(
        self, user_keys: list[str], report: ScanReport | None = None, names: list[str] | None = None
    ) -> ScanReport:
        """Look for any of `user_keys` in the registered models and their versions.

        Covers the registered-model name, description, tags and aliases and, for every version,
        its tags (gate.reason, lineage.*, ...), description, source and run link.
        """
        report = report if report is not None else ScanReport()
        pattern = re.compile("|".join(re.escape(k) for k in sorted(set(user_keys))))

        def check(where: str, detail: str, text: str) -> None:
            for m in pattern.finditer(text):
                report.hits.append(Hit(where, "", detail, m.group(0)))

        for name in names if names is not None else self.registered_models():
            model = self.registered_model(name)
            report.registry_objects += 1
            check("model_name", name, name)
            check("model_description", name, model.get("description") or "")
            for t in model.get("tags") or []:
                check("model_tag", f"{name}:{t['key']}", f"{t['key']}={t['value']}")
            for a in model.get("aliases") or []:
                check("model_alias", name, f"{a['alias']}={a.get('version', '')}")
            for v in self.versions(name):
                report.registry_objects += 1
                detail = f"{name}@v{v['version']}"
                check("version_description", detail, v.get("description") or "")
                check("version_source", detail, f"{v.get('source', '')} {v.get('run_id', '')}")
                for key, value in self.tags(v).items():
                    check("version_tag", f"{detail}:{key}", f"{key}={value}")
        return report

    @staticmethod
    def scan_text(
        report: ScanReport, where: str, detail: str, text: str, user_keys: list[str]
    ) -> None:
        """Add a hit for every user key found in free text (e.g. a job's stdout)."""
        pattern = re.compile("|".join(re.escape(k) for k in sorted(set(user_keys))))
        report.text_sources += 1
        for m in pattern.finditer(text):
            report.hits.append(Hit(where, "", detail, m.group(0)))

    # ── scratch registered model (negative control of the registry scan) ──
    def create_scratch_version_with_tag(self, name: str, key: str, value: str) -> None:
        run_id = self.create_scratch_run()
        source = self.run(run_id)["info"]["artifact_uri"]
        self._call("POST", "/api/2.0/mlflow/registered-models/create", body={"name": name})
        version = self._call(
            "POST",
            "/api/2.0/mlflow/model-versions/create",
            body={"name": name, "source": source, "run_id": run_id},
        )["model_version"]["version"]
        self._call(
            "POST",
            "/api/2.0/mlflow/model-versions/set-tag",
            body={"name": name, "version": version, "key": key, "value": value},
        )
