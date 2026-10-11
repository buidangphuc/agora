"""Run the real platform-recsys image against the stack for the ML recsys changes.

`docker run` on the stack network with mlr_job_driver.py as the program (see its docstring for the
plan). Each pytest-xdist worker has its own Redis DB (10+N) and Qdrant collections `e2e_mlr_gwN_*`,
so scenarios never share state and the live serving data (DB 0, default collections) is never touched.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import tempfile
from pathlib import Path
from typing import Any

from config.settings import get_settings

_DRIVER = Path(__file__).with_name("mlr_job_driver.py")
_TIMEOUT_S = 900


def _worker_index() -> int:
    return int(os.environ.get("PYTEST_XDIST_WORKER", "gw0").removeprefix("gw") or 0)


def run_plan(steps: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Execute the driver plan and return one result dict per step."""
    settings = get_settings()
    idx = _worker_index()
    work = Path(tempfile.mkdtemp(prefix="mlr-e2e-"))
    try:
        shutil.copy(_DRIVER, work / "driver.py")
        plan = {"namespace": f"e2e_mlr_gw{idx}", "redis_db": 10 + idx, "steps": steps}
        (work / "plan.json").write_text(json.dumps(plan))
        work.chmod(0o777)  # the job image runs as uid 1001
        subprocess.run(
            ["docker", "run", "--rm", "--network", settings.stack_network, "-v", f"{work}:/work",
             "-e", "QDRANT_URL=http://qdrant:6333", "-e", "REDIS_HOST=redis",
             "--entrypoint", "python", settings.recsys_image, "/work/driver.py"],
            check=True, capture_output=True, text=True, timeout=_TIMEOUT_S,
        )  # fmt: skip
        return json.loads((work / "result.json").read_text())["steps"]
    finally:
        shutil.rmtree(work, ignore_errors=True)
