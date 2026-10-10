"""Run the real platform-recsys batch job against the stack, in an isolated namespace.

The job image is started with `docker run` on the stack network, with
recsys_job_driver.py mounted as its program. The driver runs `python -m recsys` one or
more times and reports each run's exit code, the job's own summary and a snapshot of
the registry/serving stores. The namespace is per pytest-xdist worker (Redis DB 10+N
and Qdrant collections e2e_recsys_gwN_*), so parallel scenarios never share state and
the live serving data (Redis DB 0, the default collections) is never touched.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path

from config.settings import get_settings

_DRIVER = Path(__file__).with_name("recsys_job_driver.py")
_JOB_TIMEOUT_S = 600


@dataclass(frozen=True)
class JobRun:
    exit_code: int
    summary: dict | None
    state: dict
    log: str = ""


def _worker_index() -> int:
    worker = os.environ.get("PYTEST_XDIST_WORKER", "gw0")
    return int(worker.removeprefix("gw") or 0)


def run_recsys_job(
    events: list[dict],
    runs: list[dict[str, str | None]],
    *,
    dataset_dir: Path | None = None,
    expect_summary: bool = True,
    live: bool = False,
    reset: bool = False,
    buyer_id: str = "",
) -> list[JobRun]:
    """Run the job once per entry of `runs` (env overrides) over a governed dataset.

    The job reads only a governed dataset (featurestore-datasets). By default the driver builds a
    small `als_interactions` fixture (parquet + manifest) from `events`, each {"user", "listing",
    "event_type", "ts"} (ts = epoch seconds), and passes it as DATASET_PATH. With `dataset_dir`
    (an offline dir produced by the featurestore job) that directory is mounted read-only and
    DATASET_DIR points at its datasets/als_interactions/v1. An override value of None unsets the
    variable.

    recsys-generation-publish: a run's dict may carry the reserved keys "@fixture" (good_a,
    better_b, third_c, regressing, one_size; every fixture includes `buyer_id` as a user) and
    "@command" ("rollback"). `live=True` publishes to the stack's serving data (Redis DB 0, the
    default collections and aliases) instead of the per-worker namespace, so the gateway reads what
    the job published; `reset=True` forgets every generation first. Live runs are destructive:
    they rewrite what every other recommendation scenario sees. `expect_summary=False` allows runs that exit before printing a summary (refusals).
    """
    settings = get_settings()
    idx = _worker_index()
    work = Path(tempfile.mkdtemp(prefix="recsys-e2e-"))
    try:
        shutil.copy(_DRIVER, work / "driver.py")
        plan = {
            "namespace": f"e2e_recsys_gw{idx}",
            "redis_db": 10 + idx,
            "live": live,
            "reset": reset,
            "buyer_id": buyer_id,
            "events": events,
            "runs": runs,
            "dataset_mounted": dataset_dir is not None,
        }
        (work / "plan.json").write_text(json.dumps(plan))
        work.chmod(0o777)  # the job image runs as uid 1001
        mount = ["-v", f"{dataset_dir}:/dataset:ro"] if dataset_dir is not None else []
        subprocess.run(
            [
                "docker",
                "run",
                "--rm",
                "--network",
                settings.stack_network,
                "-v",
                f"{work}:/work",
                *mount,
                "-e",
                "QDRANT_URL=http://qdrant:6333",
                "-e",
                "REDIS_HOST=redis",
                "--entrypoint",
                "python",
                settings.recsys_image,
                "/work/driver.py",
            ],
            check=True,
            capture_output=True,
            text=True,
            timeout=_JOB_TIMEOUT_S,
        )
        result = json.loads((work / "result.json").read_text())
    finally:
        shutil.rmtree(work, ignore_errors=True)
    out = []
    for r in result["runs"]:
        if expect_summary:
            assert r["summary"] is not None, f"the job printed no summary:\n{r['log_tail']}"
        out.append(
            JobRun(
                exit_code=r["exit_code"],
                summary=r["summary"],
                state=r["state"],
                log=r["log_tail"],
            )
        )
    return out
