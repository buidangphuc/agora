"""Black-box helpers for change analytics-user-erasure (initiative trust-legal).

`platform.identity.v1.UserErased` is produced to the real `identity.events` topic
(`erasure_events_support`). Observations:

* team-analytics: read-only copies of the DuckDB warehouse (`tii_support`), scanned over every
  column of every table; Parquet exports read through the featurestore job image (it ships duckdb);
  consumer lag and DLQ topics through rpk.
* platform-recsys / team-ai / featurestore: the stack Redis and Qdrant on their host-published
  ports; the featurestore job is the real image (`fsm_job_flow`).

Environment (defaults match the local stack):
    ANALYTICS_ERASURE_GROUP   team-analytics.erasure   (ERASURE_CONSUMER_GROUP of team-analytics)
    RECSYS_ERASURE_GROUP      platform-recsys.erasure
    TEAM_AI_ERASURE_GROUP     team-ai.user-erasure
    AUE_REDIS_HOST/PORT/DB    localhost / 6379 / 0   (recs serving + team-ai keys)
    QDRANT_URL                http://localhost:6333
    AUE_EXPORT_WAIT_S         390  (PARQUET_EXPORT_INTERVAL_SECONDS is 300 locally)
"""

from __future__ import annotations

import json
import os
import re
import socket
import subprocess
import time
import uuid
from typing import Any

import httpx

from tests.e2e.flows import fsm_job_flow as fsm
from tests.e2e.support import erasure_events_support as ev
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import tii_support as tii

ANALYTICS_GROUP = os.getenv("ANALYTICS_ERASURE_GROUP", "team-analytics.erasure")
RECSYS_GROUP = os.getenv("RECSYS_ERASURE_GROUP", "platform-recsys.erasure")
TEAM_AI_GROUP = os.getenv("TEAM_AI_ERASURE_GROUP", "team-ai.user-erasure")
ANALYTICS_DLQ = "identity.events.analytics.dlq"
RECSYS_DLQ = "identity.events.recsys.dlq"
TEAM_AI_DLQ = "identity.events.team-ai.dlq"
ALL_DLQS = (ANALYTICS_DLQ, RECSYS_DLQ, TEAM_AI_DLQ)

USER_STATUS_CHANGED_TYPE = "platform.identity.v1.UserStatusChanged"
PSEUDONYM = re.compile(r"^erased:[0-9a-f]{32}$")
WITHIN_S = 60.0
EXPORT_WAIT_S = float(os.getenv("AUE_EXPORT_WAIT_S", "390"))
QDRANT_URL = os.getenv("QDRANT_URL", "http://localhost:6333")


# ── warehouse ────────────────────────────────────────────────────────────
def warehouse_hits(needles: list[str]) -> dict[str, int]:
    """`table.column` -> matching row count, over every column of every table, on one copy."""
    import shutil
    import tempfile
    from pathlib import Path

    import duckdb

    tmp = Path(tempfile.mkdtemp(prefix="aue-wh-"))
    try:
        db = tii._copy_warehouse(tmp)
        con = duckdb.connect(str(db), read_only=True)
        try:
            cols = con.execute(
                "SELECT table_name, column_name FROM information_schema.columns "
                "WHERE table_schema = 'main' AND table_name NOT LIKE 'duckdb_%'"
            ).fetchall()
            hits: dict[str, int] = {}
            for table, col in cols:
                for needle in needles:
                    n = con.execute(
                        f'SELECT COUNT(*) FROM "{table}" WHERE CAST("{col}" AS VARCHAR) LIKE ?',
                        [f"%{needle}%"],
                    ).fetchone()[0]
                    if n:
                        hits[f"{table}.{col}"] = hits.get(f"{table}.{col}", 0) + int(n)
            return hits
        finally:
            con.close()
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def wait_no_warehouse_hits(needles: list[str], timeout: float = WITHIN_S) -> None:
    deadline = time.monotonic() + timeout
    hits: dict[str, int] = {}
    err: Exception | None = None
    while time.monotonic() < deadline:
        try:
            hits = warehouse_hits(needles)
            err = None
            if not hits:
                return
        except Exception as exc:  # noqa: BLE001 - torn copy: retry
            err = exc
        time.sleep(2.0)
    raise AssertionError(
        f"warehouse still contains {needles} after {timeout:.0f}s: {hits!r} (last error {err!r})"
    )


