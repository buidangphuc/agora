"""Host side of the recsys-auto-train e2e: run the job in its image, read Redis, stop MLflow.

The job image is started with `docker run` on the stack network with recsys_job_runner.py mounted
as its program (see that file). Redis is read with `docker exec <redis> redis-cli` (the host venv
has no redis client). Nothing here imports recsys code.

    STACK_NETWORK     compose network of the stack (config.settings)
    RECSYS_IMAGE      job image (config.settings)
    REDIS_CONTAINER   default agora-redis-1
    MLFLOW_CONTAINER  default agora-mlflow-1
    QDRANT_URL        default http://localhost:6333
    MLFLOW_IN_STACK   MLflow as the job reaches it, default http://mlflow:5050
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path

import httpx

from config.settings import get_settings
from src.api.mlflow_registry import MlflowRegistry

_REPO = Path(__file__).resolve().parents[3]
_RUNNER = Path(__file__).with_name("recsys_job_runner.py")
_DRIVER = _REPO / "platform-e2e" / "tests" / "e2e" / "flows" / "recsys_job_driver.py"
_JOB_TIMEOUT_S = 900

MLFLOW_IN_STACK = os.getenv("MLFLOW_IN_STACK", "http://mlflow:5050")
PREFIX = "recs:v1"
ITEM_PREFIX = "item_als_vectors__"
USER_PREFIX = "user_als_vectors__"


def redis_container() -> str:
    return os.getenv("REDIS_CONTAINER", "agora-redis-1")


def mlflow_container() -> str:
    return os.getenv("MLFLOW_CONTAINER", "agora-mlflow-1")


def qdrant_url() -> str:
    return os.getenv("QDRANT_URL", "http://localhost:6333").rstrip("/")


@dataclass(frozen=True)
class JobResult:
    exit_code: int
    duration_s: float
    log: str

    @property
    def started_spark(self) -> bool:
        # The JVM gateway announces itself on stderr as soon as a Spark session starts.
        markers = ("Setting default log level", "SparkContext", "Using Spark's default log4j")
        return any(m in self.log for m in markers)


def job_action(
    dataset: str = "better_b",
    *,
    age_hours: float = 0,
    env: dict[str, str | None] | None = None,
    command: str | None = None,
) -> dict:
    """One `python -m recsys` run; MLFLOW_TRACKING_URI defaults to the stack's MLflow."""
    merged: dict[str, str | None] = {"MLFLOW_TRACKING_URI": MLFLOW_IN_STACK, **(env or {})}
    return {
        "op": "job",
        "dataset": dataset,
        "age_hours": age_hours,
        "env": merged,
        "command": command,
    }


RESET = {"op": "reset"}


def run_actions(actions: list[dict], *, buyer_id: str = "") -> list[JobResult]:
    """Run the actions in one throwaway container; returns one JobResult per `job` action."""
    settings = get_settings()
    work = Path(tempfile.mkdtemp(prefix="recsys-rat-"))
    try:
        shutil.copy(_RUNNER, work / "runner.py")
        shutil.copy(_DRIVER, work / "driver.py")
        plan = {
            # keys read when driver.py is imported
            "namespace": "e2e_recsys_rat",
            "redis_db": 0,
            "live": True,
            "buyer_id": buyer_id,
            "events": [],
            "runs": [],
            "actions": actions,
        }
        (work / "plan.json").write_text(json.dumps(plan))
        work.chmod(0o777)  # the job image runs as uid 1001
        proc = subprocess.run(
            [
                "docker",
                "run",
                "--rm",
                "--network",
                settings.stack_network,
                "-v",
                f"{work}:/work",
                "-e",
                "QDRANT_URL=http://qdrant:6333",
                "-e",
                "REDIS_HOST=redis",
                "--entrypoint",
                "python",
                settings.recsys_image,
                "/work/runner.py",
            ],
            capture_output=True,
            text=True,
            timeout=_JOB_TIMEOUT_S,
            check=False,
        )
        assert proc.returncode == 0, f"recsys runner failed:\n{(proc.stdout + proc.stderr)[-3000:]}"
        result = json.loads((work / "result.json").read_text())
    finally:
        shutil.rmtree(work, ignore_errors=True)
    return [
        JobResult(r["exit_code"], r["duration_s"], r["log"])
        for r in result["results"]
        if r["op"] == "job"
    ]


