"""materialize: compute every registered view as of AS_OF, write offline + online, then gate on parity."""

from __future__ import annotations

import json
import os
from datetime import datetime, timezone
from pathlib import Path

import duckdb
import pyarrow as pa
import pyarrow.parquet as pq

from featurestore import inputs, online
from featurestore.registry import View
from featurestore.settings import Settings

_PA = {"int": pa.int64(), "float": pa.float64()}


def stamp(ts: datetime) -> str:
    return ts.strftime("%Y%m%dT%H%M%SZ")


def iso(ts: datetime | None) -> str | None:
    return None if ts is None else ts.strftime("%Y-%m-%dT%H:%M:%SZ")


def compute_view(con: duckdb.DuckDBPyConnection, view: View, as_of: datetime) -> list[dict]:
    cur = con.execute(view.sql_text, {"as_of": as_of})
    cols = [d[0] for d in cur.description]
    want = {"entity_id", *view.features}
    if set(cols) != want:
        raise ValueError(f"{view.key} returns columns {sorted(cols)}, registry declares {sorted(want)}")
    rows = [dict(zip(cols, r, strict=True)) for r in cur.fetchall()]
    for r in rows:
        r["entity_id"] = str(r["entity_id"])
    return sorted(rows, key=lambda r: r["entity_id"])


def write_snapshot(offline_dir: Path, view: View, as_of: datetime, rows: list[dict]) -> str:
    rel = f"{view.name}/v{view.version}/as_of={stamp(as_of)}.parquet"
    dest = offline_dir / rel
    dest.parent.mkdir(parents=True, exist_ok=True)
    schema = pa.schema(
        [(view.entity, pa.string())] + [(k, _PA.get(t, pa.string())) for k, t in view.features.items()]
    )
    renamed = [{view.entity if k == "entity_id" else k: v for k, v in r.items()} for r in rows]
    table = pa.Table.from_pylist(renamed, schema=schema)
    tmp = dest.with_suffix(".parquet.tmp")
    pq.write_table(table, tmp)
    os.replace(tmp, dest)
    return rel


def materialize(settings: Settings, views: list[View], r, now: datetime | None = None) -> dict:
    as_of = settings.as_of
    materialized_at = now or datetime.now(timezone.utc).replace(tzinfo=None, microsecond=0)
    con = inputs.connect(settings.input_dir, as_of)
    wm = inputs.watermark(con)
    meta = {"as_of": iso(as_of), "materialized_at": iso(materialized_at), "input_watermark": iso(wm)}
    entries, computed = [], {}
    for v in views:
        rows = compute_view(con, v, as_of)
        rel = write_snapshot(settings.offline_dir, v, as_of, rows)
        computed[v.key] = rows
        entries.append(
            {
                "name": v.name,
                "version": v.version,
                "entity": v.entity,
                "rows": len(rows),
                "definition_sha256": v.sha256,
                "snapshot": rel,
            }
        )
    manifest = {
        **meta,
        "views": entries,
        "inputs": inputs.input_stats(settings.input_dir),
    }
    run_dir = settings.offline_dir / "runs" / stamp(as_of)
    run_dir.mkdir(parents=True, exist_ok=True)
    mp = run_dir / "manifest.json"
    tmp = mp.with_suffix(".json.tmp")
    tmp.write_text(json.dumps(manifest, indent=2))
    os.replace(tmp, mp)
    for v in views:
        online.write_view(r, v.name, v.version, computed[v.key], settings.ttl_seconds, meta)
    manifest["_rows"] = computed
    return manifest
