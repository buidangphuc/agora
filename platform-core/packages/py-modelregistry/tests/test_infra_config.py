"""Infra compose checks for the MLflow registry (spec: model-registry,
requirement "MLflow registry runs on the infra stack")."""
import re
import shlex
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[3]
COMPOSE = ROOT / "infra" / "docker-compose.yaml"
INIT_SH = ROOT / "infra" / "postgres-init" / "10-create-service-databases.sh"


def _services() -> dict:
    return yaml.safe_load(COMPOSE.read_text(encoding="utf-8"))["services"]


def _svc(name: str) -> dict:
    services = _services()
    assert name in services, f"service {name!r} missing from {COMPOSE}"
    return services[name]


def _tokens(value) -> list[str]:
    """Flatten a compose command/entrypoint (str or list) into shell tokens."""
    if value is None:
        return []
    parts = value if isinstance(value, list) else [value]
    out: list[str] = []
    for p in parts:
        out.extend(shlex.split(p.replace(";", " ; "), posix=True))
    return out


def _env(svc: dict) -> dict:
    env = svc.get("environment") or {}
    if isinstance(env, list):
        env = dict(e.split("=", 1) for e in env)
    return {k: str(v) for k, v in env.items()}


def _mlflow_args() -> str:
    svc = _svc("mlflow")
    return " ".join(_tokens(svc.get("entrypoint")) + _tokens(svc.get("command")))


# Scenario: Backend store is not the shared postgres
def test_backend_store_is_dedicated():
    pg = _svc("postgres-mlflow")
    assert pg["image"].startswith("postgres:16"), pg["image"]
    assert _svc("postgres") is not pg
    assert pg.get("volumes"), "postgres-mlflow needs its own volume"
    assert "healthcheck" in pg
    # must not share the shared postgres's data volume
    shared_vols = {v.split(":")[0] for v in _svc("postgres")["volumes"]}
    own_vols = {v.split(":")[0] for v in pg["volumes"]}
    assert not (shared_vols & own_vols)

    ml = _svc("mlflow")
    uri = re.search(r"--backend-store-uri\s+(\S+)", _mlflow_args())
    assert uri, "mlflow has no --backend-store-uri"
    assert re.match(r"postgresql(\+\w+)?://[^/]*@postgres-mlflow(:\d+)?/", uri.group(1)), uri.group(1)
    assert "@postgres:" not in uri.group(1)
    assert "postgres-mlflow" in ml["depends_on"]
    assert ml["depends_on"]["postgres-mlflow"]["condition"] == "service_healthy"

    # no mlflow database in the shared postgres init script
    assert "mlflow" not in INIT_SH.read_text(encoding="utf-8").lower()


# Scenario: Server is up with the pinned version
def test_mlflow_pinned_v3_2_0_on_5050():
    ml = _svc("mlflow")
    assert re.search(r"mlflow:v?3\.2\.0\b", ml.get("image", "")) or "3.2.0" in str(
        ml.get("build", "")
    ), "mlflow must be pinned to 3.2.0 (no latest)"
    assert "latest" not in ml.get("image", "")
    args = _mlflow_args()
    assert re.search(r"--port\s+5050\b", args)
    assert re.search(r"--host\s+0\.0\.0\.0\b", args)
    assert any(str(p).endswith("5050:5050") for p in ml["ports"])
    assert "healthcheck" in ml and "/health" in str(ml["healthcheck"]["test"])


def test_mlflow_artifacts_on_minio():
    ml = _svc("mlflow")
    assert re.search(r"--artifacts-destination\s+s3://mlflow-artifacts/?(\s|$)", _mlflow_args())
    assert _env(ml).get("MLFLOW_S3_ENDPOINT_URL") == "http://minio:9000"
    assert ml["depends_on"]["minio"]["condition"] == "service_healthy"


# Scenario: Bucket creation is idempotent
def test_minio_init_creates_registry_bucket_idempotently():
    svc = _svc("minio-init")
    script = " ".join(_tokens(svc.get("entrypoint")) + _tokens(svc.get("command")))
    stmts = [s.strip() for s in script.split(";")]
    mb = [s for s in stmts if "mb " in s and "mlflow-artifacts" in s]
    assert len(mb) == 1, stmts
    assert "--ignore-existing" in mb[0]
    assert "local/mlflow-artifacts" in mb[0]
    # private bucket: no anonymous policy on it
    anon = [s for s in stmts if "anonymous" in s and "mlflow-artifacts" in s]
    assert anon == []
    # existing public listing-images behaviour untouched
    assert any("anonymous set download local/listing-images" in s for s in stmts)
    assert script.rstrip().endswith("exit 0")