# ── Redis reads (docker exec redis-cli) ───────────────────────────────────
def _redis_cli(*args: str) -> str:
    proc = subprocess.run(
        ["docker", "exec", redis_container(), "redis-cli", "-n", "0", *args],
        capture_output=True,
        text=True,
        timeout=60,
        check=False,
    )
    assert proc.returncode == 0, f"redis-cli {args}: {proc.stderr}"
    return proc.stdout.strip()


def redis_get(key: str) -> str | None:
    out = _redis_cli("GET", key)
    return out or None


def generation_ids() -> set[str]:
    """Generations that have keys in Redis (recs:v1:gen:<generation>:...)."""
    out = _redis_cli("--scan", "--pattern", f"{PREFIX}:gen:*")
    start = len(f"{PREFIX}:gen:")
    return {line[start:].split(":", 1)[0] for line in out.splitlines() if line}


def pointers() -> dict[str, str | None]:
    return {
        "serving": redis_get(f"{PREFIX}:serving"),
        "previous": redis_get(f"{PREFIX}:previous"),
    }


def generation_collections() -> set[str]:
    resp = httpx.get(f"{qdrant_url()}/collections", timeout=30.0)
    resp.raise_for_status()
    names = {c["name"] for c in resp.json()["result"]["collections"]}
    return {n for n in names if n.startswith((ITEM_PREFIX, USER_PREFIX))}


def serving_state() -> dict:
    """Everything a scenario compares before and after a run."""
    return {
        **pointers(),
        "generations": sorted(generation_ids()),
        "collections": sorted(generation_collections()),
    }


# ── MLflow container lifecycle (registry-down scenario) ───────────────────
def stop_mlflow() -> None:
    subprocess.run(
        ["docker", "stop", mlflow_container()], check=True, capture_output=True, timeout=120
    )


def start_mlflow(registry: MlflowRegistry | None = None) -> None:
    """Teardown of the registry-down scenario: start the container and wait for /health."""
    subprocess.run(
        ["docker", "start", mlflow_container()], check=True, capture_output=True, timeout=120
    )
    (registry or MlflowRegistry()).wait_healthy(180.0)


# ── the nightly chain on the compose stack (jobs profile) ─────────────────
@dataclass(frozen=True)
class ComposeRun:
    step: str
    exit_code: int
    output: str


def compose_chain(wrapper: str, *, job_env: dict[str, str] | None = None) -> list[ComposeRun]:
    """featurestore dataset job, then the recsys job, through the stack's compose wrapper.

    `job_env` adds `-e` flags to the recsys job only; the nightly-chain scenario passes none so
    that the compose file's own wiring (MLFLOW_TRACKING_URI, ENABLE_*) is what is under test.
    """
    flags = [part for k, v in (job_env or {}).items() for part in ("-e", f"{k}={v}")]
    steps = {
        "featurestore-dataset": [
            "--profile",
            "featurestore",
            "run",
            "--rm",
            "featurestore-dataset",
        ],
        "platform-recsys": ["--profile", "jobs", "run", "--rm", *flags, "platform-recsys"],
    }
    out: list[ComposeRun] = []
    for name, args in steps.items():
        proc = subprocess.run(
            [wrapper, *args], capture_output=True, text=True, timeout=1500, check=False
        )
        out.append(ComposeRun(name, proc.returncode, proc.stdout + proc.stderr))
        if proc.returncode != 0:
            break
    return out


def restore_real_generation(wrapper: str) -> None:
    """Leave a REAL generation serving after destructive scenarios (production path, no force).

    The nightly job is denied a force promotion, so the registry is emptied first: with no
    `recsys-als` champion the real dataset's candidate is promotable and becomes the champion.
    """
    run_actions([RESET])
    MlflowRegistry().reset_registry()
    runs = compose_chain(wrapper, job_env={"MLFLOW_TRACKING_URI": MLFLOW_IN_STACK})
    failed = [r for r in runs if r.exit_code != 0]
    assert (
        not failed and len(runs) == 2
    ), f"[recsys] restore step {runs[-1].step} failed:\n{runs[-1].output[-3000:]}"
    assert redis_get(f"{PREFIX}:serving"), (
        "[recsys] the restore left no serving generation (structural gate rejected the real data?):\n"
        f"{runs[-1].output[-3000:]}"
    )
