"""Run the real platform-forecast job against the stack (forecast-nightly-job).

Modelled on ofb_flow / fsm_job_flow: `docker run --rm --network <stack>` of the job image.

* `make_export` builds a synthetic `order_facts.parquet` (inside the job image) in a temp dir.
* `run_job` runs `python -m forecast run` on either that dir or the analytics volume, writing Redis DB 3,
  and returns exit code, stdout and stderr apart (the stdout summary line is part of the contract).
* `Redis` is the host RESP client for the stack Redis on DB 3 (GET, TTL, KEYS, DEL).

Environment: FORECAST_IMAGE (platform-forecast:local); the volume and Redis host/port come from
fsm_job_flow (FSM_ANALYTICS_VOLUME, FSM_REDIS_HOST, FSM_REDIS_PORT). STACK_NETWORK from the e2e settings.
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
from tests.e2e.flows import fsm_job_flow as fsm

REDIS_DB = 3
META_KEY = "fc:v1:meta"
_DRIVER = Path(__file__).with_name("fnj_job_driver.py")
_TIMEOUT_S = 600


def image() -> str:
    return os.getenv("FORECAST_IMAGE", "platform-forecast:local")


@dataclass(frozen=True)
class Result:
    exit_code: int
    stdout: str
    stderr: str


def make_export(
    lines: list[dict], *, file: bool = True, drop_columns: tuple[str, ...] = ()
) -> Path:
    """Synthetic input dir holding order_facts.parquet from {seller, listing, day, qty, status} lines."""
    work = Path(tempfile.mkdtemp(prefix="fnj-drv-"))
    shutil.copy(_DRIVER, work / "driver.py")
    plan = {"op": "mkexport", "lines": lines, "file": file, "drop_columns": list(drop_columns)}
    (work / "plan.json").write_text(json.dumps(plan))
    work.chmod(0o777)
    args = ["docker", "run", "--rm", "-v", f"{work}:/work"]
    args += ["--entrypoint", "python", image(), "/work/driver.py"]
    proc = subprocess.run(args, capture_output=True, text=True, timeout=_TIMEOUT_S, check=False)
    assert proc.returncode == 0, f"export driver failed:\n{proc.stderr[-2000:]}"
    inputs = work / "in"
    inputs.chmod(0o755)
    return inputs


def drop(inputs: Path) -> None:
    shutil.rmtree(inputs.parent, ignore_errors=True)


def run_job(*, as_of: str, inputs: Path | None = None) -> Result:
    """`python -m forecast run` over synthetic `inputs`, or over the analytics volume."""
    src = f"{inputs}:/data:ro" if inputs else f"{fsm.analytics_volume()}:/data:ro"
    env = {
        "INPUT_DIR": "/data",
        "REDIS_URL": f"redis://redis:6379/{REDIS_DB}",
        "AS_OF": as_of,
    }
    args = ["docker", "run", "--rm", "--network", get_settings().stack_network, "-v", src]
    for k, v in env.items():
        args += ["-e", f"{k}={v}"]
    args += [image(), "run"]
    proc = subprocess.run(args, capture_output=True, text=True, timeout=_TIMEOUT_S, check=False)
    return Result(proc.returncode, proc.stdout, proc.stderr)


class Redis(fsm.Online):
    """RESP client on DB 3 with array replies (KEYS) on top of fsm_job_flow.Online."""

    def __init__(self) -> None:
        super().__init__()
        self.call("SELECT", str(REDIS_DB))

    def _read(self):
        line = self._file.readline().rstrip(b"\r\n")
        if line[:1] == b"*":
            n = int(line[1:])
            if n < 0:
                return None
            items = []
            for _ in range(n):
                head = self._file.readline().rstrip(b"\r\n")
                size = int(head[1:])
                items.append(self._file.read(size + 2)[:-2].decode())
            return items
        return self._read_scalar(line)

    def _read_scalar(self, line: bytes):
        kind, rest = line[:1], line[1:]
        if kind == b"-":
            raise RuntimeError(rest.decode())
        if kind == b"+":
            return rest.decode()
        if kind == b":":
            return int(rest)
        if kind == b"$":
            n = int(rest)
            if n < 0:
                return None
            return self._file.read(n + 2)[:-2].decode()
        raise RuntimeError(f"unsupported RESP reply {line!r}")

    def keys(self, pattern: str) -> list[str]:
        return sorted(self.call("KEYS", pattern) or [])

    def ttl(self, key: str) -> int:
        return self.call("TTL", key)

    def delete(self, *keys: str) -> None:
        if keys:
            self.call("DEL", *keys)
