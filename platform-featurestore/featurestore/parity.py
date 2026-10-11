"""Online/offline parity (design D5)."""

from __future__ import annotations

import hashlib
import json
import math
from dataclasses import dataclass
from pathlib import Path

import pyarrow.parquet as pq

from featurestore.online import read_row
from featurestore.settings import ConfigError


@dataclass(frozen=True)
class Mismatch:
    view: str
    entity: str
    feature: str
    online: object
    offline: object

    def line(self) -> str:
        return (
            f"parity mismatch view={self.view} entity={self.entity} feature={self.feature} "
            f"online={self.online} offline={self.offline}"
        )


def values_equal(a, b) -> bool:
    if (
        isinstance(a, bool)
        or isinstance(b, bool)
        or not isinstance(a, (int, float))
        or not isinstance(b, (int, float))
    ):
        return a == b
    return math.isclose(a, b, rel_tol=1e-9, abs_tol=1e-9)


def sample_ids(ids: list[str], as_of_key: str, n: int) -> list[str]:
    def h(i: str) -> str:
        return hashlib.sha256(f"{i}|{as_of_key}".encode()).hexdigest()

    return sorted(ids, key=h)[:n]


def check_view(r, view: str, version: int, rows: list[dict], as_of_key: str, n: int) -> list[Mismatch]:
    by_id = {row["entity_id"]: row for row in rows}
    out: list[Mismatch] = []
    for eid in sample_ids(list(by_id), as_of_key, n):
        offline = {k: v for k, v in by_id[eid].items() if k != "entity_id"}
        online = read_row(r, view, version, eid)
        if online is None:
            out.append(Mismatch(view, eid, "<row>", None, "present"))
            continue
        for feat, ov in offline.items():
            if feat not in online or not values_equal(online[feat], ov):
                out.append(Mismatch(view, eid, feat, online.get(feat), ov))
    return out


def latest_manifest(offline_dir: Path) -> dict:
    best = None
    for p in (offline_dir / "runs").glob("*/manifest.json"):
        m = json.loads(p.read_text())
        k = (m["materialized_at"], p.parent.name)
        if best is None or k > best[0]:
            best = (k, m)
    if best is None:
        raise ConfigError(f"no manifest under {offline_dir / 'runs'}")
    return best[1]


def check_manifest(r, offline_dir: Path, manifest: dict, n: int) -> list[Mismatch]:
    out: list[Mismatch] = []
    for v in manifest["views"]:
        ent = v.get("entity", "entity_id")
        rows = [
            {("entity_id" if k == ent else k): val for k, val in r.items()}
            for r in pq.read_table(offline_dir / v["snapshot"]).to_pylist()
        ]
        out += check_view(r, v["name"], v["version"], rows, manifest["as_of"], n)
    return out
