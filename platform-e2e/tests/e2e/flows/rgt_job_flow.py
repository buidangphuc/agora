"""Run the real platform-recsys image against the stack for recsys-gbdt-trainer.

Same isolation as mlr_job_flow (a Redis DB and Qdrant collections per xdist worker, serving data never
touched), with the driver rgt_job_driver.py.
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

_DRIVER = Path(__file__).with_name("rgt_job_driver.py")
_TIMEOUT_S = 1500


def run_plan(steps: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Execute the driver plan and return one result dict per step."""
    settings = get_settings()
    idx = int(os.environ.get("PYTEST_XDIST_WORKER", "gw0").removeprefix("gw") or 0)
    work = Path(tempfile.mkdtemp(prefix="rgt-e2e-"))
    try:
        shutil.copy(_DRIVER, work / "driver.py")
        plan = {"namespace": f"e2e_rgt_gw{idx}", "redis_db": 10 + idx, "steps": steps}
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
