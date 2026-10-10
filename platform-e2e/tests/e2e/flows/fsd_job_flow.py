"""Run the real platform-featurestore job as `dataset` against the stack (featurestore-datasets).

Reuses the featurestore-materialization flow (image, analytics volume, offline dir, `run_job`);
only the read-only inspector differs (fsd_job_driver.py).
"""

from __future__ import annotations

import json
import shutil
import subprocess
import tempfile
from pathlib import Path

from tests.e2e.flows import fsm_job_flow as job

_DRIVER = Path(__file__).with_name("fsd_job_driver.py")

new_offline_dir = job.new_offline_dir
drop_offline_dir = job.drop_offline_dir


def build_dataset(offline: Path, *, as_of: str = "") -> job.JobResult:
    """`python -m featurestore dataset`. An empty `as_of` means now."""
    return job.run_job("dataset", offline, as_of=as_of)


def inspect(op: str, offline: Path | None = None, **plan) -> dict:
    """Run the read-only driver in the job image; returns its JSON."""
    work = Path(tempfile.mkdtemp(prefix="fsd-drv-"))
    try:
        shutil.copy(_DRIVER, work / "driver.py")
        (work / "plan.json").write_text(json.dumps({"op": op, **plan}))
        work.chmod(0o777)
        args = ["docker", "run", "--rm", "-v", f"{work}:/work:ro"]
        args += ["-v", f"{job.analytics_volume()}:/data:ro"]
        if offline is not None:
            args += ["-v", f"{offline}:/features:ro"]
        args += ["--entrypoint", "python", job.image(), "/work/driver.py"]
        proc = subprocess.run(
            args, capture_output=True, text=True, timeout=job._TIMEOUT_S, check=False
        )
        assert proc.returncode == 0, f"driver failed:\n{proc.stderr[-2000:]}"
        return json.loads(proc.stdout.strip().splitlines()[-1])
    finally:
        shutil.rmtree(work, ignore_errors=True)