def wait_rows(sql: str, params: list[Any], at_least: int, what: str) -> list[tuple]:
    rows = tii.warehouse_rows(sql, params, until=lambda r: len(r) >= at_least)
    assert len(rows) >= at_least, f"{what}: {len(rows)} < {at_least}"
    return rows


def count(sql: str, params: list[Any] | None = None) -> int:
    return int(tii.warehouse_query(sql, params or [])[0][0])


def table_count(table: str) -> int:
    return count(f"SELECT COUNT(*) FROM {table}")


# ── identity.events records ──────────────────────────────────────────────
def publish_status_changed(user_id: str) -> str:
    """A well-formed envelope of another identity event type (UserStatusChanged)."""
    event_id = ev.new_event_id()
    payload = ev.ide._ld(1, user_id.encode()) + ev.ide._ld(2, b"suspended")
    value = (
        ev.ide._ld(1, event_id.encode())
        + ev.ide._ld(2, USER_STATUS_CHANGED_TYPE.encode())
        + ev.ide._ld(3, ev.ide._timestamp(time.time()))
        + ev.ide._ld(7, payload)
    )
    stack.produce(ev.TOPIC, key=user_id.encode(), value=value)
    return event_id


def has_error_header(record: dict) -> bool:
    return any(k == "error" and v for k, v in record["headers"])


# ── Parquet exports ──────────────────────────────────────────────────────
_SCAN = r"""
import glob, json, sys, duckdb
needles = json.load(open("/work/needles.json"))
con = duckdb.connect()
out = {}
for path in sorted(glob.glob("/data/*.parquet")):
    cols = [r[0] for r in con.execute(f"DESCRIBE SELECT * FROM read_parquet('{path}')").fetchall()]
    total = 0
    for c in cols:
        for n in needles:
            total += con.execute(
                f'SELECT COUNT(*) FROM read_parquet(\'{path}\') WHERE CAST("{c}" AS VARCHAR) LIKE ?',
                ["%" + n + "%"],
            ).fetchone()[0]
    out[path.rsplit("/", 1)[1]] = total
print(json.dumps(out))
"""


def parquet_hits(needles: list[str]) -> dict[str, int]:
    """File name -> number of matches of `needles` over all columns of the export files."""
    import shutil
    import tempfile
    from pathlib import Path

    work = Path(tempfile.mkdtemp(prefix="aue-pq-"))
    try:
        (work / "scan.py").write_text(_SCAN)
        (work / "needles.json").write_text(json.dumps(needles))
        work.chmod(0o755)
        proc = subprocess.run(
            [
                "docker", "run", "--rm", "-v", f"{work}:/work:ro",
                "-v", f"{fsm.analytics_volume()}:/data:ro",
                "--entrypoint", "python", fsm.image(), "/work/scan.py",
            ],  # fmt: skip
            capture_output=True,
            text=True,
            timeout=300,
            check=False,
        )
        assert proc.returncode == 0, f"parquet scan failed:\n{proc.stderr[-1500:]}"
        return json.loads(proc.stdout.strip().splitlines()[-1])
    finally:
        shutil.rmtree(work, ignore_errors=True)


def wait_parquet(needles: list[str], present: bool, timeout: float = EXPORT_WAIT_S) -> dict:
    deadline = time.monotonic() + timeout
    seen: dict[str, int] = {}
    while time.monotonic() < deadline:
        seen = parquet_hits(needles)
        total = sum(seen.values())
        if (total > 0) == present:
            return seen
        time.sleep(10.0)
    raise AssertionError(
        f"export files {'never contained' if present else 'still contain'} {needles} "
        f"after {timeout:.0f}s: {seen!r}"
    )


