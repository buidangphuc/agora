"""A ranking dataset and featurestore snapshots for the GBDT tests: clicks depend on item price.

24 listings ``gb-00..gb-23``: listing k has price 20000 * (k + 1) and every other registry feature is
constant, so the fixed-weight baseline (popularity and ctr only) cannot tell them apart, while the click
probability falls with price (cheap items are clicked). Ids are reversed against price so an id tie-break
cannot help the baseline.
"""

from __future__ import annotations

import hashlib
import json
import random
from datetime import datetime, timedelta
from pathlib import Path

import pandas as pd

ITEMS = 24
LISTS = 300
SHOWN = 8
FIRST_IMPRESSION = datetime(2026, 9, 10, 8, 0, 0)
SNAPSHOT_AS_OF = "20260901T000000Z"
POP_COLS = ["listing_id", "views_7d", "clicks_7d", "add_to_cart_7d", "favorites_current", "review_count",
            "avg_rating", "ctr_7d"]  # fmt: skip


def listing_id(k: int) -> str:
    return f"gb-{ITEMS - 1 - k:02d}"


def price(k: int) -> int:
    return 20_000 * (k + 1)


def click_probability(k: int) -> float:
    return 0.75 - 0.65 * k / (ITEMS - 1)


def rank_rows(seed: int = 7, signal: bool = True) -> list[dict]:
    rng = random.Random(seed)
    rows = []
    for n in range(LISTS):
        at = FIRST_IMPRESSION + timedelta(minutes=n * 90)  # ~19 days
        shown = rng.sample(range(ITEMS), SHOWN)
        for pos, k in enumerate(shown, start=1):
            p = click_probability(k) if signal else 0.3
            roll = rng.random()
            label = 0
            if roll < p:
                label = 2 if rng.random() < 0.25 else 1
            rows.append(
                {"user_key": f"u{n % 40}", "impression_id": f"imp-{n:04d}", "listing_id": listing_id(k),
                 "position": pos, "label": label, "occurred_at": at + timedelta(seconds=pos)}
            )  # fmt: skip
    return rows


def write_rank_dataset(directory: Path, rows: list[dict], as_of: str = "20261005T020000Z") -> Path:
    """The dataset the way platform-featurestore writes it: ``as_of=<stamp>.parquet`` and its manifest."""
    directory.mkdir(parents=True, exist_ok=True)
    parquet = directory / f"as_of={as_of}.parquet"
    df = pd.DataFrame(
        rows, columns=["user_key", "impression_id", "listing_id", "position", "label", "occurred_at"]
    )
    df = df.astype({"position": "int64", "label": "int64"})
    df.to_parquet(parquet, index=False, coerce_timestamps="ms", allow_truncated_timestamps=True)
    manifest = {
        "name": "rank_training", "version": 1, "as_of": as_of, "window_days": 30, "rows": len(df),
        "users": int(df["user_key"].nunique()), "items": int(df["listing_id"].nunique()),
        "definition_sha256": "r" * 64, "input_watermark": "2026-10-05T02:00:00Z",
        "file_sha256": hashlib.sha256(parquet.read_bytes()).hexdigest(), "file": parquet.name,
    }  # fmt: skip
    (directory / f"as_of={as_of}.manifest.json").write_text(json.dumps(manifest))
    return parquet


def write_popularity(directory: Path, as_of: str = SNAPSHOT_AS_OF, ctr: float = 0.05) -> Path:
    directory.mkdir(parents=True, exist_ok=True)
    rows = [[listing_id(k), 100, 5, 1, 1, 3, 4.0, ctr] for k in range(ITEMS)]
    path = directory / f"as_of={as_of}.parquet"
    pd.DataFrame(rows, columns=POP_COLS).to_parquet(path, index=False)
    return path


def write_attributes(directory: Path, as_of: str = SNAPSHOT_AS_OF) -> Path:
    directory.mkdir(parents=True, exist_ok=True)
    rows = [[listing_id(k), "seller", "cat-x", price(k)] for k in range(ITEMS)]
    path = directory / f"as_of={as_of}.parquet"
    pd.DataFrame(rows, columns=["listing_id", "seller_id", "category_id", "price"]).to_parquet(
        path, index=False
    )
    return path


def write_all(root: Path, signal: bool = True) -> dict[str, str]:
    """Dataset + one snapshot of each view older than every impression; returns Settings kwargs."""
    write_rank_dataset(root / "rank", rank_rows(signal=signal))
    write_popularity(root / "pop")
    write_attributes(root / "attr")
    return {
        "rank_dataset_dir": str(root / "rank"),
        "item_features_dir": str(root / "pop"),
        "item_attributes_dir": str(root / "attr"),
    }
