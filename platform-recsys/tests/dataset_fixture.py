"""Write a governed dataset (parquet + manifest) the way platform-featurestore does."""

from __future__ import annotations

import hashlib
import json
from datetime import datetime, timedelta, timezone
from pathlib import Path

import pandas as pd

DATASET_NAME = "als_interactions"


def write_dataset(directory: Path, rows: list[dict], as_of: str = "20261005T020000Z") -> Path:
    """rows: dicts with user_key, listing_id, weight, interactions, last_occurred_at."""
    directory.mkdir(parents=True, exist_ok=True)
    parquet = directory / f"as_of={as_of}.parquet"
    df = pd.DataFrame(
        rows, columns=["user_key", "listing_id", "weight", "interactions", "last_occurred_at"]
    ).astype({"weight": "float64", "interactions": "int32"})
    df.to_parquet(parquet, index=False, coerce_timestamps="ms", allow_truncated_timestamps=True)
    sha = hashlib.sha256(parquet.read_bytes()).hexdigest()
    manifest = {
        "name": DATASET_NAME,
        "version": 1,
        "as_of": as_of,
        "window_days": 30,
        "rows": len(df),
        "users": int(df["user_key"].nunique()),
        "items": int(df["listing_id"].nunique()),
        "definition_sha256": "d" * 64,
        "input_watermark": "2026-10-05T02:00:00Z",
        "file_sha256": sha,
        "file": parquet.name,
    }
    (directory / f"as_of={as_of}.manifest.json").write_text(json.dumps(manifest))
    return parquet


def sample_rows() -> list[dict]:
    """Three users x three listings, each user's last-discovered listing distinct (cf. old sample)."""
    now = datetime.now(timezone.utc)
    pairs = [
        ("u1", "l1", 3.0, 2, 8),
        ("u1", "l2", 1.0, 1, 7),
        ("u1", "l3", 1.0, 1, 6),
        ("u2", "l1", 1.0, 1, 5),
        ("u2", "l2", 2.0, 1, 4),
        ("u2", "l3", 1.0, 1, 3),
        ("u3", "l2", 1.0, 1, 2),
        ("u3", "l3", 5.0, 1, 1),
        ("u3", "l1", 1.0, 1, 0),
    ]
    return [
        {
            "user_key": u,
            "listing_id": lid,
            "weight": w,
            "interactions": n,
            "last_occurred_at": now - timedelta(hours=h),
        }
        for u, lid, w, n, h in pairs
    ]