# ── Redis (RESP with arrays) ─────────────────────────────────────────────
class Redis:
    def __init__(self, db: int | None = None) -> None:
        host = os.getenv("AUE_REDIS_HOST", "localhost")
        port = int(os.getenv("AUE_REDIS_PORT", "6379"))
        self._sock = socket.create_connection((host, port), timeout=10)
        self._file = self._sock.makefile("rb")
        self.call("SELECT", str(int(os.getenv("AUE_REDIS_DB", "0")) if db is None else db))

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
        if kind == b"+":
            return rest.decode()
        if kind == b":":
            return int(rest)
        if kind == b"$":
            n = int(rest)
            if n < 0:
                return None
            return self._file.read(n + 2)[:-2].decode()
        if kind == b"*":
            n = int(rest)
            return [self._read() for _ in range(max(n, 0))]
        raise RuntimeError(f"unsupported RESP reply {line!r}")

    def set(self, key: str, value: str = "e2e") -> None:
        self.call("SET", key, value)

    def exists(self, key: str) -> bool:
        return bool(self.call("EXISTS", key))

    def delete(self, *keys: str) -> None:
        if keys:
            self.call("DEL", *keys)

    def containing(self, needle: str) -> list[str]:
        """Keys of the DB that embed `needle` (SCAN-free: KEYS with an escaped glob)."""
        glob = re.sub(r"([\\*?\[\]])", r"\\\1", needle)
        return list(self.call("KEYS", f"*{glob}*"))


def wait_for(fn, ok, what: str, timeout: float = WITHIN_S):
    return ev.wait_for(fn, ok, what, timeout)


# ── Qdrant ───────────────────────────────────────────────────────────────
def qdrant(method: str, path: str, body: dict | None = None) -> dict:
    resp = httpx.request(method, f"{QDRANT_URL}{path}", json=body, timeout=30.0)
    assert resp.status_code < 300, f"qdrant {method} {path} -> {resp.status_code}: {resp.text}"
    return resp.json()


def new_user_collection() -> str:
    name = f"user_als_vectors__e2eaue{uuid.uuid4().hex[:8]}"
    qdrant("PUT", f"/collections/{name}", {"vectors": {"size": 4, "distance": "Cosine"}})
    return name


def put_user_point(collection: str, user_key: str) -> None:
    qdrant(
        "PUT",
        f"/collections/{collection}/points?wait=true",
        {
            "points": [
                {
                    "id": str(uuid.uuid4()),
                    "vector": [0.1, 0.2, 0.3, 0.4],
                    "payload": {"user_key": user_key},
                }
            ]
        },
    )


def user_points(collection: str, user_key: str) -> int:
    res = qdrant(
        "POST",
        f"/collections/{collection}/points/count",
        {"filter": {"must": [{"key": "user_key", "match": {"value": user_key}}]}, "exact": True},
    )
    return int(res["result"]["count"])


def drop_collection(name: str) -> None:
    httpx.delete(f"{QDRANT_URL}/collections/{name}", timeout=30.0)


def team_ai_env(name: str, default: str) -> str:
    try:
        return stack.container_env("team-ai").get(name) or default
    except Exception:  # noqa: BLE001 - container not named per default: use the documented default
        return default


def table_sizes() -> dict[str, int]:
    """Row count of every warehouse table, from one read-only copy."""
    import shutil
    import tempfile
    from pathlib import Path

    import duckdb

    tmp = Path(tempfile.mkdtemp(prefix="aue-sz-"))
    try:
        db = tii._copy_warehouse(tmp)
        con = duckdb.connect(str(db), read_only=True)
        try:
            tables = [
                r[0]
                for r in con.execute(
                    "SELECT table_name FROM information_schema.tables WHERE table_schema = 'main'"
                ).fetchall()
            ]
            return {t: int(con.execute(f'SELECT COUNT(*) FROM "{t}"').fetchone()[0]) for t in tables}
        finally:
            con.close()
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def stable_table_sizes(settle_s: float = 6.0, timeout: float = 60.0) -> dict[str, int]:
    """Table sizes that did not change over `settle_s` (the sink has nothing in flight)."""
    deadline = time.monotonic() + timeout
    prev = table_sizes()
    while time.monotonic() < deadline:
        time.sleep(settle_s)
        cur = table_sizes()
        if cur == prev:
            return cur
        prev = cur
    return prev
