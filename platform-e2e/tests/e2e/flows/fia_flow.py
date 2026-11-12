"""Drives the real platform-featurestore job image for featurestore-item-attributes.

Reuses the image, analytics volume, per-worker Redis DB and RESP client of fsm_job_flow. Synthetic
scenarios build their own Parquet inputs inside the image (fia_job_driver.py), so the warehouse
schemas come from the image and not from the host venv.
"""

from __future__ import annotations

import json
import shutil
import subprocess
import tempfile
from pathlib import Path

from config.settings import get_settings
from tests.e2e.flows import fsm_job_flow as fsm
from tests.e2e.support import tii_support as tii

_DRIVER = Path(__file__).with_name("fia_job_driver.py")
_TIMEOUT_S = 600


def _driver(plan: dict, mounts: list[str], work_rw: bool = False) -> dict:
    work = Path(tempfile.mkdtemp(prefix="fia-drv-"))
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


def probe_export(listings: list[str], users: list[str]) -> dict:
    """The listing export on the analytics volume for these listings, and the resolved events of these users."""
    mounts = ["-v", f"{fsm.analytics_volume()}:/data:ro"]
    return _driver({"op": "probe", "listings": listings, "users": users}, mounts)


def _consumer_state() -> tuple[str, str, str, list[str]]:
    name = tii.analytics_container()
    insp = json.loads(subprocess.check_output(["docker", "inspect", name], text=True))[0]
    return name, insp["Config"]["Image"], insp["Config"].get("User") or "", insp["Config"]["Env"]


def seed_null_status(seed: dict) -> list[dict]:
    name, _, user, _ = _consumer_state()
    subprocess.run(["docker", "stop", name], capture_output=True, text=True, check=True)
    try:
        mounts = ["-v", f"{fsm.analytics_volume()}:/data"]
        if user:
            mounts += ["--user", user]
        return _driver({"op": "seed_null", "row": seed}, mounts)["stored"]
    finally:
        subprocess.run(["docker", "start", name], capture_output=True, text=True, check=True)


def run_replay() -> fsm.JobResult:
    name, image, _, env = _consumer_state()
    subprocess.run(["docker", "stop", name], capture_output=True, text=True, check=True)
    try:
        args = ["docker", "run", "--rm", "--network", get_settings().stack_network]
        args += ["-v", f"{fsm.analytics_volume()}:/data"]
        for e in env:
            args += ["-e", e]
        args += ["--entrypoint", "/listing-replay", image]
        proc = subprocess.run(args, capture_output=True, text=True, timeout=_TIMEOUT_S, check=False)
    finally:
        subprocess.run(["docker", "start", name], capture_output=True, text=True, check=True)
    return fsm.JobResult(proc.returncode, proc.stdout + proc.stderr)


def make_inputs(
    events: list[dict] | None = None,
    listings: list[dict] | None = None,
    drop_columns: list[str] | None = None,
) -> Path:
    """Synthetic inputs; `listings=None` omits listing_sellers.parquet. Returns the input directory."""
    plan = {
        "op": "mkinputs",
        "events": events or [],
        "listings": listings,
        "drop_columns": drop_columns or [],
    }
    out = _driver(plan, [], work_rw=True)
    inputs = Path(out["_work"]) / "in"
    inputs.chmod(0o755)
    return inputs


def drop(inputs: Path) -> None:
    shutil.rmtree(inputs.parent, ignore_errors=True)


def run_job(
    command: str, offline: Path, *, as_of: str = "", inputs: Path | None = None
) -> fsm.JobResult:
    """`python -m featurestore <command>` on synthetic `inputs`, or on the analytics volume. Writes the
    worker's Redis DB (fsm.redis_db), never DB 2."""
    src = f"{inputs}:/data:ro" if inputs else f"{fsm.analytics_volume()}:/data:ro"
    env = {
        "FEATURESTORE_REDIS_URL": f"redis://redis:6379/{fsm.redis_db()}",
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


def snapshot_rows(offline: Path, view: str) -> dict:
    """The rows of the newest snapshot of `view` in a job's offline dir."""
    return _driver({"op": "rows", "view": view}, ["-v", f"{offline}:/features:ro"])
