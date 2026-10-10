"""Drives the real platform-featurestore job image for the ranking dataset (recsys-gbdt-trainer).

Reuses the image and analytics volume of fsm_job_flow. `dataset` needs no Redis, so nothing here writes to
the stack's Redis; the dataset goes to a per-scenario temp dir.
"""

from __future__ import annotations

import json
import shutil
import subprocess
import tempfile
from pathlib import Path

from tests.e2e.flows import fsm_job_flow as fsm

_DRIVER = Path(__file__).with_name("rgd_job_driver.py")
_TIMEOUT_S = 600


def _driver(plan: dict, mounts: list[str], work_rw: bool = False) -> dict:
    work = Path(tempfile.mkdtemp(prefix="rgd-drv-"))
    try:
        shutil.copy(_DRIVER, work / "driver.py")
        (work / "plan.json").write_text(json.dumps(plan))
        work.chmod(0o777)
        args = ["docker", "run", "--rm", "-v", f"{work}:/work{'' if work_rw else ':ro'}", *mounts]
        args += ["--entrypoint", "python", fsm.image(), "/work/driver.py"]
        proc = subprocess.run(args, capture_output=True, text=True, timeout=_TIMEOUT_S, check=False)
        assert proc.returncode == 0, f"driver failed:\n{proc.stderr[-2000:]}"
        out = json.loads(proc.stdout.strip().splitlines()[-1])
        if work_rw:
            out["_work"] = str(work)
        return out
    finally:
        if not work_rw:
            shutil.rmtree(work, ignore_errors=True)


def probe_export(users: list[str]) -> dict:
    return _driver({"op": "probe", "users": users}, ["-v", f"{fsm.analytics_volume()}:/data:ro"])


def make_inputs(events: list[dict]) -> Path:
    out = _driver({"op": "mkinputs", "events": events}, [], work_rw=True)
    inputs = Path(out["_work"]) / "in"
    inputs.chmod(0o755)
    return inputs


def drop(inputs: Path) -> None:
    shutil.rmtree(inputs.parent, ignore_errors=True)


def run_dataset(offline: Path, *, as_of: str = "", inputs: Path | None = None) -> fsm.JobResult:
    """`python -m featurestore dataset` on synthetic `inputs` or on the analytics volume."""
    src = f"{inputs}:/data:ro" if inputs else f"{fsm.analytics_volume()}:/data:ro"
    args = [
        "docker",
        "run",
        "--rm",
        "-v",
        src,
        "-v",
        f"{offline}:/features",
        "-e",
        f"AS_OF={as_of}",
    ]
    args += [fsm.image(), "dataset"]
    proc = subprocess.run(args, capture_output=True, text=True, timeout=_TIMEOUT_S, check=False)
    return fsm.JobResult(proc.returncode, proc.stdout + proc.stderr)


def dataset_rows(offline: Path, impressions: list[str]) -> dict:
    return _driver({"op": "rows", "impressions": impressions}, ["-v", f"{offline}:/features:ro"])
