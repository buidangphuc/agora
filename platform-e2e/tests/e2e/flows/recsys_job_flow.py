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
    summary: dict
    state: dict


def _worker_index() -> int:
    worker = os.environ.get("PYTEST_XDIST_WORKER", "gw0")
    return int(worker.removeprefix("gw") or 0)


def run_recsys_job(events: list[dict], runs: list[dict[str, str]]) -> list[JobRun]:
    """Run the job once per entry of `runs` (env overrides) over `events`.

    Each event is {"user", "listing", "event_type", "ts"} (ts = epoch seconds).
    """
    settings = get_settings()
    idx = _worker_index()
    work = Path(tempfile.mkdtemp(prefix="recsys-e2e-"))
    try:
        shutil.copy(_DRIVER, work / "driver.py")
        plan = {
            "namespace": f"e2e_recsys_gw{idx}",
            "redis_db": 10 + idx,
            "events": events,
            "runs": runs,
        }
        (work / "plan.json").write_text(json.dumps(plan))
        work.chmod(0o777)  # the job image runs as uid 1001
        subprocess.run(
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
        assert r["summary"] is not None, f"the job printed no summary:\n{r['log_tail']}"
        out.append(JobRun(exit_code=r["exit_code"], summary=r["summary"], state=r["state"]))
    return out
