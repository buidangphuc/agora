"""Drives the real platform-featurestore job image for order-facts-buyer.

Same image and stack as fsm_job_flow (it reuses its image/volume/Online helpers), but every run
writes to Redis DB 3 so these scenarios never overwrite the `fs:*:current|meta` pointers that the
featurestore-materialization scenarios read in DB 2.

* `inspect_orders` reads the analytics export `order_facts.parquet` (read-only).
* `make_inputs` builds synthetic Parquet inputs (order lines only) in a temp dir.
* `run_job` runs `materialize` / `parity` on either the analytics volume or the synthetic inputs.
"""

from __future__ import annotations

import json
import shutil
import subprocess
import tempfile
from pathlib import Path

from config.settings import get_settings
from tests.e2e.flows import fsm_job_flow as fsm

REDIS_DB = 3
_DRIVER = Path(__file__).with_name("ofb_job_driver.py")
_TIMEOUT_S = 600


def _driver(plan: dict, mounts: list[str], work_rw: bool = False) -> dict:
    work = Path(tempfile.mkdtemp(prefix="ofb-drv-"))
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


def inspect_orders(order_ids: list[str]) -> dict:
    """Columns of order_facts.parquet on the analytics volume and the rows of the given orders."""
    mounts = ["-v", f"{fsm.analytics_volume()}:/data:ro"]
    return _driver({"op": "orders", "order_ids": order_ids}, mounts)


def make_inputs(lines: list[dict]) -> Path:
    """Synthetic inputs holding only these order lines ({order, buyer, at, status?}); returns the dir."""
    out = _driver({"op": "mkinputs", "lines": lines}, [], work_rw=True)
    inputs = Path(out["_work"]) / "in"
    inputs.chmod(0o755)
    return inputs


def drop(inputs: Path) -> None:
    shutil.rmtree(inputs.parent, ignore_errors=True)


def run_job(
    command: str, offline: Path, *, as_of: str = "", inputs: Path | None = None
) -> fsm.JobResult:
    """`python -m featurestore <command>` against synthetic `inputs`, or the analytics volume."""
    src = f"{inputs}:/data:ro" if inputs else f"{fsm.analytics_volume()}:/data:ro"
    env = {
        "FEATURESTORE_REDIS_URL": f"redis://redis:6379/{REDIS_DB}",
        "FEATURESTORE_PARITY_SAMPLE": "100000",
        "AS_OF": as_of,
    }
    args = ["docker", "run", "--rm", "--network", get_settings().stack_network]
    args += ["-v", src, "-v", f"{offline}:/features"]
    for k, v in env.items():
        args += ["-e", f"{k}={v}"]
    args += [fsm.image(), command]
    proc = subprocess.run(args, capture_output=True, text=True, timeout=_TIMEOUT_S, check=False)
    return fsm.JobResult(proc.returncode, proc.stdout + proc.stderr)


def online() -> fsm.Online:
    """The stack Redis client, on DB 3."""
    cli = fsm.Online()
    cli.call("SELECT", str(REDIS_DB))
    return cli
