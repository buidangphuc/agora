"""Generate a tiny governed `als_interactions` dataset (parquet + manifest) for local runs.

Mirrors what platform-featurestore writes: ``as_of=<stamp>.parquet`` with the columns
user_key, listing_id, weight, interactions, last_occurred_at, and the
``as_of=<stamp>.manifest.json`` beside it. Uses pandas + pyarrow only (no Spark).

    python sample_data/generate_sample.py ./data/datasets/als_interactions/v1
"""

from __future__ import annotations

import hashlib
import json
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

COLUMNS = ["user_key", "listing_id", "weight", "interactions", "last_occurred_at"]

# (user_key, listing_id, weight, interactions): a few users over a handful of listings
# with a spread of weights so ALS has signal.
_PAIRS = [
    ("user-1", "listing-a", 3.0, 2),
    ("user-1", "listing-b", 1.0, 1),
    ("user-1", "listing-c", 5.0, 1),
    ("user-2", "listing-a", 1.0, 1),
    ("user-2", "listing-b", 2.0, 1),
    ("user-2", "listing-d", 1.0, 1),
    ("anon-1", "listing-b", 1.0, 1),
    ("anon-1", "listing-c", 7.0, 2),
    ("anon-2", "listing-a", 0.5, 1),
    ("anon-2", "listing-d", 3.0, 2),
    ("user-3", "listing-c", 1.0, 1),
    ("user-3", "listing-d", 5.0, 1),
    ("user-3", "listing-a", 1.0, 1),
]


def main(dst_dir: str) -> None:
    import pandas as pd  # noqa: PLC0415

    now = datetime.now(timezone.utc).replace(microsecond=0)
    as_of = now.strftime("%Y%m%dT%H%M%SZ")
    rows = [(u, lid, w, n, now - timedelta(hours=i)) for i, (u, lid, w, n) in enumerate(reversed(_PAIRS))]
    df = pd.DataFrame(rows, columns=COLUMNS).astype({"weight": "float64", "interactions": "int32"})
    out = Path(dst_dir)
    out.mkdir(parents=True, exist_ok=True)
    parquet = out / f"as_of={as_of}.parquet"
    df.to_parquet(parquet, index=False, coerce_timestamps="ms", allow_truncated_timestamps=True)
    manifest = {
        "name": "als_interactions",
        "version": 1,
        "as_of": as_of,
        "window_days": 30,
        "rows": len(df),
        "users": int(df["user_key"].nunique()),
        "items": int(df["listing_id"].nunique()),
        "definition_sha256": "",
        "input_watermark": now.isoformat().replace("+00:00", "Z"),
        "file_sha256": hashlib.sha256(parquet.read_bytes()).hexdigest(),
        "file": parquet.name,
    }
    (out / f"as_of={as_of}.manifest.json").write_text(json.dumps(manifest, indent=2))
    print(f"wrote {len(df)} rows -> {parquet}")


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else "./data/datasets/als_interactions/v1")
