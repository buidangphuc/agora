"""Steps for model_registry/model_registry.feature (change add-model-registry).

Stack: platform-core/infra compose services postgres-mlflow, minio, minio-init, mlflow.

    MLFLOW_URL            default http://localhost:5050
    MINIO_URL             default http://localhost:9000  (minioadmin / minioadmin)
    MODELREGISTRY_COMPOSE_FILE     default <repo>/platform-core/infra/docker-compose.yaml
    MODELREGISTRY_COMPOSE_PROJECT  default agora   (compose project the stack runs under)

mlflow and boto3 are imported lazily, after the server was reached, so a missing stack shows up
as "registry not reachable" and not as an import error. The library under test is used from the
monorepo path in a subprocess (PYTHONPATH), so the e2e never imports implementation code itself.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import textwrap
import uuid
from pathlib import Path

import httpx
import pytest
from pytest_bdd import given, parsers, then, when

REPO = Path(__file__).resolve().parents[4]
LIB_SRC = REPO / "platform-core" / "packages" / "py-modelregistry" / "src"
BUCKET = "mlflow-artifacts"


def mlflow_url() -> str:
    return os.getenv("MLFLOW_URL", "http://localhost:5050").rstrip("/")


def minio_url() -> str:
    return os.getenv("MINIO_URL", "http://localhost:9000").rstrip("/")


def _require_registry() -> None:
    try:
        httpx.get(f"{mlflow_url()}/health", timeout=5).raise_for_status()
    except Exception as exc:  # noqa: BLE001
        pytest.fail(f"MLflow registry not reachable at {mlflow_url()}: {exc!r}")


def _client():
    _require_registry()
    import mlflow
    from mlflow import MlflowClient

    mlflow.set_tracking_uri(mlflow_url())
    mlflow.set_registry_uri(mlflow_url())
    return MlflowClient(tracking_uri=mlflow_url(), registry_uri=mlflow_url())


def _s3():
    import boto3

    return boto3.client(
        "s3",
        endpoint_url=minio_url(),
        aws_access_key_id="minioadmin",
        aws_secret_access_key="minioadmin",
        region_name="us-east-1",
    )


def _bucket_objects() -> dict[str, tuple[int, str]]:
    out: dict[str, tuple[int, str]] = {}
    for page in _s3().get_paginator("list_objects_v2").paginate(Bucket=BUCKET):
        for o in page.get("Contents", []):
            out[o["Key"]] = (o["Size"], o["ETag"])
    return out


def _delete_model(name: str) -> None:
    client = _client()
    from mlflow.exceptions import MlflowException

    try:
        client.delete_registered_model(name)
    except MlflowException:
        pass  # absent already


@pytest.fixture
def rctx() -> dict:
    ctx: dict = {"models": []}
    yield ctx
    try:  # reset state between scenarios: delete every probe model the scenario touched
        for name in ctx["models"]:
            _delete_model(name)
    except Exception:  # noqa: BLE001
        pass


def _log_run_with_artifact(client, model: str, tmp: Path) -> tuple[str, str]:
    """Log one run with a model artifact under model/ and register a version. Returns (run_id, version)."""
    exp = client.get_experiment_by_name("e2e-registry") or None
    exp_id = exp.experiment_id if exp else client.create_experiment("e2e-registry")
    run = client.create_run(exp_id)
    run_id = run.info.run_id
    f = tmp / "model.bin"
    f.write_bytes(uuid.uuid4().bytes)
    client.log_artifact(run_id, str(f), artifact_path="model")
    client.log_metric(run_id, "probe_metric", 0.5)
    client.set_terminated(run_id)
    try:
        client.create_registered_model(model)
    except Exception:  # noqa: BLE001
        pass
    mv = client.create_model_version(model, source=f"{run.info.artifact_uri}/model", run_id=run_id)
    return run_id, mv.version


# ── mlflow_server_up ─────────────────────────────────────────────────────
@when("the version endpoint of the registry on port 5050 is called")
def call_version(rctx: dict) -> None:
    try:
        rctx["resp"] = httpx.get(f"{mlflow_url()}/version", timeout=10)
    except Exception as exc:  # noqa: BLE001
        pytest.fail(f"MLflow registry not reachable at {mlflow_url()}: {exc!r}")


@then(parsers.parse('the registry answers 200 and the body is "{body}"'))
def version_is(rctx: dict, body: str) -> None:
    resp = rctx["resp"]
    assert resp.status_code == 200, resp.status_code
    assert resp.text.strip().strip('"') == body, resp.text


# ── shared givens ────────────────────────────────────────────────────────
@given(parsers.parse('the probe model "{name}" does not exist in the registry'))
def probe_absent(rctx: dict, name: str) -> None:
    _delete_model(name)
    rctx["models"].append(name)


# ── run_and_artifact_stored ──────────────────────────────────────────────
@when(
    parsers.parse(
        'a client logs one run with a model artifact and creates a version of "{name}"'
    )
)
def log_run_and_version(rctx: dict, name: str, tmp_path: Path) -> None:
    client = _client()
    rctx["run_id"], rctx["version"] = _log_run_with_artifact(client, name, tmp_path)


@then(parsers.parse('version {n:d} of "{name}" exists in the registry'))
def version_exists(rctx: dict, n: int, name: str) -> None:
    mv = _client().get_model_version(name, str(n))
    assert mv.run_id == rctx["run_id"], (mv.run_id, rctx["run_id"])


@then(parsers.parse('the artifact object of that run exists in bucket "{bucket}" on MinIO'))
def artifact_in_minio(rctx: dict, bucket: str) -> None:
    keys = [k for k in _bucket_objects() if rctx["run_id"] in k and k.endswith("model.bin")]
    assert keys, f"no model.bin of run {rctx['run_id']} in bucket {bucket}"


# ── bucket_init_idempotent ───────────────────────────────────────────────
@given(parsers.parse('a run with a model artifact is stored in bucket "{bucket}"'))
def run_stored(rctx: dict, bucket: str, tmp_path: Path) -> None:
    rctx["run_id"], _ = _log_run_with_artifact(_client(), rctx["models"][-1], tmp_path)
    rctx["before"] = _bucket_objects()
    assert any(rctx["run_id"] in k for k in rctx["before"]), "artifact not stored"


@when("the minio-init job runs a second time")
def rerun_init(rctx: dict) -> None:
    compose = os.getenv(
        "MODELREGISTRY_COMPOSE_FILE", str(REPO / "platform-core" / "infra" / "docker-compose.yaml")
    )
    project = os.getenv("MODELREGISTRY_COMPOSE_PROJECT", "agora")
    rctx["init"] = subprocess.run(
        ["docker", "compose", "-f", compose, "-p", project, "run", "--rm", "--no-deps", "minio-init"],
        capture_output=True,
        text=True,
        timeout=180,
    )


@then("the minio-init job exits with status 0")
def init_ok(rctx: dict) -> None:
    p = rctx["init"]
    assert p.returncode == 0, f"{p.returncode}: {p.stderr[-400:]}"


@then(parsers.parse('the bucket "{bucket}" still holds exactly the same objects as before'))
def bucket_unchanged(rctx: dict, bucket: str) -> None:
    assert _bucket_objects() == rctx["before"]


# ── gate_against_real_server ─────────────────────────────────────────────
def _lib(code: str, env: dict) -> dict:
    """Run python code against the monorepo copy of py-modelregistry; the last stdout line is JSON."""
    full_env = {**os.environ, "PYTHONPATH": str(LIB_SRC), "MLFLOW_TRACKING_URI": mlflow_url(), **env}
    p = subprocess.run(
        [sys.executable, "-c", textwrap.dedent(code)],
        capture_output=True,
        text=True,
        env=full_env,
        timeout=120,
    )
    assert p.returncode == 0, f"library call failed ({p.returncode}): {p.stderr[-600:]}"
    return json.loads(p.stdout.strip().splitlines()[-1])


@given(
    parsers.parse(
        'a test policy for "{name}" with primary "{primary}" and min relative improvement {mri:f}'
    )
)
def write_policy(rctx: dict, name: str, primary: str, mri: float, tmp_path: Path) -> None:
    path = tmp_path / "policies.yaml"
    path.write_text(f"{name}:\n  primary: {primary}\n  min_relative_improvement: {mri}\n")
    rctx["policies"] = str(path)


@given(parsers.parse('a weak champion for "{name}" seeded with "{metric}" {value:f}'))
def seed_weak(rctx: dict, name: str, metric: str, value: float) -> None:
    _require_registry()
    _lib(
        f"""
        import json
        from modelregistry import seed
        seed({name!r}, {{{metric!r}: {value}}}, {{"data_version": "d1", "eval_protocol": "p1"}}, "e2e weak champion")
        print(json.dumps({{"ok": True}}))
        """,
        {"MODELREGISTRY_POLICIES": rctx["policies"]},
    )


@when(parsers.parse('a stronger candidate with "{metric}" {value:f} is submitted for "{name}"'))
def submit_stronger(rctx: dict, metric: str, value: float, name: str, tmp_path: Path) -> None:
    art = tmp_path / "model.bin"
    art.write_bytes(b"e2e-candidate")
    run_id = _client().create_run(
        _client().create_experiment(f"e2e-gate-{uuid.uuid4().hex[:8]}")
    ).info.run_id
    rctx["decision"] = _lib(
        f"""
        import dataclasses, json
        from modelregistry import submit
        d = submit({name!r}, {run_id!r}, {{{metric!r}: {value}}}, {str(art)!r},
                   {{"data_version": "d2", "eval_protocol": "p1"}})
        print(json.dumps(dataclasses.asdict(d) if dataclasses.is_dataclass(d) else dict(d._asdict())))
        """,
        {"MODELREGISTRY_POLICIES": rctx["policies"]},
    )


@then(parsers.parse('the submit decision outcome is "{outcome}"'))
def decision_outcome(rctx: dict, outcome: str) -> None:
    assert rctx["decision"]["outcome"] == outcome, rctx["decision"]


@then(parsers.parse('in the registry alias "{alias}" of "{name}" is on version {n:d}'))
def alias_on(alias: str, name: str, n: int) -> None:
    mv = _client().get_model_version_by_alias(name, alias)
    assert mv.version == str(n), (alias, mv.version)


@then(parsers.parse('version {n:d} of "{name}" has tag "{tag}" equal to "{value}"'))
def version_tag(n: int, name: str, tag: str, value: str) -> None:
    mv = _client().get_model_version(name, str(n))
    assert mv.tags.get(tag) == value, mv.tags
