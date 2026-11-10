"""The item_attributes@v2 snapshot every platform-recsys training run needs (shared by the job drivers).

Copied next to `driver.py` into /work by every job flow and imported by the driver INSIDE the
platform-recsys image, so it uses only pandas and pyarrow. The job refuses a training run without a
readable snapshot (exit 2, `attributes=missing`); `default_env` gives a run the default one: every
listing of its dataset LISTING_STATUS_PUBLISHED. A dataset that cannot be read yields no snapshot, so
a run without a dataset still fails on the dataset check first.
"""

from __future__ import annotations

import glob
import os
from datetime import datetime, timezone

import pandas as pd

PUBLISHED = "LISTING_STATUS_PUBLISHED"


def dataset_listings(env: dict) -> list[str]:
    """Listing ids of the dataset a run reads (DATASET_PATH file, else newest file in DATASET_DIR)."""
    path = env.get("DATASET_PATH") or ""
    if not path and env.get("DATASET_DIR"):
        files = sorted(glob.glob(f"{env['DATASET_DIR']}/as_of=*.parquet"))
        path = files[-1] if files else ""
    if not path or not os.path.isfile(path):
        return []
    try:
        return sorted({str(v) for v in pd.read_parquet(path, columns=["listing_id"])["listing_id"]})
    except Exception:
        return []


def has_own_source(env: dict) -> bool:
    return bool(env.get("ITEM_ATTRIBUTES_V2_PATH") or env.get("ITEM_ATTRIBUTES_V2_DIR"))


def write_item_attributes(statuses: dict, root: str) -> str:
    """Write an item_attributes@v2 snapshot (one row per listing, null status kept) under `root`."""
    directory = f"{root}/item_attributes/v2"
    os.makedirs(directory, exist_ok=True)
    now = datetime.now(timezone.utc)
    df = pd.DataFrame(
        {
            "listing_id": list(statuses),
            "seller_id": [f"seller-{listing}" for listing in statuses],
            "category_id": [f"category-{listing}" for listing in statuses],
            "price": [1000.0] * len(statuses),
            "status": [statuses[listing] for listing in statuses],
            "as_of": [now] * len(statuses),
        }
    )
    df["status"] = df["status"].astype("object")
    df.to_parquet(
        f"{directory}/as_of={now.strftime('%Y%m%dT%H%M%SZ')}.parquet",
        index=False,
        coerce_timestamps="ms",
        allow_truncated_timestamps=True,
    )
    return directory


def default_env(env: dict, root: str) -> dict[str, str]:
    """`{"ITEM_ATTRIBUTES_V2_DIR": dir}` with every dataset listing published; `{}` when the run has
    its own v2 source or its dataset cannot be read."""
    if has_own_source(env):
        return {}
    listed = dataset_listings(env)
    if not listed:
        return {}
    return {"ITEM_ATTRIBUTES_V2_DIR": write_item_attributes({x: PUBLISHED for x in listed}, root)}
