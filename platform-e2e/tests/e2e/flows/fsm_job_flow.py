"""Run the real platform-featurestore job against the stack (featurestore-materialization).

Modelled on recsys_job_flow: `docker run --rm --network <stack>` of the job image.

* `run_job` runs `python -m featurestore <materialize|parity>` with the analytics volume read-only
  at /data and a per-test temp offline dir at /features, Redis DB 2 on the stack network.
* `inspect` runs fsm_job_driver.py (read-only) in the same image to read Parquet/manifests.
* `Online` is a tiny RESP client for the stack Redis as published on the host (no redis package in
  the e2e venv). It only touches DB 2, the job's own index.

Environment (defaults match the local `agora` compose project):
    FEATURESTORE_IMAGE            platform-featurestore:local
    FSM_ANALYTICS_VOLUME          agora_analytics_data
    FSM_REDIS_HOST / FSM_REDIS_PORT   localhost / 6380   (host-published stack Redis)
    FSM_REDIS_DB                  2
STACK_NETWORK comes from the e2e settings.
"""

from __future__ import annotations

import json
import os
import shutil
import socket
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path

from config.settings import get_settings

_DRIVER = Path(__file__).with_name("fsm_job_driver.py")
_TIMEOUT_S = 600


def image() -> str:
    return os.getenv("FEATURESTORE_IMAGE", "platform-featurestore:local")


def analytics_volume() -> str:
    return os.getenv("FSM_ANALYTICS_VOLUME", "agora_analytics_data")


def redis_db() -> int:
    return int(os.getenv("FSM_REDIS_DB", "2"))


def new_offline_dir() -> Path:
    d = Path(tempfile.mkdtemp(prefix="fsm-e2e-"))
    d.chmod(0o777)  # the job image runs as a non-root uid
    return d


def drop_offline_dir(d: Path) -> None:
    shutil.rmtree(d, ignore_errors=True)


@dataclass(frozen=True)
class JobResult:
    exit_code: int
    output: str


def run_job(
    command: str, offline: Path, *, as_of: str = "", parity_sample: int = 100_000
) -> JobResult:
    """`python -m featurestore <command>`. An empty `as_of` means now."""
    env = {
        "FEATURESTORE_REDIS_URL": f"redis://redis:6379/{redis_db()}",
        "FEATURESTORE_PARITY_SAMPLE": str(parity_sample),
        "AS_OF": as_of,
    }
    args = ["docker", "run", "--rm", "--network", get_settings().stack_network]
    args += ["-v", f"{analytics_volume()}:/data:ro", "-v", f"{offline}:/features"]
    for k, v in env.items():
        args += ["-e", f"{k}={v}"]
    args += [image(), command]
    proc = subprocess.run(args, capture_output=True, text=True, timeout=_TIMEOUT_S, check=False)
    return JobResult(proc.returncode, proc.stdout + proc.stderr)


def inspect(op: str, offline: Path | None = None, **plan) -> dict:
    """Run the read-only driver in the job image; returns its JSON."""
    work = Path(tempfile.mkdtemp(prefix="fsm-drv-"))
    try:
        shutil.copy(_DRIVER, work / "driver.py")
        (work / "plan.json").write_text(json.dumps({"op": op, **plan}))
        work.chmod(0o777)
        args = ["docker", "run", "--rm", "-v", f"{work}:/work:ro"]
        args += ["-v", f"{analytics_volume()}:/data:ro"]
        if offline is not None:
            args += ["-v", f"{offline}:/features:ro"]
        args += ["--entrypoint", "python", image(), "/work/driver.py"]
        proc = subprocess.run(args, capture_output=True, text=True, timeout=_TIMEOUT_S, check=False)
        assert proc.returncode == 0, f"driver failed:\n{proc.stderr[-2000:]}"
        return json.loads(proc.stdout.strip().splitlines()[-1])
    finally:
        shutil.rmtree(work, ignore_errors=True)


class Online:
    """Minimal RESP client: SELECT, GET, SET (keeping the TTL), TTL."""

    def __init__(self) -> None:
        host = os.getenv("FSM_REDIS_HOST", "localhost")
        port = int(os.getenv("FSM_REDIS_PORT", "6380"))
        self._sock = socket.create_connection((host, port), timeout=10)
        self._file = self._sock.makefile("rb")
        self.call("SELECT", str(redis_db()))

    def close(self) -> None:
        self._file.close()
        self._sock.close()

    def call(self, *parts: str):
        out = f"*{len(parts)}\r\n"
        for p in parts:
            b = p.encode()
            out += f"${len(b)}\r\n{p}\r\n"
        self._sock.sendall(out.encode())
        return self._read()

    def _read(self):
        line = self._file.readline().rstrip(b"\r\n")
        kind, rest = line[:1], line[1:]
        if kind == b"-":
            raise RuntimeError(rest.decode())
        if kind in (b"+",):
            return rest.decode()
        if kind == b":":
            return int(rest)
        if kind == b"$":
            n = int(rest)
            if n < 0:
                return None
            data = self._file.read(n + 2)[:-2]
            return data.decode()
        raise RuntimeError(f"unsupported RESP reply {line!r}")

    def get(self, key: str) -> str | None:
        return self.call("GET", key)

    def set_keepttl(self, key: str, value: str) -> None:
        self.call("SET", key, value, "KEEPTTL")
