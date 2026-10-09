"""dataset: build every registered governed dataset as of AS_OF, with a manifest (design D1-D3)."""

from __future__ import annotations

import hashlib
import json
import os

import duckdb
import pyarrow as pa
import pyarrow.parquet as pq

from featurestore import inputs
from featurestore.job import iso, stamp
from featurestore.registry import Dataset
from featurestore.settings import Settings

COLUMNS = ["user_key", "listing_id", "weight", "interactions", "last_occurred_at"]
SCHEMA = pa.schema(
    [
        ("user_key", pa.string()),
        ("listing_id", pa.string()),
        ("weight", pa.float64()),
        ("interactions", pa.int64()),
        ("last_occurred_at", pa.timestamp("us")),
    ]
)


def compute_dataset(con: duckdb.DuckDBPyConnection, ds: Dataset, settings: Settings) -> pa.Table:
    cur = con.execute(ds.sql_text, {"as_of": settings.as_of, "window_days": settings.dataset_window_days})
    cols = [d[0] for d in cur.description]
    if cols != COLUMNS:
        raise ValueError(f"{ds.key} returns columns {cols}, a dataset must return {COLUMNS}")
    rows = [dict(zip(cols, r, strict=True)) for r in cur.fetchall()]
    return pa.Table.from_pylist(rows, schema=SCHEMA)


def _sha256(path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def _atomic_write(path, data: str) -> None:
    tmp = path.with_name(path.name + ".tmp")
    tmp.write_text(data)
    os.replace(tmp, path)


def build_all(settings: Settings, datasets: list[Dataset]) -> list[dict]:
    con = inputs.connect(settings.input_dir, settings.as_of)
    wm = inputs.watermark(con)
    out = []
    for ds in datasets:
        table = compute_dataset(con, ds, settings)
        d = settings.offline_dir / "datasets" / ds.name / f"v{ds.version}"
        d.mkdir(parents=True, exist_ok=True)
        base = f"as_of={stamp(settings.as_of)}"
        dest = d / f"{base}.parquet"
        tmp = d / f"{base}.parquet.tmp"
        pq.write_table(table, tmp)
        os.replace(tmp, dest)
        manifest = {
            "name": ds.name,
            "version": ds.version,
            "as_of": iso(settings.as_of),
            "window_days": settings.dataset_window_days,
            "rows": table.num_rows,
            "users": len(set(table.column("user_key").to_pylist())),
            "items": len(set(table.column("listing_id").to_pylist())),
            "definition_sha256": ds.sha256,
            "input_watermark": iso(wm),
            "file_sha256": _sha256(dest),  # of the bytes after the atomic rename
            "file": dest.name,  # relative to the manifest's directory
        }
        _atomic_write(d / f"{base}.manifest.json", json.dumps(manifest, indent=2))
        out.append(manifest)
    return out
