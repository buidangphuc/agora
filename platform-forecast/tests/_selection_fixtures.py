from __future__ import annotations

import datetime as dt

import numpy as np
import pandas as pd

AS_OF = dt.datetime(2026, 11, 10, 2, 45, tzinfo=dt.timezone.utc)
WEEKLY = [5.0, 5.0, 8.0, 8.0, 12.0, 40.0, 45.0]


def make_series(listings: dict[tuple[str, str], list[float]]) -> pd.DataFrame:
    end = pd.Timestamp(AS_OF.date() - dt.timedelta(days=1))
    frames = []
    for (seller, listing), units in listings.items():
        idx = pd.date_range(end=end, periods=len(units))
        frames.append(
            pd.DataFrame(
                {
                    "seller_id": seller,
                    "listing_id": listing,
                    "date": idx,
                    "units": np.asarray(units, dtype="int64"),
                }
            )
        )
    return pd.concat(frames, ignore_index=True)


def seasonal_noisy(days: int, seed: int) -> list[float]:
    rng = np.random.default_rng(seed)
    return [
        max(0.0, round(WEEKLY[d % 7] + rng.normal(0, 5.0)))
        for d in range(days)
    ]


def trend_series() -> pd.DataFrame:
    return make_series({("S", f"L{i}"): seasonal_noisy(84, i) for i in range(8)})
